package hey

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestRequireHTTPS(t *testing.T) {
	if err := requireHTTPS("https://example.com"); err != nil {
		t.Fatalf("expected no error for HTTPS, got %v", err)
	}
	if err := requireHTTPS("http://example.com"); err == nil {
		t.Fatal("expected error for HTTP")
	}
	if err := requireHTTPS("://bad"); err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestIsSameOrigin(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"https://example.com/a", "https://example.com/b", true},
		{"https://example.com", "https://other.com", false},
		{"https://example.com", "http://example.com", false},
		{"https://example.com:443/a", "https://example.com/b", true},
		{"http://example.com:80/a", "http://example.com/b", true},
		{"https://example.com:8080", "https://example.com:9090", false},
		{"/relative", "https://example.com", false},
		{"https://example.com", "/relative", false},
		{"://bad", "https://ok.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.a+"_"+tc.b, func(t *testing.T) {
			got := isSameOrigin(tc.a, tc.b)
			if got != tc.want {
				t.Fatalf("isSameOrigin(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestIsLocalhost(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"http://localhost:8080", true},
		{"http://127.0.0.1:3000", true},
		{"http://[::1]:3000", true},
		{"http://sub.localhost:3000", true},
		{"https://example.com", false},
		{"://bad", false},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			got := isLocalhost(tc.url)
			if got != tc.want {
				t.Fatalf("isLocalhost(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

func TestResolveURL(t *testing.T) {
	got := resolveURL("https://example.com/foo", "/bar")
	if got != "https://example.com/bar" {
		t.Fatalf("expected https://example.com/bar, got %s", got)
	}

	got = resolveURL("https://example.com/foo", "https://other.com/baz")
	if got != "https://other.com/baz" {
		t.Fatalf("expected absolute URL preserved, got %s", got)
	}

	// Invalid base returns target as-is
	got = resolveURL("://bad", "/bar")
	if got != "/bar" {
		t.Fatalf("expected fallback to target, got %s", got)
	}
}

func TestRequireSecureEndpoint(t *testing.T) {
	if err := RequireSecureEndpoint("http://localhost:3000"); err != nil {
		t.Fatalf("expected localhost HTTP to be allowed, got %v", err)
	}
	if err := RequireSecureEndpoint("https://example.com"); err != nil {
		t.Fatalf("expected HTTPS to be allowed, got %v", err)
	}
	if err := RequireSecureEndpoint("http://example.com"); err == nil {
		t.Fatal("expected error for non-localhost HTTP")
	}
}

func TestRedactHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer secret")
	h.Set("Cookie", "session=abc")
	h.Set("Content-Type", "application/json")
	h.Set("X-CSRF-Token", "tok")

	redacted := RedactHeaders(h)

	if redacted.Get("Authorization") != "[REDACTED]" {
		t.Fatal("expected Authorization redacted")
	}
	if redacted.Get("Cookie") != "[REDACTED]" {
		t.Fatal("expected Cookie redacted")
	}
	if redacted.Get("X-CSRF-Token") != "[REDACTED]" {
		t.Fatal("expected X-CSRF-Token redacted")
	}
	if redacted.Get("Content-Type") != "application/json" {
		t.Fatal("expected Content-Type preserved")
	}

	// Original should not be modified
	if h.Get("Authorization") != "Bearer secret" {
		t.Fatal("expected original header unchanged")
	}
}

func TestLimitedReadAll(t *testing.T) {
	data := "hello world"
	r := strings.NewReader(data)
	result, err := limitedReadAll(r, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != data {
		t.Fatalf("expected %q, got %q", data, result)
	}

	r2 := strings.NewReader("too long")
	_, err = limitedReadAll(r2, 3)
	if err == nil {
		t.Fatal("expected error for exceeding limit")
	}
}

func TestTruncateString(t *testing.T) {
	if got := truncateString("hi", 10); got != "hi" {
		t.Fatalf("expected 'hi', got %q", got)
	}
	if got := truncateString("hello world", 8); got != "hello..." {
		t.Fatalf("expected 'hello...', got %q", got)
	}
	if got := truncateString("hello", 3); got != "hel" {
		t.Fatalf("expected 'hel' (maxLen<=3 no ellipsis), got %q", got)
	}
	if got := truncateString("hello", 5); got != "hello" {
		t.Fatalf("expected 'hello', got %q", got)
	}
}

func TestNormalizeHost(t *testing.T) {
	tests := []struct {
		rawURL string
		want   string
	}{
		{"https://example.com", "example.com"},
		{"https://example.com:443", "example.com"},
		{"http://example.com:80", "example.com"},
		{"https://example.com:8080", "example.com:8080"},
	}
	for _, tc := range tests {
		t.Run(tc.rawURL, func(t *testing.T) {
			u, _ := url.Parse(tc.rawURL)
			got := normalizeHost(u)
			if got != tc.want {
				t.Fatalf("normalizeHost(%q) = %q, want %q", tc.rawURL, got, tc.want)
			}
		})
	}
}
