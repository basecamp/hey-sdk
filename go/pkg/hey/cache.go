package hey

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Cache provides ETag-based HTTP caching.
type Cache struct {
	dir string
	mu  sync.RWMutex
}

// NewCache creates a new cache with the given directory.
func NewCache(dir string) *Cache {
	return &Cache{dir: dir}
}

// Key generates a cache key for a URL and authorization value. Account-scoped
// request URLs carry their account filter in the query, so each linked account
// has distinct cache entries.
func (c *Cache) Key(url, token string) string {
	tokenHash := ""
	if token != "" {
		h := sha256.Sum256([]byte(token))
		tokenHash = hex.EncodeToString(h[:8])
	}

	input := url + ":" + tokenHash
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])
}

// GetETag returns the cached ETag for a key, or empty string if not found.
func (c *Cache) GetETag(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	etagsFile := filepath.Join(c.dir, "etags.json")
	data, err := os.ReadFile(etagsFile)
	if err != nil {
		return ""
	}

	var etags map[string]string
	if err := json.Unmarshal(data, &etags); err != nil {
		return ""
	}

	return etags[key]
}

// GetBody returns the cached response body for a key, or nil if not found.
func (c *Cache) GetBody(key string) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	bodyFile := filepath.Join(c.dir, "responses", key+".body")
	data, err := os.ReadFile(bodyFile)
	if err != nil {
		return nil
	}
	return data
}

// GetHeaders returns the response headers stored with a key's body, or nil when there are
// none: an entry an SDK older than header storage wrote, which can answer a 304 with its
// body but not with the Link and X-Total-Count it came with.
func (c *Cache) GetHeaders(key string) http.Header {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, err := os.ReadFile(c.headersFile(key))
	if err != nil {
		return nil
	}
	var stored map[string][]string
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil
	}
	// Another SDK sharing the directory may write the names in another case.
	header := make(http.Header, len(stored))
	for name, values := range stored {
		for _, value := range values {
			header.Add(name, value)
		}
	}
	return header
}

// Set stores a response body and ETag for a key, with no headers beside them.
func (c *Cache) Set(key string, body []byte, etag string) error {
	return c.SetResponse(key, body, etag, http.Header{})
}

// SetResponse stores a response body, its ETag and the headers it came with for a key, so a
// 304 can be answered with the page as it was first read: a 304 need not repeat the Link
// and X-Total-Count of the 200 it confirms (RFC 9111 §4.3.4). Credentials and the fields
// RFC 9111 §3.1 keeps out of a cache are left out.
func (c *Cache) SetResponse(key string, body []byte, etag string, header http.Header) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	responsesDir := filepath.Join(c.dir, "responses")
	if err := os.MkdirAll(responsesDir, 0700); err != nil {
		return err
	}

	bodyFile := filepath.Join(responsesDir, key+".body")
	if err := writeFileAtomically(bodyFile, body); err != nil {
		return err
	}

	headers, err := json.MarshalIndent(storableHeaders(header), "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomically(c.headersFile(key), headers); err != nil {
		return err
	}

	etagsFile := filepath.Join(c.dir, "etags.json")
	etags := make(map[string]string)

	if data, err := os.ReadFile(etagsFile); err == nil {
		_ = json.Unmarshal(data, &etags)
	}

	etags[key] = etag

	data, err := json.MarshalIndent(etags, "", "  ")
	if err != nil {
		return err
	}

	return writeFileAtomically(etagsFile, data)
}

func (c *Cache) headersFile(key string) string {
	return filepath.Join(c.dir, "responses", key+".headers")
}

func writeFileAtomically(path string, data []byte) error {
	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmpFile, path); err != nil {
		_ = os.Remove(tmpFile)
		return err
	}
	return nil
}

// unstoredHeaders are the fields a cache entry never keeps: credentials, which have no place
// on disk, and the connection's own fields, which RFC 9111 §3.1 keeps out of a cache.
var unstoredHeaders = []string{
	"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie", "X-CSRF-Token",
	"Connection", "Keep-Alive", "Proxy-Connection", "TE", "Transfer-Encoding", "Upgrade",
}

// storableHeaders is what of a response's headers a cache entry keeps beside its body.
func storableHeaders(header http.Header) http.Header {
	stored := header.Clone()
	if stored == nil {
		stored = http.Header{}
	}
	for _, name := range header.Values("Connection") {
		for _, listed := range strings.Split(name, ",") {
			stored.Del(strings.TrimSpace(listed))
		}
	}
	for _, name := range unstoredHeaders {
		stored.Del(name)
	}
	return stored
}

// revalidatedHeaders is what a 304 answers with: the headers stored with the body, each
// field the 304 carried replacing the stored one of that name, as RFC 9111 §4.3.4 has a
// cache update its entry. Content-Length describes the 304's own empty body, not the
// stored one, so it is not taken (RFC 9111 §3.2).
func revalidatedHeaders(stored, notModified http.Header) http.Header {
	merged := stored.Clone()
	if merged == nil {
		merged = http.Header{}
	}
	for name, values := range storableHeaders(notModified) {
		if name == "Content-Length" {
			continue
		}
		merged[name] = append([]string(nil), values...)
	}
	return merged
}

// Clear removes all cached data.
func (c *Cache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	responsesDir := filepath.Join(c.dir, "responses")
	if err := os.RemoveAll(responsesDir); err != nil {
		return err
	}

	etagsFile := filepath.Join(c.dir, "etags.json")
	_ = os.Remove(etagsFile)

	return nil
}

// Invalidate removes cached data for a specific key.
func (c *Cache) Invalidate(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	bodyFile := filepath.Join(c.dir, "responses", key+".body")
	_ = os.Remove(bodyFile)
	_ = os.Remove(c.headersFile(key))

	etagsFile := filepath.Join(c.dir, "etags.json")
	etags := make(map[string]string)

	if data, err := os.ReadFile(etagsFile); err == nil {
		_ = json.Unmarshal(data, &etags)
	}

	delete(etags, key)

	data, err := json.MarshalIndent(etags, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(etagsFile, data, 0600)
}
