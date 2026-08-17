package hey

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// SnippetsService handles snippets — reusable bits of text for the composer.
//
// Snippets have no JSON surface: every write redirects and the list is HTML.
type SnippetsService struct {
	client *Client
}

// NewSnippetsService creates a new SnippetsService.
func NewSnippetsService(client *Client) *SnippetsService {
	return &SnippetsService{client: client}
}

// Snippet is a reusable bit of text.
type Snippet struct {
	ID   int64
	Name string
}

// snippetEditLinkRe matches the edit links on the snippets page, which is where the ids and
// names live — the page carries no ids of its own.
var snippetEditLinkRe = regexp.MustCompile(`href="[^"]*/snippets/(\d+)/edit"[^>]*>([^<]*)<`)

// List returns the snippets, alphabetically. Snippet bodies are not on the list page.
func (s *SnippetsService) List(ctx context.Context) (result []Snippet, err error) {
	op := OperationInfo{
		Service: "Snippets", Operation: "ListSnippets",
		ResourceType: "snippet", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.GetHTML(ctx, "/snippets")
		if rerr != nil {
			return rerr
		}

		for _, match := range snippetEditLinkRe.FindAllStringSubmatch(string(resp.Data), -1) {
			id, perr := strconv.ParseInt(match[1], 10, 64)
			if perr != nil {
				continue
			}
			result = append(result, Snippet{ID: id, Name: strings.TrimSpace(match[2])})
		}
		return nil
	})
	return result, err
}

// Create adds a snippet.
func (s *SnippetsService) Create(ctx context.Context, name, content string) error {
	op := OperationInfo{
		Service: "Snippets", Operation: "CreateSnippet",
		ResourceType: "snippet", IsMutation: true,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.PostForm(ctx, "/snippets", snippetForm(name, content))
		return err
	})
}

// Update edits a snippet. Empty fields are left alone.
func (s *SnippetsService) Update(ctx context.Context, snippetID int64, name, content string) error {
	op := OperationInfo{
		Service: "Snippets", Operation: "UpdateSnippet",
		ResourceType: "snippet", IsMutation: true, ResourceID: snippetID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.PatchForm(ctx, fmt.Sprintf("/snippets/%d", snippetID), snippetForm(name, content))
		return err
	})
}

// Delete throws a snippet away.
func (s *SnippetsService) Delete(ctx context.Context, snippetID int64) error {
	op := OperationInfo{
		Service: "Snippets", Operation: "DeleteSnippet",
		ResourceType: "snippet", IsMutation: true, ResourceID: snippetID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.DeleteForm(ctx, fmt.Sprintf("/snippets/%d", snippetID))
		return err
	})
}

// snippetForm renders a snippet as the nested form the server expects.
func snippetForm(name, content string) url.Values {
	values := url.Values{}
	if name != "" {
		values.Set("snippet[name]", name)
	}
	if content != "" {
		values.Set("snippet[content]", content)
	}
	return values
}
