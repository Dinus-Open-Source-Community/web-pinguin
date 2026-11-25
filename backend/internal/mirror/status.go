package mirror

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

type Target struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
type Status struct {
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Status    string    `json:"status"`
	CheckedAt time.Time `json:"checked_at"`
	LatencyMS int64     `json:"latency_ms"`
}

func Check(ctx context.Context, targets []Target) []Status {
	return check(ctx, targets, newHTTPClient())
}

func check(ctx context.Context, targets []Target, client *http.Client) []Status {
	out := make([]Status, 0, len(targets))
	for _, target := range targets {
		item := Status{Name: target.Name, URL: target.URL, Status: "unavailable", CheckedAt: time.Now().UTC()}
		if IsValidTargetURL(target.URL) {
			start := time.Now()
			req, reqErr := http.NewRequestWithContext(ctx, http.MethodHead, target.URL, nil)
			if reqErr == nil {
				resp, doErr := client.Do(req)
				item.LatencyMS = time.Since(start).Milliseconds()
				if doErr == nil {
					resp.Body.Close()
					item.Status = "healthy"
					if resp.StatusCode >= 500 {
						item.Status = "degraded"
					}
				}
			}
		}
		out = append(out, item)
	}
	return out
}

func IsValidTargetURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && strings.EqualFold(parsed.Scheme, "https") && parsed.Opaque == "" && parsed.Hostname() != "" && parsed.User == nil && parsed.Fragment == "" && !privateHost(parsed.Hostname())
}

func newHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("mirror host did not resolve to an IP address")
		}
		for _, candidate := range ips {
			if !isPublicAddress(candidate.IP) {
				return nil, fmt.Errorf("mirror host resolved to a non-public IP address")
			}
		}
		var dialErr error
		for _, candidate := range ips {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
			if err == nil {
				return conn, nil
			}
			dialErr = err
		}
		return nil, dialErr
	}}
	return &http.Client{Timeout: 4 * time.Second, Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}

func isPublicAddress(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() || addr.IsMulticast() {
		return false
	}
	for _, blocked := range []netip.Prefix{
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("2001:db8::/32"),
	} {
		if blocked.Contains(addr) {
			return false
		}
	}
	return true
}

func privateHost(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	ip := net.ParseIP(host)
	if ip != nil {
		return !isPublicAddress(ip)
	}
	return host == "localhost" || strings.HasSuffix(strings.ToLower(host), ".localhost")
}
