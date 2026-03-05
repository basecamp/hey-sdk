package hey

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Response body size limits.
const (
	// MaxResponseBodyBytes is the maximum size for successful API response bodies (50 MB).
	MaxResponseBodyBytes int64 = 50 * 1024 * 1024
	// MaxErrorBodyBytes is the maximum size for error response bodies (1 MB).
	MaxErrorBodyBytes int64 = 1 * 1024 * 1024
	// MaxErrorMessageBytes is the maximum length for error messages included in errors (500 bytes).
	MaxErrorMessageBytes = 500
)

// limitedReadAll reads up to maxBytes from r.
func limitedReadAll(r io.Reader, maxBytes int64) ([]byte, error) {
	lr := io.LimitReader(r, maxBytes+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("response body exceeds %d byte limit", maxBytes)
	}
	return data, nil
}

// truncateString truncates s to maxLen bytes, appending "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// requireHTTPS validates that the given URL uses the https:// scheme.
func requireHTTPS(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("URL must use HTTPS: %s", rawURL)
	}
	return nil
}

// isSameOrigin checks whether two absolute URLs share the same scheme and host.
func isSameOrigin(a, b string) bool {
	ua, err := url.Parse(a)
	if err != nil {
		return false
	}
	ub, err := url.Parse(b)
	if err != nil {
		return false
	}
	if ua.Scheme == "" || ub.Scheme == "" {
		return false
	}
	return strings.EqualFold(ua.Scheme, ub.Scheme) &&
		strings.EqualFold(normalizeHost(ua), normalizeHost(ub))
}

// resolveURL resolves a possibly-relative URL against a base URL.
func resolveURL(base, target string) string {
	bu, err := url.Parse(base)
	if err != nil {
		return target
	}
	tu, err := url.Parse(target)
	if err != nil {
		return target
	}
	return bu.ResolveReference(tu).String()
}

// normalizeHost returns the host with default ports stripped.
func normalizeHost(u *url.URL) string {
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		return host
	}
	if (strings.EqualFold(u.Scheme, "https") && port == "443") ||
		(strings.EqualFold(u.Scheme, "http") && port == "80") {
		return host
	}
	return host + ":" + port
}

// isLocalhost checks if a URL points to localhost (for test environments).
func isLocalhost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Hostname()

	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}

	if strings.HasSuffix(host, ".localhost") {
		return true
	}

	return false
}

// RequireSecureEndpoint validates that an endpoint URL is secure.
func RequireSecureEndpoint(rawURL string) error {
	if isLocalhost(rawURL) {
		return nil
	}
	return requireHTTPS(rawURL)
}

// sensitiveHeaders is the list of headers that should be redacted for logging.
var sensitiveHeaders = []string{
	"Authorization",
	"Cookie",
	"Set-Cookie",
	"X-CSRF-Token",
}

// RedactHeaders returns a copy of the headers with sensitive values replaced by "[REDACTED]".
func RedactHeaders(headers http.Header) http.Header {
	result := headers.Clone()
	for _, key := range sensitiveHeaders {
		if result.Get(key) != "" {
			result.Set(key, "[REDACTED]")
		}
	}
	return result
}
