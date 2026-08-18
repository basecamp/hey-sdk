package hey

import (
	"context"
	"fmt"
	"net/url"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
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

// List returns the snippets, alphabetically, with their content as text and as HTML.
func (s *SnippetsService) List(ctx context.Context) (result []generated.Snippet, err error) {
	op := OperationInfo{
		Service: "Snippets", Operation: "ListSnippets",
		ResourceType: "snippet", IsMutation: false,
	}
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().ListSnippetsWithResponse(ctx)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		if resp.JSON200 != nil {
			result = *resp.JSON200
		}
		return nil
	})
	return result, err
}

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
