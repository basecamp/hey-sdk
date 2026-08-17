package hey

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// ClearancesService handles HEY Screener operations.
type ClearancesService struct {
	client *Client
}

// NewClearancesService creates a new ClearancesService.
func NewClearancesService(client *Client) *ClearancesService {
	return &ClearancesService{client: client}
}

// PendingClearance is an email sender awaiting a Screener decision.
type PendingClearance struct {
	ID           int64  `json:"id"`
	EntryID      int64  `json:"entry_id,omitempty"`
	TopicID      int64  `json:"topic_id,omitempty"`
	Name         string `json:"name,omitempty"`
	EmailAddress string `json:"email_address,omitempty"`
	Subject      string `json:"subject,omitempty"`
	FeedBoxID    int64  `json:"feed_box_id,omitempty"`
	TrailBoxID   int64  `json:"trail_box_id,omitempty"`
}

// List returns all pending Screener clearances.
func (s *ClearancesService) List(ctx context.Context) ([]PendingClearance, error) {
	return s.ListWithLimit(ctx, 0)
}

// ListWithLimit returns pending Screener clearances up to limit. A zero limit
// follows every cursor page.
func (s *ClearancesService) ListWithLimit(ctx context.Context, limit int) (result []PendingClearance, err error) {
	if limit < 0 {
		return nil, fmt.Errorf("clearance limit must not be negative")
	}
	result = make([]PendingClearance, 0)

	op := OperationInfo{
		Service: "Clearances", Operation: "ListClearances",
		ResourceType: "clearance", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().ListClearancesWithResponse(contextWithAccept(ctx, "text/html"))
		if rerr != nil {
			return rerr
		}
		if rerr = CheckResponse(resp.HTTPResponse); rerr != nil {
			return rerr
		}
		if resp.HTTPResponse == nil || resp.HTTPResponse.Request == nil || resp.HTTPResponse.Request.URL == nil {
			return fmt.Errorf("cannot follow clearance pagination: response has no request URL")
		}

		currentURL := resp.HTTPResponse.Request.URL.String()
		originURL := s.client.cfg.BaseURL
		if !isSameOrigin(originURL, currentURL) {
			return fmt.Errorf("clearance list redirected to different origin")
		}
		page, rerr := parseClearancesPageHTML(string(resp.Body))
		if rerr != nil {
			return rerr
		}

		seenPages := map[string]struct{}{currentURL: {}}
		seenClearances := make(map[int64]struct{})
		for pageNumber := 1; ; pageNumber++ {
			for _, clearance := range page.Clearances {
				if _, seen := seenClearances[clearance.ID]; seen {
					continue
				}
				seenClearances[clearance.ID] = struct{}{}
				result = append(result, clearance)
				if limit > 0 && len(result) >= limit {
					return nil
				}
			}

			if page.NextPage == "" {
				return nil
			}
			if pageNumber >= s.client.httpOpts.MaxPages {
				return fmt.Errorf("clearance pagination exceeded max pages (%d)", s.client.httpOpts.MaxPages)
			}

			nextURL := resolveURL(currentURL, page.NextPage)
			if !isSameOrigin(originURL, nextURL) {
				return fmt.Errorf("clearance pagination link points to different origin")
			}
			if _, seen := seenPages[nextURL]; seen {
				return fmt.Errorf("clearance pagination loop detected")
			}
			seenPages[nextURL] = struct{}{}

			nextResponse, rerr := s.client.GetHTML(ctx, nextURL)
			if rerr != nil {
				return rerr
			}
			page, rerr = parseClearancesPageHTML(string(nextResponse.Data))
			if rerr != nil {
				return rerr
			}
			currentURL = nextURL
		}
	})
	return result, err
}

// Approve screens a sender in. A zero designationBoxID uses the Imbox.
func (s *ClearancesService) Approve(ctx context.Context, clearanceID, designationBoxID int64) error {
	if designationBoxID < 0 {
		return fmt.Errorf("designation box ID must not be negative")
	}
	return s.update(ctx, clearanceID, "approved", designationBoxID)
}

// Deny screens a sender out.
func (s *ClearancesService) Deny(ctx context.Context, clearanceID int64) error {
	return s.update(ctx, clearanceID, "denied", 0)
}

