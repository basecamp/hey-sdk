package hey

import (
	"testing"
)

func TestCache_Key(t *testing.T) {
	cache := NewCache(t.TempDir())

	key1 := cache.Key("https://example.com/api", "token1")
	key2 := cache.Key("https://example.com/api", "token2")
	key3 := cache.Key("https://example.com/api", "token1")

	if key1 == key2 {
		t.Fatal("expected different keys for different tokens")
	}
	if key1 != key3 {
		t.Fatal("expected same key for same URL+token")
	}

	keyNoToken := cache.Key("https://example.com/api", "")
	if keyNoToken == key1 {
		t.Fatal("expected different key for empty token")
	}
}

func TestCache_SetAndGet(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)

	key := cache.Key("https://example.com/boxes.json", "tok")
	body := []byte(`[{"id":1}]`)
	etag := `"abc123"`

	if err := cache.Set(key, body, etag); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	gotETag := cache.GetETag(key)
	if gotETag != etag {
		t.Fatalf("expected ETag %q, got %q", etag, gotETag)
	}

	gotBody := cache.GetBody(key)
	if string(gotBody) != string(body) {
		t.Fatalf("expected body %q, got %q", body, gotBody)
	}
}

func TestCache_GetETag_Missing(t *testing.T) {
	cache := NewCache(t.TempDir())
	if etag := cache.GetETag("nonexistent"); etag != "" {
		t.Fatalf("expected empty etag, got %q", etag)
	}
}

func TestCache_GetBody_Missing(t *testing.T) {
	cache := NewCache(t.TempDir())
	if body := cache.GetBody("nonexistent"); body != nil {
		t.Fatal("expected nil body for missing key")
	}
}

func TestCache_Invalidate(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)

	key := cache.Key("https://example.com/test", "tok")
	if err := cache.Set(key, []byte(`{}`), `"etag"`); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if err := cache.Invalidate(key); err != nil {
		t.Fatalf("Invalidate failed: %v", err)
	}

	if etag := cache.GetETag(key); etag != "" {
		t.Fatalf("expected empty etag after invalidation, got %q", etag)
	}
	if body := cache.GetBody(key); body != nil {
		t.Fatal("expected nil body after invalidation")
	}
}

func TestCache_Clear(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)

	if err := cache.Set(cache.Key("u1", "t"), []byte("a"), `"e1"`); err != nil {
		t.Fatalf("Set u1 failed: %v", err)
	}
	if err := cache.Set(cache.Key("u2", "t"), []byte("b"), `"e2"`); err != nil {
		t.Fatalf("Set u2 failed: %v", err)
	}

	if err := cache.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	if body := cache.GetBody(cache.Key("u1", "t")); body != nil {
		t.Fatal("expected nil after clear")
	}
	if etag := cache.GetETag(cache.Key("u1", "t")); etag != "" {
		t.Fatalf("expected empty etag after clear, got %q", etag)
	}
}

func TestCache_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)

	key := cache.Key("https://example.com/x", "tok")
	if err := cache.Set(key, []byte(`old`), `"v1"`); err != nil {
		t.Fatalf("first Set failed: %v", err)
	}
	if err := cache.Set(key, []byte(`new`), `"v2"`); err != nil {
		t.Fatalf("second Set failed: %v", err)
	}

	if got := string(cache.GetBody(key)); got != "new" {
		t.Fatalf("expected overwritten body, got %q", got)
	}
	if got := cache.GetETag(key); got != `"v2"` {
		t.Fatalf("expected overwritten etag, got %q", got)
	}
}
