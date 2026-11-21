package hey

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

func TestGetTrashPagePreservesCursor(t *testing.T) {
	for _, next := range []string{"opaque-next", ""} {
		t.Run(next, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/topics/trash.json" || r.URL.Query().Get("page") != "opaque-current" {
					t.Errorf("unexpected request: %s", r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				if next != "" {
					w.Header().Set("Link", `<https://app.hey.com/topics/trash.json?page=`+next+`>; rel="next"`)
				}
				_, _ = w.Write([]byte(`{"title":"Trash","topics":[{"id":42,"name":"Conference travel plans"}]}`))
			}))
			defer srv.Close()
			client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))
			cursor := "opaque-current"
			page, err := client.Topics().GetTrashPage(context.Background(), &generated.GetTrashTopicsParams{Page: &cursor})
			if err != nil {
				t.Fatal(err)
			}
			if page.NextPage != next || page.Topics == nil || len(page.Topics.Topics) != 1 || page.Topics.Topics[0].Id != 42 {
				t.Fatalf("wrong page: %+v", page)
			}
		})
	}
}

func TestGetTrashPageReturnsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer srv.Close()
	client := NewClient(&Config{BaseURL: srv.URL}, &StaticTokenProvider{Token: "test-token"}, WithMaxRetries(0))
	page, err := client.Topics().GetTrashPage(context.Background(), nil)
	if err == nil || page != nil {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}
