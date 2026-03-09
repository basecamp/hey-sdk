package hey

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStaticTokenProvider(t *testing.T) {
	p := &StaticTokenProvider{Token: "my-token"}
	token, err := p.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "my-token" {
		t.Fatalf("expected 'my-token', got %q", token)
	}
}

func TestBearerAuth(t *testing.T) {
	auth := &BearerAuth{
		TokenProvider: &StaticTokenProvider{Token: "bearer-tok"},
	}

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://example.com/api", nil)
	if err := auth.Authenticate(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := req.Header.Get("Authorization")
	if got != "Bearer bearer-tok" {
		t.Fatalf("expected 'Bearer bearer-tok', got %q", got)
	}
}

func TestCredentialStore_FileBased(t *testing.T) {
	dir := t.TempDir()

	// Force file-based storage
	t.Setenv("HEY_NO_KEYRING", "1")
	store := NewCredentialStore(dir)

	if store.UsingKeyring() {
		t.Fatal("expected file-based store")
	}

	origin := "https://app.hey.com"
	creds := &Credentials{
		AccessToken:  "access-tok",
		RefreshToken: "refresh-tok",
		ExpiresAt:    9999999999,
		Scope:        "full",
		UserID:       "user-1",
	}

	if err := store.Save(origin, creds); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Load(origin)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.AccessToken != "access-tok" {
		t.Fatalf("expected access-tok, got %q", loaded.AccessToken)
	}
	if loaded.UserID != "user-1" {
		t.Fatalf("expected user-1, got %q", loaded.UserID)
	}

	if err := store.Delete(origin); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.Load(origin)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestCredentialStore_Load_NonExistent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HEY_NO_KEYRING", "1")
	store := NewCredentialStore(dir)

	_, err := store.Load("https://nowhere.com")
	if err == nil {
		t.Fatal("expected error for non-existent credentials")
	}
}

func TestCredentialStore_MultipleOrigins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HEY_NO_KEYRING", "1")
	store := NewCredentialStore(dir)

	if err := store.Save("https://a.com", &Credentials{AccessToken: "tok-a"}); err != nil {
		t.Fatalf("Save a failed: %v", err)
	}
	if err := store.Save("https://b.com", &Credentials{AccessToken: "tok-b"}); err != nil {
		t.Fatalf("Save b failed: %v", err)
	}

	a, err := store.Load("https://a.com")
	if err != nil {
		t.Fatalf("Load a failed: %v", err)
	}
	b, err := store.Load("https://b.com")
	if err != nil {
		t.Fatalf("Load b failed: %v", err)
	}

	if a.AccessToken != "tok-a" {
		t.Fatalf("expected tok-a, got %q", a.AccessToken)
	}
	if b.AccessToken != "tok-b" {
		t.Fatalf("expected tok-b, got %q", b.AccessToken)
	}
}

func TestAuthManager_AccessToken_FromEnv(t *testing.T) {
	t.Setenv("HEY_TOKEN", "env-token")
	t.Setenv("HEY_NO_KEYRING", "1")

	cfg := DefaultConfig()
	mgr := NewAuthManager(cfg, http.DefaultClient)

	token, err := mgr.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "env-token" {
		t.Fatalf("expected env-token, got %q", token)
	}
}

func TestAuthManager_IsAuthenticated_FromEnv(t *testing.T) {
	t.Setenv("HEY_TOKEN", "env-token")
	t.Setenv("HEY_NO_KEYRING", "1")

	cfg := DefaultConfig()
	mgr := NewAuthManager(cfg, http.DefaultClient)

	if !mgr.IsAuthenticated() {
		t.Fatal("expected authenticated with HEY_TOKEN env var")
	}
}

func TestAuthManager_IsAuthenticated_NoToken(t *testing.T) {
	t.Setenv("HEY_NO_KEYRING", "1")
	// Ensure HEY_TOKEN is not set
	t.Setenv("HEY_TOKEN", "")

	cfg := DefaultConfig()
	dir := t.TempDir()
	store := &CredentialStore{useKeyring: false, fallbackDir: dir}
	mgr := NewAuthManagerWithStore(cfg, http.DefaultClient, store)

	if mgr.IsAuthenticated() {
		t.Fatal("expected not authenticated with no credentials")
	}
}

func TestAuthManager_Refresh(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"access_token":"new-token","refresh_token":"new-refresh","expires_in":3600}`))
	}))
	defer tokenServer.Close()

	dir := t.TempDir()
	t.Setenv("HEY_NO_KEYRING", "1")
	store := &CredentialStore{useKeyring: false, fallbackDir: dir}

	origin := NormalizeBaseURL("https://app.hey.com")
	store.Save(origin, &Credentials{
		AccessToken:   "old-token",
		RefreshToken:  "old-refresh",
		ExpiresAt:     1,
		TokenEndpoint: tokenServer.URL,
	})

	cfg := DefaultConfig()
	mgr := NewAuthManagerWithStore(cfg, http.DefaultClient, store)

	if err := mgr.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	creds, _ := store.Load(origin)
	if creds.AccessToken != "new-token" {
		t.Fatalf("expected new-token, got %q", creds.AccessToken)
	}
	if creds.RefreshToken != "new-refresh" {
		t.Fatalf("expected new-refresh, got %q", creds.RefreshToken)
	}
}

func TestAuthManager_SetAndGetUserID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HEY_NO_KEYRING", "1")
	store := &CredentialStore{useKeyring: false, fallbackDir: dir}

	origin := NormalizeBaseURL("https://app.hey.com")
	store.Save(origin, &Credentials{AccessToken: "tok"})

	cfg := DefaultConfig()
	mgr := NewAuthManagerWithStore(cfg, http.DefaultClient, store)

	if err := mgr.SetUserID("user-42"); err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetUserID(); got != "user-42" {
		t.Fatalf("expected user-42, got %q", got)
	}
}

func TestAuthManager_Logout(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HEY_NO_KEYRING", "1")
	store := &CredentialStore{useKeyring: false, fallbackDir: dir}

	origin := NormalizeBaseURL("https://app.hey.com")
	store.Save(origin, &Credentials{AccessToken: "tok"})

	cfg := DefaultConfig()
	mgr := NewAuthManagerWithStore(cfg, http.DefaultClient, store)

	if err := mgr.Logout(); err != nil {
		t.Fatal(err)
	}

	if mgr.IsAuthenticated() {
		t.Fatal("expected not authenticated after logout")
	}
}
