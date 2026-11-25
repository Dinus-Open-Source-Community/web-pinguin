package listing

import (
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidPath = errors.New("invalid listing path")
	ErrNotFound    = errors.New("listing item not found")
	ErrNotFile     = errors.New("listing item is not a regular file")
)

type Service struct {
	root *os.Root
}

type Entry struct {
	Name        string     `json:"name"`
	Path        string     `json:"path"`
	Kind        string     `json:"kind"`
	SizeBytes   int64      `json:"size_bytes,omitempty"`
	ModifiedAt  *time.Time `json:"modified_at,omitempty"`
	DownloadURL string     `json:"download_url,omitempty"`
}

func New(rootPath string) (*Service, error) {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}
	info, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	if !info.IsDir() {
		_ = root.Close()
		return nil, errors.New("LISTING_DIR must point to a directory")
	}
	return &Service{root: root}, nil
}

func (s *Service) Close() error {
	return s.root.Close()
}

func (s *Service) Check() error {
	info, err := s.root.Stat(".")
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("LISTING_DIR is no longer available")
	}
	return nil
}

func (s *Service) List(relativePath string) ([]Entry, error) {
	relativePath, err := normalize(relativePath)
	if err != nil {
		return nil, err
	}

	dir, err := s.root.Open(relativePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer dir.Close()

	info, err := dir.Stat()
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, ErrNotFound
	}

	children, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}

	entries := make([]Entry, 0, len(children))
	for _, child := range children {
		name := child.Name()
		if name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
			continue
		}
		itemPath := name
		if relativePath != "." {
			itemPath = path.Join(relativePath, name)
		}

		itemInfo, statErr := s.root.Stat(itemPath)
		if statErr != nil || (!itemInfo.IsDir() && !itemInfo.Mode().IsRegular()) {
			// os.Root prevents a symlink from resolving outside LISTING_DIR.
			continue
		}

		entry := Entry{Name: name, Path: itemPath, ModifiedAt: timePointer(itemInfo.ModTime())}
		if itemInfo.IsDir() {
			entry.Kind = "directory"
		} else {
			entry.Kind = "file"
			entry.SizeBytes = itemInfo.Size()
			entry.DownloadURL = "/api/v1/download?path=" + url.QueryEscape(itemPath)
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind == "directory"
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, nil
}

func (s *Service) OpenFile(relativePath string) (*os.File, fs.FileInfo, error) {
	relativePath, err := normalize(relativePath)
	if err != nil || relativePath == "." {
		return nil, nil, ErrInvalidPath
	}

	file, err := s.root.Open(relativePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, ErrNotFile
	}
	return file, info, nil
}

func normalize(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "." {
		return ".", nil
	}
	if strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || strings.ContainsRune(value, 0) {
		return "", ErrInvalidPath
	}

	cleaned := path.Clean(value)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || !fs.ValidPath(cleaned) {
		return "", ErrInvalidPath
	}
	return cleaned, nil
}

func timePointer(value time.Time) *time.Time {
	return &value
}
