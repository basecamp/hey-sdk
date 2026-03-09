package hey

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.BaseURL != "https://app.hey.com" {
		t.Fatalf("expected default base URL, got %q", cfg.BaseURL)
	}
	if cfg.OAuthClientID == "" {
		t.Fatal("expected non-empty OAuth client ID")
	}
	if cfg.CacheEnabled {
		t.Fatal("expected cache disabled by default")
	}
}

func TestLoadConfig_NonExistentFile(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if cfg.BaseURL != "https://app.hey.com" {
		t.Fatalf("expected defaults, got %q", cfg.BaseURL)
	}
}

func TestLoadConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.json")

	data, _ := json.Marshal(map[string]any{
		"base_url":      "https://custom.hey.com",
		"cache_enabled": true,
	})
	if err := os.WriteFile(cfgFile, data, 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseURL != "https://custom.hey.com" {
		t.Fatalf("expected custom base URL, got %q", cfg.BaseURL)
	}
	if !cfg.CacheEnabled {
		t.Fatal("expected cache enabled")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgFile, []byte("{bad json"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(cfgFile)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	cfg := DefaultConfig()

	t.Setenv("HEY_BASE_URL", "https://env.hey.com")
	t.Setenv("HEY_OAUTH_CLIENT_ID", "env-client-id")
	t.Setenv("HEY_CACHE_DIR", "/tmp/hey-cache")
	t.Setenv("HEY_CACHE_ENABLED", "true")

	cfg.LoadConfigFromEnv()

	if cfg.BaseURL != "https://env.hey.com" {
		t.Fatalf("expected env base URL, got %q", cfg.BaseURL)
	}
	if cfg.OAuthClientID != "env-client-id" {
		t.Fatalf("expected env client ID, got %q", cfg.OAuthClientID)
	}
	if cfg.CacheDir != "/tmp/hey-cache" {
		t.Fatalf("expected env cache dir, got %q", cfg.CacheDir)
	}
	if !cfg.CacheEnabled {
		t.Fatal("expected cache enabled from env")
	}
}

func TestLoadConfigFromEnv_CacheEnabled_Values(t *testing.T) {
	cfg := DefaultConfig()

	t.Setenv("HEY_CACHE_ENABLED", "1")
	cfg.LoadConfigFromEnv()
	if !cfg.CacheEnabled {
		t.Fatal("expected '1' to enable cache")
	}

	cfg.CacheEnabled = true
	t.Setenv("HEY_CACHE_ENABLED", "false")
	cfg.LoadConfigFromEnv()
	if cfg.CacheEnabled {
		t.Fatal("expected 'false' to disable cache")
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	if got := NormalizeBaseURL("https://example.com/"); got != "https://example.com" {
		t.Fatalf("expected trailing slash stripped, got %q", got)
	}
	if got := NormalizeBaseURL("https://example.com"); got != "https://example.com" {
		t.Fatalf("expected no change, got %q", got)
	}
}
