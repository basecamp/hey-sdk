package hey

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientForAccountIsImmutable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":1,"accounts":[{"id":11,"status":"active"},{"id":22,"status":"active"},{"id":33,"status":"active"}]}`)
	}))
	t.Cleanup(server.Close)
	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"})
	first := mustForAccount(t, root, 11)
	second := mustForAccount(t, root, 22)

	if _, ok := root.AccountID(); ok {
		t.Fatal("root client unexpectedly has an account")
	}
	if got, ok := first.AccountID(); !ok || got != 11 {
		t.Fatalf("first AccountID = %d, %v, want 11, true", got, ok)
	}
	if got, ok := second.AccountID(); !ok || got != 22 {
		t.Fatalf("second AccountID = %d, %v, want 22, true", got, ok)
	}
	if first.httpClient == root.httpClient || second.httpClient == root.httpClient {
		t.Fatal("account-scoped clients should own cookie-filtering HTTP clients")
	}
	if first.clientShared != root.clientShared || second.clientShared != root.clientShared {
		t.Fatal("account-scoped clients should share root client resources")
	}
	if first.Boxes() == root.Boxes() || first.Boxes() == second.Boxes() {
		t.Fatal("account-scoped clients should own their service instances")
	}

	replaced := mustForAccount(t, first, 33)
	if got, _ := first.AccountID(); got != 11 {
		t.Fatalf("deriving another scope mutated first client to %d", got)
	}
	if got, _ := replaced.AccountID(); got != 33 {
		t.Fatalf("replacement AccountID = %d, want 33", got)
	}
}

func TestClientForAccountSeedsAccountIdentityState(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":1,
			"accounts":[{"id":2,"status":"active"}],
			"all_users":[{"id":202,"account_id":2}],
			"senders":[{"id":22,"account_id":2}]
		}`)
	}))
	t.Cleanup(server.Close)
	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"})
	client := mustForAccount(t, root, 2)

	if got, err := client.DefaultSenderID(context.Background()); err != nil || got != 22 {
		t.Fatalf("DefaultSenderID = %d, %v", got, err)
	}
	if got, err := client.AccountUserID(context.Background()); err != nil || got != 202 {
		t.Fatalf("AccountUserID = %d, %v", got, err)
	}
	if requests.Load() != 1 {
		t.Fatalf("identity requests = %d, want 1", requests.Load())
	}
}

func TestClientForAccountRejectsInvalidIDs(t *testing.T) {
	root := NewClient(&Config{BaseURL: "http://localhost:3000"}, &StaticTokenProvider{Token: "token"})
	for _, accountID := range []int64{0, -1} {
		t.Run(strconv.FormatInt(accountID, 10), func(t *testing.T) {
			_, err := root.ForAccount(context.Background(), accountID)
			if sdkErr, ok := err.(*Error); !ok || sdkErr.Code != CodeUsage {
				t.Fatalf("ForAccount(%d) error = %v, want usage", accountID, err)
			}
		})
	}
}

func TestClientForAccountRejectsUnknownAndInaccessibleAccounts(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":1,"accounts":[
			{"id":1,"status":"active","purpose":"home"},
			{"id":2,"status":"canceled","purpose":"home"},
			{"id":3,"status":"inactive","purpose":"home"},
			{"id":4,"status":"inactive","purpose":"work"}
		]}`)
	}))
	t.Cleanup(server.Close)
	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"})

	for _, accountID := range []int64{2, 3, 99} {
		_, err := root.ForAccount(context.Background(), accountID)
		if sdkErr, ok := err.(*Error); !ok || sdkErr.Code != CodeNotFound {
			t.Errorf("ForAccount(%d) error = %v, want not_found", accountID, err)
		}
	}
	if _, err := root.ForAccount(context.Background(), 4); err != nil {
		t.Fatalf("inactive work account should remain accessible: %v", err)
	}
	if requests.Load() != 4 {
		t.Fatalf("identity requests = %d, want 4", requests.Load())
	}
}

