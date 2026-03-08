package hey

import (
	"context"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// SearchService handles search operations.
type SearchService struct {
	client *Client
}

// NewSearchService creates a new SearchService.
func NewSearchService(client *Client) *SearchService {
	return &SearchService{client: client}
}

// Search searches for content.
func (s *SearchService) Search(ctx context.Context, params *generated.SearchParams) (result *generated.SearchResponseContent, err error) {
	op := OperationInfo{
		Service: "Search", Operation: "Search",
		ResourceType: "search", IsMutation: false,
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
	resp, err := s.client.gen.SearchWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}
