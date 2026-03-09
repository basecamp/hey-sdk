package hey

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetAll_SinglePage(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`[{"id":1},{"id":2}]`))
	})

	items, err := client.GetAll(context.Background(), "/items.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestGetAll_MultiplePages(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		if page == "2" {
			w.Write([]byte(`[{"id":2}]`))
		} else {
			w.Header().Set("Link", fmt.Sprintf(`<%s/items.json?page=2>; rel="next"`, serverURL))
			w.Write([]byte(`[{"id":1}]`))
		}
	}))
	defer server.Close()
	serverURL = server.URL

	cfg := &Config{BaseURL: server.URL}
	client := NewClient(cfg, &StaticTokenProvider{Token: "test-token"})

	items, err := client.GetAll(context.Background(), "/items.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items across pages, got %d", len(items))
	}
}

func TestGetAllWithLimit(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		if page == "2" {
			w.Write([]byte(`[{"id":3},{"id":4}]`))
		} else {
			w.Header().Set("Link", fmt.Sprintf(`<%s/items.json?page=2>; rel="next"`, serverURL))
			w.Write([]byte(`[{"id":1},{"id":2}]`))
		}
	}))
	defer server.Close()
	serverURL = server.URL

	cfg := &Config{BaseURL: server.URL}
	client := NewClient(cfg, &StaticTokenProvider{Token: "test-token"})

	items, err := client.GetAllWithLimit(context.Background(), "/items.json", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items (limited), got %d", len(items))
	}
}

func TestGetAll_CrossOriginRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", `<https://evil.com/steal>; rel="next"`)
		w.Write([]byte(`[{"id":1}]`))
	}))
	defer server.Close()

	cfg := &Config{BaseURL: server.URL}
	client := NewClient(cfg, &StaticTokenProvider{Token: "test-token"})

	_, err := client.GetAll(context.Background(), "/items.json")
	if err == nil {
		t.Fatal("expected error for cross-origin pagination link")
	}
}

func TestGetAll_MaxPages(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Always return a next link to test maxPages
		w.Header().Set("Link", fmt.Sprintf(`<%s/items.json?page=next>; rel="next"`, serverURL))
		w.Write([]byte(`[{"id":1}]`))
	}))
	defer server.Close()
	serverURL = server.URL

	cfg := &Config{BaseURL: server.URL}
	client := NewClient(cfg, &StaticTokenProvider{Token: "test-token"}, WithMaxPages(3))

	items, err := client.GetAll(context.Background(), "/items.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items (capped at maxPages), got %d", len(items))
	}
}

func TestFollowPagination_NilResponse(t *testing.T) {
	cfg := &Config{BaseURL: "http://localhost:3000"}
	client := NewClient(cfg, &StaticTokenProvider{Token: "t"})

	items, err := client.FollowPagination(context.Background(), nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if items != nil {
		t.Fatal("expected nil for nil response")
	}
}

func TestFollowPagination_LimitAlreadyMet(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Link", `<http://example.com/page2>; rel="next"`)

	cfg := &Config{BaseURL: "http://localhost:3000"}
	client := NewClient(cfg, &StaticTokenProvider{Token: "t"})

	items, err := client.FollowPagination(context.Background(), resp, 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	if items != nil {
		t.Fatal("expected nil when limit already met")
	}
}

func TestFollowPagination_NoLinkHeader(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	cfg := &Config{BaseURL: "http://localhost:3000"}
	client := NewClient(cfg, &StaticTokenProvider{Token: "t"})

	items, err := client.FollowPagination(context.Background(), resp, 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if items != nil {
		t.Fatal("expected nil with no Link header")
	}
}

func TestFollowPagination_CrossOriginRejection(t *testing.T) {
	reqURL, _ := http.NewRequestWithContext(context.Background(), "GET", "http://localhost:3000/items.json", nil)
	resp := &http.Response{
		Header:  http.Header{},
		Request: reqURL,
	}
	resp.Header.Set("Link", `<https://evil.com/steal>; rel="next"`)

	cfg := &Config{BaseURL: "http://localhost:3000"}
	client := NewClient(cfg, &StaticTokenProvider{Token: "t"})

	_, err := client.FollowPagination(context.Background(), resp, 5, 0)
	if err == nil {
		t.Fatal("expected cross-origin rejection")
	}
}

func TestGetAll_EmptyItems(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	})

	items, err := client.GetAll(context.Background(), "/items.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestGetAll_InvalidJSON(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not json`))
	})

	_, err := client.GetAll(context.Background(), "/items.json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// Verify json.RawMessage items survive round-trip
func TestGetAll_RawMessagePreservation(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"id":1,"name":"test"},{"id":2,"nested":{"a":1}}]`))
	})

	items, err := client.GetAll(context.Background(), "/items.json")
	if err != nil {
		t.Fatal(err)
	}

	var first struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(items[0], &first); err != nil {
		t.Fatal(err)
	}
	if first.ID != 1 || first.Name != "test" {
		t.Fatalf("unexpected first item: %+v", first)
	}
}
