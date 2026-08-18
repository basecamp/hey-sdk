package hey

import (
	"context"
	"strconv"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// SearchService searches mail.
//
// HEY has no JSON search endpoint — /search and /advanced_search render HTML — so results are
// read off the advanced search page. Only the refine options are JSON, via Filters.
type SearchService struct {
	client *Client
}

// NewSearchService creates a new SearchService.
func NewSearchService(client *Client) *SearchService {
	return &SearchService{client: client}
}

// SearchParams describes an advanced search. Query is the free-text part; the rest map onto
// the refine[...] parameters the advanced search form submits.
type SearchParams struct {
	// Query is the words to search for.
	Query string
	// Page is the 1-based results page. Zero asks for the first.
	Page int

	// Required words must all appear.
	Required string
	// Any is a set of words, at least one of which must appear.
	Any string
	// None words must not appear.
	None string
	// ExactPhrase must appear verbatim.
	ExactPhrase string
	// From narrows by sender, To by recipient, Subject by subject line.
	From    string
	To      string
	Subject string
	// Date is "last_7_days", "last_30_days", "last_90_days" or a four-digit year.
	Date string
	// In narrows to a box: "imbox", "feed", "papertrail" or "trash".
	In string
	// Label narrows to a folder name.
	Label string
	// Attachment narrows by attachment kind, or "any".
	Attachment string
}

// Search runs an advanced search and returns the matching threads, grouped by topic as
// the search page shows them: the topic, your posting of it, and the entries that matched
// (summaries; read a message with Messages().Get). The next page, if any, is followed
// by passing Page.
func (s *SearchService) Search(ctx context.Context, params SearchParams) (result *generated.AdvancedSearchResult, err error) {
	op := OperationInfo{
		Service: "Search", Operation: "AdvancedSearch",
		ResourceType: "search", IsMutation: false,
	}
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().AdvancedSearchWithResponse(ctx, params.generated())
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = resp.JSON200
		return nil
	})
	return result, err
}

// Filters returns the options the advanced search refine form offers: boxes, date ranges,
// labels and attachment kinds.
func (s *SearchService) Filters(ctx context.Context) (result *generated.AdvancedSearchFilters, err error) {
	op := OperationInfo{
		Service: "Search", Operation: "GetAdvancedSearchFilters",
		ResourceType: "search", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetAdvancedSearchFiltersWithResponse(ctx)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = resp.JSON200
		return nil
	})
	return result, err
}

// generated maps the params onto the generated request, sending only what is set.
func (p SearchParams) generated() *generated.AdvancedSearchParams {
	opt := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	gp := &generated.AdvancedSearchParams{
		Q:                 opt(p.Query),
		RefineFrom:        opt(p.From),
		RefineTo:          opt(p.To),
		RefineSubject:     opt(p.Subject),
		RefineExactPhrase: opt(p.ExactPhrase),
		RefineRequired:    opt(p.Required),
		RefineAny:         opt(p.Any),
		RefineNone:        opt(p.None),
		RefineDate:        opt(p.Date),
		RefineIn:          opt(p.In),
		RefineLabel:       opt(p.Label),
		RefineAttachment:  opt(p.Attachment),
	}
	if p.Page > 1 {
		gp.Page = opt(strconv.Itoa(p.Page))
	}
	return gp
}
