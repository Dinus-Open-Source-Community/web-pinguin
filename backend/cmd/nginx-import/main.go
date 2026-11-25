// Command nginx-import ingests an Nginx access log whose format is:
// $time_iso8601 $status $body_bytes_sent $request_uri $remote_addr
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/analytics"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/geolocation"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/platform/config"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/platform/postgres"
)

type checkpoint struct {
	Offset      int64  `json:"offset"`
	Identity    string `json:"identity,omitempty"`
	Generation  string `json:"generation,omitempty"`
	TailHash    string `json:"tail_hash,omitempty"`
	LinesRead   int64  `json:"lines_read,omitempty"`
	LinesFailed int64  `json:"lines_failed,omitempty"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("nginx import failed", "error_class", importErrorClass(err), "error", err)
		os.Exit(1)
	}
}

type classifiedImportError struct {
	class string
	err   error
}

func (e *classifiedImportError) Error() string { return e.err.Error() }
func (e *classifiedImportError) Unwrap() error { return e.err }

func importErrorClass(err error) string {
	var classified *classifiedImportError
	if errors.As(err, &classified) {
		return classified.class
	}
	return "startup_or_filesystem_error"
}

func classifyImportError(class string, err error) error {
	return &classifiedImportError{class: class, err: err}
}

func run(logger *slog.Logger) error {
	logPath := strings.TrimSpace(os.Getenv("NGINX_ACCESS_LOG"))
	if logPath == "" {
		return classifyImportError("configuration_error", fmt.Errorf("NGINX_ACCESS_LOG is required"))
	}
	logPath, err := filepath.Abs(logPath)
	if err != nil {
		return classifyImportError("configuration_error", err)
	}
	db, err := config.LoadDatabase()
	if err != nil {
		return classifyImportError("configuration_error", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, db)
	if err != nil {
		return classifyImportError("transient_database_error", fmt.Errorf("connect to database: %w", err))
	}
	defer pool.Close()
	store := analytics.NewStore(pool)
	var geo *geolocation.Lookup
	geoPath := strings.TrimSpace(os.Getenv("GEOIP_DB_PATH"))
	if geoPath != "" {
		geo, err = geolocation.Open(geoPath)
		if err != nil {
			return classifyImportError("geolocation_database_error", fmt.Errorf("open GEOIP_DB_PATH: %w", err))
		}
		defer geo.Close()
	}
	statePath := strings.TrimSpace(os.Getenv("NGINX_STATE_PATH"))
	if statePath == "" {
		statePath = logPath + ".pinguin-offset.json"
	}
	state := checkpoint{}
	if data, err := os.ReadFile(statePath); err == nil {
		if err := json.Unmarshal(data, &state); err != nil {
			return classifyImportError("checkpoint_state_error", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.Open(logPath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	identity := fileIdentity(info)
	reset := info.Size() < state.Offset || (state.Identity != "" && identity != "" && state.Identity != identity)
	if !reset && state.Offset > 0 && state.TailHash != "" {
		tail, tailErr := checkpointTail(file, state.Offset)
		if tailErr != nil {
			return tailErr
		}
		reset = tail != state.TailHash
	}
	if reset {
		logger.Warn("nginx log was truncated or replaced; restarting at byte zero", "previous_offset", state.Offset)
		state.Offset = 0
		state.TailHash = ""
		state.Generation = ""
	}
	state.Identity = identity
	if state.Generation == "" {
		if reset {
			state.Generation, err = newGeneration()
			if err != nil {
				return err
			}
		} else {
			// Preserve source keys produced by the pre-generation importer so a
			// crash between insert and checkpoint remains deduplicated on upgrade.
			state.Generation = "legacy"
		}
	}
	if err := writeCheckpoint(statePath, state); err != nil {
		return err
	}
	if _, err := file.Seek(state.Offset, 0); err != nil {
		return err
	}
	reader := bufio.NewReader(file)
	for {
		start := state.Offset
		line, readErr := reader.ReadString('\n')
		if readErr != nil && len(line) == 0 {
			break
		}
		if !strings.HasSuffix(line, "\n") {
			logger.Info("nginx importer reached partial trailing line; checkpoint retained", "byte_offset", start)
			break
		}
		lineBytes := len(line)
		line = strings.TrimSuffix(line, "\n")
		rawLine := strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(rawLine) != "" {
			fields := strings.SplitN(rawLine, " ", 5)
			if len(fields) != 5 {
				state.LinesFailed++
				if err := writeCheckpoint(statePath, state); err != nil {
					return err
				}
				return classifyImportError("permanent_input_error", fmt.Errorf("invalid nginx log line at byte %d; offset was not advanced", start))
			}
			occurredAt, e1 := time.Parse(time.RFC3339, fields[0])
			status, e2 := strconv.Atoi(fields[1])
			bytesSent, e3 := strconv.ParseInt(fields[2], 10, 64)
			if e1 != nil || e2 != nil || e3 != nil || status < 100 || status > 599 || bytesSent < 0 {
				state.LinesFailed++
				if err := writeCheckpoint(statePath, state); err != nil {
					return err
				}
				return classifyImportError("permanent_input_error", fmt.Errorf("invalid nginx log fields at byte %d; offset was not advanced", start))
			}
			requestPath := strings.SplitN(fields[3], "?", 2)[0]
			if !strings.HasPrefix(requestPath, "/") || strings.Contains(requestPath, "\\") {
				state.LinesFailed++
				if err := writeCheckpoint(statePath, state); err != nil {
					return err
				}
				return classifyImportError("permanent_input_error", fmt.Errorf("invalid request URI at byte %d; offset was not advanced", start))
			}
			geoValue := strings.Trim(fields[4], " \"")
			geoResult := geolocation.Result{}
			if ip, parseErr := netip.ParseAddr(geoValue); parseErr == nil {
				ip = ip.Unmap()
				if isNonPublicGeoIPAddress(ip) {
					// Private, loopback, and special-use addresses are intentionally
					// represented as unknown and never passed to GeoLite2.
				} else {
					if geo == nil {
						state.LinesFailed++
						if err := writeCheckpoint(statePath, state); err != nil {
							return err
						}
						return classifyImportError("configuration_error", fmt.Errorf("GEOIP_DB_PATH is required when the log contains client IP addresses; offset was not advanced"))
					}
					lookup, found, lookupErr := geo.Resolve(ip.String())
					if lookupErr != nil {
						state.LinesFailed++
						if err := writeCheckpoint(statePath, state); err != nil {
							return err
						}
						return classifyImportError("geolocation_database_error", fmt.Errorf("geolocate event at byte %d: %w; offset was not advanced", start, lookupErr))
					}
					if found {
						geoResult = lookup
					}
				}
			} else {
				country := strings.ToUpper(geoValue)
				if len(country) == 2 && country >= "AA" && country <= "ZZ" {
					geoResult = geolocation.Result{CountryCode: country, CountryName: country, RegionCode: "COUNTRY", RegionName: country}
				}
			}
			if geoResult.CountryCode == "" {
				geoResult = geolocation.Result{CountryCode: "ZZ", CountryName: "Unknown", RegionCode: "UNKNOWN", RegionName: "Unknown"}
			}
			cleanPath := strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(requestPath, "/")), "/")
			sourceName := logPath
			if state.Generation != "legacy" {
				sourceName += ":" + state.Generation
			}
			event := analytics.NginxEvent{SourceKey: analytics.NginxSourceKey(sourceName, start, []byte(rawLine)), OccurredAt: occurredAt.UTC(), FilePath: cleanPath, HTTPStatus: status, BytesSent: bytesSent, CountryCode: geoResult.CountryCode, CountryName: geoResult.CountryName, RegionCode: geoResult.RegionCode, RegionName: geoResult.RegionName, Latitude: geoResult.Latitude, Longitude: geoResult.Longitude}
			if err := store.ImportNginxEvent(ctx, event); err != nil {
				state.LinesFailed++
				if checkpointErr := writeCheckpoint(statePath, state); checkpointErr != nil {
					return checkpointErr
				}
				return classifyImportError("transient_database_error", fmt.Errorf("import event at byte %d: %w; offset was not advanced", start, err))
			}
			state.LinesRead++
		}
		state.Offset += int64(lineBytes)
		state.TailHash, err = checkpointTail(file, state.Offset)
		if err != nil {
			return err
		}
		if err := writeCheckpoint(statePath, state); err != nil {
			return err
		}
	}
	logger.Info("nginx log import completed", "offset", state.Offset, "lines_read", state.LinesRead, "lines_failed", state.LinesFailed, "file_identity_available", identity != "")
	return nil
}

func isNonPublicGeoIPAddress(ip netip.Addr) bool {
	return !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast()
}

func newGeneration() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func checkpointTail(file *os.File, offset int64) (string, error) {
	if offset <= 0 {
		return "", nil
	}
	start := offset - 256
	if start < 0 {
		start = 0
	}
	data := make([]byte, int(offset-start))
	n, err := file.ReadAt(data, start)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if n != len(data) {
		return "", io.ErrUnexpectedEOF
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func writeCheckpoint(statePath string, state checkpoint) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.WriteFile(statePath+".tmp", data, 0600); err != nil {
		return err
	}
	return os.Rename(statePath+".tmp", statePath)
}

// fileIdentity records stable filesystem identity where the platform exposes
// it. When unavailable, the importer still detects truncation by file size.
func fileIdentity(info os.FileInfo) string {
	v := reflect.ValueOf(info.Sys())
	if !v.IsValid() {
		return ""
	}
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return ""
	}
	var parts []string
	for _, name := range []string{"Dev", "Ino", "VolumeSerialNumber", "FileIndexHigh", "FileIndexLow"} {
		field := v.FieldByName(name)
		if field.IsValid() && field.CanInterface() {
			parts = append(parts, fmt.Sprint(field.Interface()))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ":")
}