func TestAccountScopeRemovesConflictingFilterCookie(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	var mailCookies []*http.Cookie
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/identity.json" {
			_, _ = io.WriteString(w, `{"id":1,"accounts":[{"id":2,"status":"active"}]}`)
			return
		}
		mailCookies = r.Cookies()
		if got := r.URL.Query().Get(filteredAccountIDParameter); got != "2" {
			t.Errorf("query account = %q, want 2", got)
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(serverURL, []*http.Cookie{
		{Name: "session_token", Value: "session"},
		{Name: filteredAccountIDParameter, Value: "1"},
	})
	root := NewClient(
		&Config{BaseURL: server.URL},
		&StaticTokenProvider{Token: "token"},
		WithHTTPClient(&http.Client{Jar: jar}),
	)
	client := mustForAccount(t, root, 2)
	if _, err := client.Get(context.Background(), "/mail.json"); err != nil {
		t.Fatal(err)
	}
	cookies := map[string]string{}
	for _, cookie := range mailCookies {
		cookies[cookie.Name] = cookie.Value
	}
	if cookies["session_token"] != "session" {
		t.Errorf("session cookie = %q", cookies["session_token"])
	}
	if _, present := cookies[filteredAccountIDParameter]; present {
		t.Errorf("filter cookie reached scoped request: %v", cookies)
	}
}

