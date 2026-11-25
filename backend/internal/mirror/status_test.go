package mirror

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestIsValidTargetURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want bool
	}{
		{name: "public HTTPS", url: "https://mirror.example.org/ubuntu/", want: true},
		{name: "HTTP is rejected", url: "http://mirror.example.org/ubuntu/"},
		{name: "userinfo is rejected", url: "https://operator:secret@mirror.example.org/"},
		{name: "private IPv4 is rejected", url: "https://192.168.1.20/"},
		{name: "shared address space is rejected", url: "https://100.64.0.10/"},
		{name: "documentation address is rejected", url: "https://203.0.113.10/"},
		{name: "loopback IPv6 is rejected", url: "https://[::1]/"},
		{name: "localhost is rejected", url: "https://mirror.localhost/"},
		{name: "fragment is rejected", url: "https://mirror.example.org/#token"},
		{name: "missing host is rejected", url: "https:///path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidTargetURL(tc.url); got != tc.want {
				t.Fatalf("IsValidTargetURL(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

func TestIsPublicAddress(t *testing.T) {
	for _, tc := range []struct {
		ip   string
		want bool
	}{
		{ip: "8.8.8.8", want: true},
		{ip: "10.0.0.1", want: false},
		{ip: "100.64.0.1", want: false},
		{ip: "127.0.0.1", want: false},
		{ip: "192.0.2.1", want: false},
		{ip: "::1", want: false},
		{ip: "fd00::1", want: false},
		{ip: "2001:db8::1", want: false},
	} {
		t.Run(tc.ip, func(t *testing.T) {
			if got := isPublicAddress(net.ParseIP(tc.ip)); got != tc.want {
				t.Fatalf("isPublicAddress(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestCheckClassifiesProbeResponses(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodHead {
			t.Fatalf("request method = %s, want HEAD", r.Method)
		}
		status := http.StatusOK
		if strings.Contains(r.URL.Host, "degraded") {
			status = http.StatusServiceUnavailable
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: r}, nil
	})}
	statuses := check(context.Background(), []Target{
		{Name: "healthy", URL: "https://healthy.example.org/"},
		{Name: "degraded", URL: "https://degraded.example.org/"},
		{Name: "private", URL: "https://127.0.0.1/"},
	}, client)
	if len(statuses) != 3 {
		t.Fatalf("got %d statuses, want 3", len(statuses))
	}
	if statuses[0].Status != "healthy" || statuses[1].Status != "degraded" || statuses[2].Status != "unavailable" {
		t.Fatalf("unexpected status classification: %+v", statuses)
	}
	if statuses[2].LatencyMS != 0 {
		t.Fatalf("private target should not be probed, latency_ms=%d", statuses[2].LatencyMS)
	}
}
