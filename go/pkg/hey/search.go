package hey

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"

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

// SearchMatch is one thread the search turned up.
type SearchMatch struct {
	PostingID int64
	TopicID   int64
	Title     string
	Creator   string
	Summary   string
	AppURL    string
}

// Search runs an advanced search and returns the matching threads.
func (s *SearchService) Search(ctx context.Context, params SearchParams) (result []SearchMatch, err error) {
	op := OperationInfo{
		Service: "Search", Operation: "AdvancedSearch",
		ResourceType: "search", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.GetHTML(ctx, "/advanced_search?"+params.query().Encode())
		if rerr != nil {
			return rerr
		}

		result, rerr = parseSearchMatchesHTML(string(resp.Data))
		return rerr
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

// query renders the params as the query string the advanced search page expects.
func (p SearchParams) query() url.Values {
	values := url.Values{}
	if p.Query != "" {
		values.Set("q", p.Query)
	}
	if p.Page > 1 {
		values.Set("page", strconv.Itoa(p.Page))
	}

	refinements := map[string]string{
		"required":     p.Required,
		"any":          p.Any,
		"none":         p.None,
		"exact_phrase": p.ExactPhrase,
		"from":         p.From,
		"to":           p.To,
		"subject":      p.Subject,
		"date":         p.Date,
		"in":           p.In,
		"label":        p.Label,
		"attachment":   p.Attachment,
	}
	for name, value := range refinements {
		if value != "" {
			values.Set("refine["+name+"]", value)
		}
	}
	return values
}

// searchResultIDRe pulls the posting id out of the per-result element ids the search page
// stamps, e.g. topic_name_posting_4471829.
var searchResultIDRe = regexp.MustCompile(`^(topic_name|creator|summary)_posting_(\d+)$`)

// searchTopicPathRe pulls the topic id out of a result's link.
var searchTopicPathRe = regexp.MustCompile(`/topics/(\d+)`)

// parseSearchMatchesHTML reads the results off the advanced search page. Each result stamps
// its posting id into the ids of its title, creator and summary elements, which is the most
// stable handle the page offers.
func parseSearchMatchesHTML(page string) ([]SearchMatch, error) {
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return nil, fmt.Errorf("failed to parse the search results: %w", err)
	}

	matches := map[int64]*SearchMatch{}
	var order []int64

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			if parts := searchResultIDRe.FindStringSubmatch(nodeAttr(node, "id")); parts != nil {
				postingID, perr := strconv.ParseInt(parts[2], 10, 64)
				if perr == nil {
					match, seen := matches[postingID]
					if !seen {
						match = &SearchMatch{PostingID: postingID}
						matches[postingID] = match
						order = append(order, postingID)
					}
					assignSearchField(match, parts[1], nodeText(node))
					if match.AppURL == "" {
						match.AppURL = enclosingLinkHref(node)
						if topic := searchTopicPathRe.FindStringSubmatch(match.AppURL); topic != nil {
							match.TopicID, _ = strconv.ParseInt(topic[1], 10, 64)
						}
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)

	results := make([]SearchMatch, 0, len(order))
	for _, postingID := range order {
		results = append(results, *matches[postingID])
	}
	return results, nil
}

// assignSearchField puts a scraped value on the field its element id names.
func assignSearchField(match *SearchMatch, field, value string) {
	switch field {
	case "topic_name":
		match.Title = value
	case "creator":
		match.Creator = value
	case "summary":
		match.Summary = value
	}
}

// enclosingLinkHref walks up to the nearest anchor and returns its href.
func enclosingLinkHref(node *html.Node) string {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Type == html.ElementNode && parent.Data == "a" {
			return nodeAttr(parent, "href")
		}
	}
	return ""
}

// nodeAttr returns a node's attribute value, or the empty string.
func nodeAttr(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// nodeText returns a node's collapsed text content.
func nodeText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			builder.WriteString(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(builder.String()), " ")
}