func TestAccountScopeGeneratedAndDocumentRequests(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if got := r.URL.Query()[filteredAccountIDParameter]; len(got) != 1 || got[0] != "42" {
			t.Errorf("%s query values = %v, want [42]", r.URL.Path, got)
		}
		if got := r.URL.Query().Get("keep"); r.URL.Path == "/manual.json" && got != "yes" {
			t.Errorf("keep = %q, want yes", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/boxes.json":
			_, _ = io.WriteString(w, `[]`)
		case "/manual.json":
			_, _ = io.WriteString(w, `{"ok":true}`)
		case "/page":
			if got := r.Header.Get("Accept"); got != "text/html" {
				t.Errorf("Accept = %q, want text/html", got)
			}
			_, _ = io.WriteString(w, `<h1>HEY</h1>`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"}, WithMaxRetries(0))
	client := scopedTestClient(root, 42)
	if _, err := client.Boxes().List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(context.Background(), "/manual.json?keep=yes&filtered_account_id=7"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetHTML(context.Background(), "/page"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 3 {
		t.Fatalf("requests = %d, want 3", requests.Load())
	}
}

func TestAllAccountsClientLeavesRequestsUnchanged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, present := r.URL.Query()[filteredAccountIDParameter]; present {
			t.Errorf("unscoped request query = %v", r.URL.Query())
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	client := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"}, WithMaxRetries(0))
	if _, err := client.Get(context.Background(), "/manual.json?keep=yes"); err != nil {
		t.Fatal(err)
	}
}

func TestAccountScopeFormAndMultipartRequests(t *testing.T) {
	var paths []string
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get(filteredAccountIDParameter); got != "42" {
			t.Errorf("%s account = %q, want 42", r.URL.Path, got)
		}
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		http.Redirect(w, r, "/done", http.StatusSeeOther)
	}))
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"}, WithMaxRetries(0))
	client := scopedTestClient(root, 42)
	if _, err := client.PostForm(context.Background(), "/form", url.Values{"name": {"Jane"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PostMultipart(context.Background(), "/multipart", "multipart/form-data; boundary=test", []byte("--test--")); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if fmt.Sprint(paths) != "[/form /multipart]" {
		t.Fatalf("paths = %v", paths)
	}
}

func TestAccountScopeRetriesAndHooksUseFinalURL(t *testing.T) {
	var requests atomic.Int32
	hooks := &accountScopeRecordingHooks{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get(filteredAccountIDParameter); got != "42" {
			t.Errorf("account = %q, want 42", got)
		}
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	root := NewClient(
		&Config{BaseURL: server.URL},
		&StaticTokenProvider{Token: "token"},
		WithMaxRetries(1),
		WithBaseDelay(time.Millisecond),
		WithMaxJitter(time.Nanosecond),
		WithHooks(hooks),
	)
	client := scopedTestClient(root, 42)
	if _, err := client.Get(context.Background(), "/retry.json"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
	for _, rawURL := range hooks.requestURLs() {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		if got := parsed.Query().Get(filteredAccountIDParameter); got != "42" {
			t.Errorf("hook URL %q account = %q", rawURL, got)
		}
	}
	if got := hooks.retryURL(); got == "" {
		t.Fatal("retry hook did not receive a URL")
	} else if parsed, _ := url.Parse(got); parsed.Query().Get(filteredAccountIDParameter) != "42" {
		t.Errorf("retry URL = %q", got)
	}
}

func TestAccountScopeGeneratedRetriesRetainFilter(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get(filteredAccountIDParameter); got != "42" {
			t.Errorf("account = %q, want 42", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"})
	client := scopedTestClient(root, 42)
	if _, err := client.Boxes().List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
}

func TestAccountScopePaginationReappliesFilter(t *testing.T) {
	var pages []string
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get(filteredAccountIDParameter); got != "42" {
			t.Errorf("account = %q, want 42", got)
		}
		page := r.URL.Query().Get("page")
		mu.Lock()
		pages = append(pages, page)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if page == "" {
			w.Header().Set("Link", fmt.Sprintf(`<%s/items.json?page=2>; rel="next"`, serverURLFromRequest(r)))
			_, _ = io.WriteString(w, `[{"id":1}]`)
			return
		}
		_, _ = io.WriteString(w, `[{"id":2}]`)
	}))
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"}, WithMaxRetries(0))
	client := scopedTestClient(root, 42)
	items, err := client.GetAll(context.Background(), "/items.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	mu.Lock()
	defer mu.Unlock()
	if fmt.Sprint(pages) != "[ 2]" {
		t.Fatalf("pages = %v", pages)
	}
}

func TestAccountScopeDoesNotReachCrossOriginUpload(t *testing.T) {
	var storageQuery url.Values
	var storageAuthorization string
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		storageQuery = r.URL.Query()
		storageAuthorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(storage.Close)

	hey := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get(filteredAccountIDParameter); got != "42" {
			t.Errorf("HEY account = %q, want 42", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"signed_id":       "signed-123",
			"attachable_sgid": "sgid-123",
			"direct_upload": map[string]any{
				"url":     storage.URL + "/signed-upload?signature=abc",
				"headers": map[string]string{"Content-Type": "text/plain"},
			},
		})
	}))
	t.Cleanup(hey.Close)

	root := NewClient(&Config{BaseURL: hey.URL}, &StaticTokenProvider{Token: "token"}, WithMaxRetries(0))
	client := scopedTestClient(root, 42)
	if _, err := client.Attachments().Upload(context.Background(), "notes.txt", "text/plain", bytes.NewReader([]byte("hello"))); err != nil {
		t.Fatal(err)
	}
	if got := storageQuery.Get("signature"); got != "abc" {
		t.Errorf("signature = %q", got)
	}
	if _, present := storageQuery[filteredAccountIDParameter]; present {
		t.Errorf("storage query includes account scope: %v", storageQuery)
	}
	if storageAuthorization != "" {
		t.Errorf("storage Authorization = %q", storageAuthorization)
	}
}

func TestAccountScopeDoesNotReachCrossOriginDownloadRedirect(t *testing.T) {
	var targetQuery url.Values
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetQuery = r.URL.Query()
		_, _ = io.WriteString(w, "download")
	}))
	t.Cleanup(target.Close)

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get(filteredAccountIDParameter); got != "42" {
			t.Errorf("HEY account = %q, want 42", got)
		}
		http.Redirect(w, r, target.URL+"/signed-download?signature=abc", http.StatusFound)
	}))
	t.Cleanup(source.Close)

	root := NewClient(&Config{BaseURL: source.URL}, &StaticTokenProvider{Token: "token"}, WithMaxRetries(0))
	client := scopedTestClient(root, 42)
	if _, err := client.GetBlob(context.Background(), "/blob"); err != nil {
		t.Fatal(err)
	}
	if got := targetQuery.Get("signature"); got != "abc" {
		t.Errorf("signature = %q", got)
	}
	if _, present := targetQuery[filteredAccountIDParameter]; present {
		t.Errorf("download query includes account scope: %v", targetQuery)
	}
}

