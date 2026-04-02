package hey

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// JournalService handles journal entry operations.
type JournalService struct {
	client *Client
}

// NewJournalService creates a new JournalService.
func NewJournalService(client *Client) *JournalService {
	return &JournalService{client: client}
}

// Get returns a journal entry for a specific day (YYYY-MM-DD format).
func (s *JournalService) Get(ctx context.Context, day string) (result *generated.Recording, err error) {
	op := OperationInfo{
		Service: "Journal", Operation: "GetJournalEntry",
		ResourceType: "journal_entry", IsMutation: false,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	s.client.initGeneratedClient()
	resp, err := s.client.gen.GetJournalEntryWithResponse(ctx, day)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// GetContent returns the HTML content of a journal entry for a specific day.
// It tries the JSON API first; if that returns 204 (no content), it falls back
// to scraping the edit page for the Trix editor content.
func (s *JournalService) GetContent(ctx context.Context, day string) (content string, err error) {
	op := OperationInfo{
		Service: "Journal", Operation: "GetJournalContent",
		ResourceType: "journal_entry", IsMutation: false,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	// Try JSON API first.
	s.client.initGeneratedClient()
	resp, err := s.client.gen.GetJournalEntryWithResponse(ctx, day)
	if err != nil {
		return "", err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return "", err
	}
	if resp.JSON200 != nil && resp.JSON200.Content != "" {
		return resp.JSON200.Content, nil
	}

	// JSON API returned 204 or empty content — scrape the edit page.
	htmlResp, err := s.client.GetHTML(ctx, fmt.Sprintf("/calendar/days/%s/journal_entry/edit", day))
	if err != nil {
		return "", nil // not fatal — just no content
	}
	return extractTrixContent(htmlResp.Data)
}

// extractTrixContent extracts journal content from the edit page HTML.
// The content is stored in a Trix editor hidden input element.
func extractTrixContent(data []byte) (string, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	return findTrixInput(doc), nil
}

func findTrixInput(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "input" {
		isTarget := false
		value := ""
		for _, a := range n.Attr {
			if a.Key == "id" && strings.Contains(a.Val, "journal") && strings.HasSuffix(a.Val, "trix_input") {
				isTarget = true
			}
			if a.Key == "value" {
				value = a.Val
			}
		}
		if isTarget && value != "" {
			return value
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if v := findTrixInput(c); v != "" {
			return v
		}
	}
	return ""
}

// Update updates a journal entry for a specific day.
//
// The HEY API expects the body wrapped as {calendar_journal_entry: {content: "..."}}.
func (s *JournalService) Update(ctx context.Context, day string, content string) (err error) {
	op := OperationInfo{
		Service: "Journal", Operation: "UpdateJournalEntry",
		ResourceType: "journal_entry", IsMutation: true,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	body := map[string]any{
		"calendar_journal_entry": map[string]any{
			"content": content,
		},
	}

	_, err = s.client.PatchMutation(ctx, fmt.Sprintf("/calendar/days/%s/journal_entry", day), body)
	return err
}