func (s *ClearancesService) update(ctx context.Context, clearanceID int64, status string, designationBoxID int64) error {
	if clearanceID <= 0 {
		return fmt.Errorf("clearance ID must be positive")
	}

	op := OperationInfo{
		Service: "Clearances", Operation: "UpdateClearance",
		ResourceType: "clearance", IsMutation: true, ResourceID: clearanceID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		values := url.Values{"status": {status}}
		if designationBoxID > 0 {
			values.Set("designation_box_id", strconv.FormatInt(designationBoxID, 10))
		}

		resp, err := s.client.genClient().UpdateClearanceWithBodyWithResponse(
			contextWithAccept(ctx, "*/*"),
			clearanceID,
			"application/x-www-form-urlencoded",
			strings.NewReader(values.Encode()),
		)
		if err != nil {
			return err
		}
		if resp.HTTPResponse != nil && (resp.HTTPResponse.StatusCode == http.StatusFound || resp.HTTPResponse.StatusCode == http.StatusSeeOther) {
			return nil
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

var clearanceIDPattern = regexp.MustCompile(`^clearance_(\d+)$`)
var clearanceEntryPathPattern = regexp.MustCompile(`^/clearances/entries/(\d+)$`)

type clearancesPage struct {
	Clearances []PendingClearance
	NextPage   string
}

func parseClearancesPageHTML(pageHTML string) (clearancesPage, error) {
	doc, err := html.Parse(strings.NewReader(pageHTML))
	if err != nil {
		return clearancesPage{}, fmt.Errorf("parse clearances HTML: %w", err)
	}

	var page clearancesPage
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "article" && clearanceHasClass(node, "clearance") {
			if clearance, ok := parseClearanceNode(node); ok {
				page.Clearances = append(page.Clearances, clearance)
			}
			return
		}
		if node.Type == html.ElementNode && node.Data == "a" && clearanceHasClass(node, "pagination-link") {
			if nextPage := clearanceNodeAttr(node, "href"); nextPage != "" {
				page.NextPage = nextPage
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return page, nil
}

func parseClearanceNode(node *html.Node) (PendingClearance, bool) {
	match := clearanceIDPattern.FindStringSubmatch(clearanceNodeAttr(node, "id"))
	if match == nil {
		return PendingClearance{}, false
	}
	id, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return PendingClearance{}, false
	}

	clearance := PendingClearance{ID: id}
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.ElementNode {
			currentID := clearanceNodeAttr(current, "id")
			switch {
			case currentID == fmt.Sprintf("name_clearance_%d", id):
				clearance.Name = clearanceNodeText(current)
			case currentID == fmt.Sprintf("email_clearance_%d", id):
				clearance.EmailAddress = clearanceNodeText(current)
			case clearanceHasClass(current, "clearance__subject"):
				clearance.Subject = clearanceNodeText(current)
			case current.Data == "turbo-frame":
				if entryMatch := clearanceEntryPathPattern.FindStringSubmatch(clearanceNodeAttr(current, "src")); entryMatch != nil {
					clearance.EntryID, _ = strconv.ParseInt(entryMatch[1], 10, 64)
				}
			case current.Data == "input" && clearanceNodeAttr(current, "name") == "reply_to_topic_id":
				clearance.TopicID, _ = strconv.ParseInt(clearanceNodeAttr(current, "value"), 10, 64)
			case current.Data == "form":
				target, boxID := clearanceDesignation(current)
				switch target {
				case "feedboxButton":
					clearance.FeedBoxID = boxID
				case "trailboxButton":
					clearance.TrailBoxID = boxID
				}
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return clearance, true
}

func clearanceDesignation(form *html.Node) (target string, boxID int64) {
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			if value := clearanceNodeAttr(node, "data-clearances-target"); value != "" {
				target = value
			}
			if node.Data == "input" && clearanceNodeAttr(node, "name") == "designation_box_id" {
				boxID, _ = strconv.ParseInt(clearanceNodeAttr(node, "value"), 10, 64)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(form)
	return target, boxID
}

func clearanceHasClass(node *html.Node, class string) bool {
	for _, candidate := range strings.Fields(clearanceNodeAttr(node, "class")) {
		if candidate == class {
			return true
		}
	}
	return false
}

func clearanceNodeAttr(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func clearanceNodeText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(builder.String()), " ")
}