func TestAccountScopeSeparatesSharedCacheKeys(t *testing.T) {
	var mu sync.Mutex
	conditional := map[string][]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accountID := r.URL.Query().Get(filteredAccountIDParameter)
		mu.Lock()
		conditional[accountID] = append(conditional[accountID], r.Header.Get("If-None-Match"))
		mu.Unlock()
		etag := `"account-` + accountID + `"`
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		_, _ = io.WriteString(w, `{"account_id":`+accountID+`}`)
	}))
	t.Cleanup(server.Close)

	root := NewClient(
		&Config{BaseURL: server.URL},
		&StaticTokenProvider{Token: "token"},
		WithMaxRetries(0),
		WithCache(NewCache(t.TempDir())),
	)
	for _, client := range []*Client{scopedTestClient(root, 1), scopedTestClient(root, 2), scopedTestClient(root, 1), scopedTestClient(root, 2)} {
		if _, err := client.Get(context.Background(), "/cached.json"); err != nil {
			t.Fatal(err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	for accountID, requests := range conditional {
		want := []string{"", `"account-` + accountID + `"`}
		if fmt.Sprint(requests) != fmt.Sprint(want) {
			t.Errorf("account %s conditionals = %v, want %v", accountID, requests, want)
		}
	}
}

func TestDefaultSenderIDUsesSelectedAccountAndSeparateCaches(t *testing.T) {
	var identityRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identityRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id": 1,
			"primary_contact": {"id": 10},
			"accounts": [{"id": 1, "status": "active"}, {"id": 2, "status": "active"}],
			"senders": [
				{"id": 11, "account_id": 1, "default": true},
				{"id": 21, "account_id": 2},
				{"id": 22, "account_id": 2, "default": true}
			]
		}`)
	}))
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"}, WithMaxRetries(0))
	first := mustForAccount(t, root, 1)
	second := mustForAccount(t, root, 2)

	for i := 0; i < 2; i++ {
		if got, err := first.DefaultSenderID(context.Background()); err != nil || got != 11 {
			t.Fatalf("first sender = %d, %v", got, err)
		}
		if got, err := second.DefaultSenderID(context.Background()); err != nil || got != 22 {
			t.Fatalf("second sender = %d, %v", got, err)
		}
	}
	if got, err := root.DefaultSenderID(context.Background()); err != nil || got != 11 {
		t.Fatalf("root sender = %d, %v", got, err)
	}
	if identityRequests.Load() != 3 {
		t.Fatalf("identity requests = %d, want 3 cached independently", identityRequests.Load())
	}
}

func TestAccountScopedClientsDoNotExchangeConcurrentSenderState(t *testing.T) {
	var mu sync.Mutex
	requests := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accountID := r.URL.Query().Get(filteredAccountIDParameter)
		mu.Lock()
		requests[accountID]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":1,"senders":[{"id":%s0,"account_id":%s,"default":true}]}`, accountID, accountID)
	}))
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"}, WithMaxRetries(0))
	clients := []struct {
		client *Client
		want   int64
	}{{scopedTestClient(root, 1), 10}, {scopedTestClient(root, 2), 20}}

	var wg sync.WaitGroup
	for _, test := range clients {
		for range 20 {
			wg.Add(1)
			go func(client *Client, want int64) {
				defer wg.Done()
				if got, err := client.DefaultSenderID(context.Background()); err != nil || got != want {
					t.Errorf("sender = %d, %v, want %d", got, err, want)
				}
			}(test.client, test.want)
		}
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if requests["1"] != 1 || requests["2"] != 1 {
		t.Fatalf("identity requests = %v, want one per account", requests)
	}
}

func TestDefaultSenderIDDoesNotCrossAccountFallback(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":1,"accounts":[{"id":2,"status":"active"}],"primary_contact":{"id":10},"senders":[{"id":11,"account_id":1,"default":true}]}`)
	}))
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"}, WithMaxRetries(0))
	client := mustForAccount(t, root, 2)
	for i := 0; i < 2; i++ {
		_, err := client.DefaultSenderID(context.Background())
		if sdkErr, ok := err.(*Error); !ok || sdkErr.Code != CodeNotFound {
			t.Fatalf("error = %v, want not_found", err)
		}
	}
	if requests.Load() != 3 {
		t.Fatalf("failed sender lookups were cached; requests = %d", requests.Load())
	}
}

func TestScopedMessageAndReplyUseAccountSender(t *testing.T) {
	var mu sync.Mutex
	actingSenders := map[string]int64{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get(filteredAccountIDParameter); got != "2" {
			t.Errorf("%s account = %q, want 2", r.URL.Path, got)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/identity.json" {
			_, _ = io.WriteString(w, `{"id":1,"senders":[{"id":11,"account_id":1,"default":true},{"id":22,"account_id":2}]}`)
			return
		}
		var body struct {
			ActingSenderID int64 `json:"acting_sender_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		mu.Lock()
		actingSenders[r.URL.Path] = body.ActingSenderID
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"}, WithMaxRetries(0))
	client := scopedTestClient(root, 2)
	if err := client.Messages().Create(context.Background(), "Subject", "Body", []string{"jane@example.com"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := client.Entries().CreateReply(context.Background(), 99, "Reply", []string{"jane@example.com"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if actingSenders["/messages.json"] != 22 || actingSenders["/entries/99/replies.json"] != 22 {
		t.Fatalf("acting senders = %v, want account sender 22", actingSenders)
	}
}

func TestAccountUserIDUsesSelectedAccountAndCaches(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":1,"all_users":[{"id":101,"account_id":1},{"id":202,"account_id":2}]}`)
	}))
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"}, WithMaxRetries(0))
	client := scopedTestClient(root, 2)
	for i := 0; i < 2; i++ {
		if got, err := client.AccountUserID(context.Background()); err != nil || got != 202 {
			t.Fatalf("AccountUserID = %d, %v", got, err)
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("identity requests = %d, want 1", requests.Load())
	}
}

func TestAccountUserIDRequiresMatchingScope(t *testing.T) {
	root := NewClient(&Config{BaseURL: "http://localhost:3000"}, &StaticTokenProvider{Token: "token"})
	if _, err := root.AccountUserID(context.Background()); err == nil {
		t.Fatal("All Accounts client AccountUserID returned no error")
	}

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":1,"all_users":[{"id":101,"account_id":1}]}`)
	}))
	t.Cleanup(server.Close)
	scopedRoot := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"}, WithMaxRetries(0))
	client := scopedTestClient(scopedRoot, 2)
	for i := 0; i < 2; i++ {
		_, err := client.AccountUserID(context.Background())
		if sdkErr, ok := err.(*Error); !ok || sdkErr.Code != CodeNotFound {
			t.Fatalf("error = %v, want not_found", err)
		}
	}
	if requests.Load() != 2 {
		t.Fatalf("failed user lookups were cached; requests = %d", requests.Load())
	}
}

func TestScopedContactCreateResolvesAndValidatesAccountUser(t *testing.T) {
	var contactRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get(filteredAccountIDParameter); got != "2" {
			t.Errorf("%s account = %q, want 2", r.URL.Path, got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/identity.json":
			_, _ = io.WriteString(w, `{"id":1,"all_users":[{"id":101,"account_id":1},{"id":202,"account_id":2}]}`)
		case "/contacts.json":
			contactRequests.Add(1)
			var body struct {
				ActingUserID int64 `json:"acting_user_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.ActingUserID != 202 {
				t.Errorf("acting_user_id = %d, want 202", body.ActingUserID)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":55,"name":"Jane","email_address":"jane@example.com"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	root := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "token"}, WithMaxRetries(0))
	client := scopedTestClient(root, 2)
	contact, err := client.Contacts().Create(context.Background(), ContactParams{Name: "Jane", EmailAddress: "jane@example.com"})
	if err != nil || contact == nil || contact.Id != 55 {
		t.Fatalf("Create = %#v, %v", contact, err)
	}
	_, err = client.Contacts().Create(context.Background(), ContactParams{
		Name: "John", EmailAddress: "john@example.com", AccountUserID: 101,
	})
	if sdkErr, ok := err.(*Error); !ok || sdkErr.Code != CodeUsage {
		t.Fatalf("mismatched account user error = %v, want usage", err)
	}
	if contactRequests.Load() != 1 {
		t.Fatalf("contact requests = %d, want 1", contactRequests.Load())
	}
}

func TestSeparateIdentityClientsDoNotShareCachedResponses(t *testing.T) {
	var mu sync.Mutex
	conditionals := map[string][]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		mu.Lock()
		conditionals[token] = append(conditionals[token], r.Header.Get("If-None-Match"))
		mu.Unlock()
		etag := `"` + token + `"`
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		_, _ = fmt.Fprintf(w, `{"token":%q}`, token)
	}))
	t.Cleanup(server.Close)

	cache := NewCache(t.TempDir())
	personalRoot := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "personal-token"}, WithMaxRetries(0), WithCache(cache))
	workRoot := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "work-token"}, WithMaxRetries(0), WithCache(cache))
	personal := scopedTestClient(personalRoot, 1)
	work := scopedTestClient(workRoot, 1)
	for _, client := range []*Client{personal, work, personal, work} {
		if _, err := client.Get(context.Background(), "/cached.json"); err != nil {
			t.Fatal(err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	for token, requests := range conditionals {
		want := []string{"", `"` + token + `"`}
		if fmt.Sprint(requests) != fmt.Sprint(want) {
			t.Errorf("%s conditionals = %v, want %v", token, requests, want)
		}
	}
}

func TestCustomAuthenticationWithoutAuthorizationDisablesResponseCache(t *testing.T) {
	var mu sync.Mutex
	conditionals := map[string][]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity := r.Header.Get("X-Test-Identity")
		mu.Lock()
		conditionals[identity] = append(conditionals[identity], r.Header.Get("If-None-Match"))
		mu.Unlock()
		w.Header().Set("ETag", `"shared-url"`)
		_, _ = fmt.Fprintf(w, `{"identity":%q}`, identity)
	}))
	t.Cleanup(server.Close)

	cache := NewCache(t.TempDir())
	personal := NewClient(
		&Config{BaseURL: server.URL}, nil,
		WithAuthStrategy(testHeaderAuth{identity: "personal"}),
		WithMaxRetries(0), WithCache(cache),
	)
	work := NewClient(
		&Config{BaseURL: server.URL}, nil,
		WithAuthStrategy(testHeaderAuth{identity: "work"}),
		WithMaxRetries(0), WithCache(cache),
	)
	for _, client := range []*Client{personal, work, personal, work} {
		resp, err := client.Get(context.Background(), "/cached.json")
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Identity string `json:"identity"`
		}
		if err := resp.UnmarshalData(&body); err != nil {
			t.Fatal(err)
		}
		if body.Identity == "" {
			t.Fatal("response lost custom identity")
		}
	}

	mu.Lock()
	defer mu.Unlock()
	for identity, requests := range conditionals {
		if fmt.Sprint(requests) != "[ ]" {
			t.Errorf("%s conditionals = %v, want caching disabled", identity, requests)
		}
	}
}

func TestSeparateIdentityClientsDoNotShareAuthenticationOrSenderState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		accountID := r.URL.Query().Get(filteredAccountIDParameter)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/identity.json" {
			senderID := int64(101)
			if token == "Bearer work-token" {
				senderID = 202
			}
			_, _ = fmt.Fprintf(w, `{"id":1,"senders":[{"id":%d,"account_id":%s,"default":true}]}`, senderID, accountID)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	personalRoot := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "personal-token"}, WithMaxRetries(0))
	workRoot := NewClient(&Config{BaseURL: server.URL}, &StaticTokenProvider{Token: "work-token"}, WithMaxRetries(0))
	personal := scopedTestClient(personalRoot, 1)
	work := scopedTestClient(workRoot, 2)

	var wg sync.WaitGroup
	for _, test := range []struct {
		client *Client
		want   int64
	}{{personal, 101}, {work, 202}} {
		for range 10 {
			wg.Add(1)
			go func(client *Client, want int64) {
				defer wg.Done()
				if got, err := client.DefaultSenderID(context.Background()); err != nil || got != want {
					t.Errorf("sender = %d, %v, want %d", got, err, want)
				}
			}(test.client, test.want)
		}
	}
	wg.Wait()
}

type testHeaderAuth struct {
	identity string
}

func (a testHeaderAuth) Authenticate(_ context.Context, req *http.Request) error {
	req.Header.Set("X-Test-Identity", a.identity)
	return nil
}

type accountScopeRecordingHooks struct {
	NoopHooks
	mu       sync.Mutex
	requests []string
	retry    string
}

func (h *accountScopeRecordingHooks) OnRequestStart(ctx context.Context, info RequestInfo) context.Context {
	h.mu.Lock()
	h.requests = append(h.requests, info.URL)
	h.mu.Unlock()
	return ctx
}

func (h *accountScopeRecordingHooks) OnRetry(_ context.Context, info RequestInfo, _ int, _ error) {
	h.mu.Lock()
	h.retry = info.URL
	h.mu.Unlock()
}

func (h *accountScopeRecordingHooks) requestURLs() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.requests...)
}

func (h *accountScopeRecordingHooks) retryURL() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.retry
}

func serverURLFromRequest(r *http.Request) string {
	return "http://" + r.Host
}

func mustForAccount(t *testing.T, client *Client, accountID int64) *Client {
	t.Helper()
	scoped, err := client.ForAccount(context.Background(), accountID)
	if err != nil {
		t.Fatalf("ForAccount(%d): %v", accountID, err)
	}
	return scoped
}

func scopedTestClient(root *Client, accountID int64) *Client {
	return &Client{
		clientShared: root.clientShared,
		httpClient:   accountScopedHTTPClient(root.httpClient),
		accountID:    accountID,
	}
}
