package hey

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// ExtenzionsService handles email extenzion operations.
//
// Extenzions allow custom email addresses on custom-domain HEY accounts
// (e.g., sales@yourdomain.com). The API uses form-encoded requests and
// HTML responses because there are no JSON endpoints for extenzions.
type ExtenzionsService struct {
	client *Client
}

// NewExtenzionsService creates a new ExtenzionsService.
func NewExtenzionsService(client *Client) *ExtenzionsService {
	return &ExtenzionsService{client: client}
}

// Extenzion represents an email extenzion.
type Extenzion struct {
	ID      int64
	Name    string
	Email   string
	Members []string
}

// CreateExtenzionParams contains the parameters for creating an extenzion.
type CreateExtenzionParams struct {
	// Name is the extenzion name (e.g., "sales" becomes sales@yourdomain.com).
	Name string
	// Members is a list of member email addresses.
	Members []string
}

// UpdateExtenzionParams contains the parameters for updating an extenzion.
type UpdateExtenzionParams struct {
	// Name is the new extenzion name. Empty string means no change.
	Name string
	// Members is the new list of member email addresses. Replaces all existing members.
	// nil means no change.
	Members []string
}

// List returns all extenzions for the given account.
// The list is parsed from HTML because there is no JSON endpoint.
func (s *ExtenzionsService) List(ctx context.Context, accountID int64) (result []Extenzion, err error) {
	op := OperationInfo{
		Service: "Extenzions", Operation: "ListExtenzions",
		ResourceType: "extenzion", IsMutation: false,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	resp, err := s.client.GetHTML(ctx, fmt.Sprintf("/accounts/%d/domains/extenzions", accountID))
	if err != nil {
		return nil, err
	}

	return parseExtenzionsHTML(string(resp.Data))
}

// Create creates a new extenzion.
// Returns the ID of the created extenzion.
func (s *ExtenzionsService) Create(ctx context.Context, accountID int64, params CreateExtenzionParams) (id int64, err error) {
	op := OperationInfo{
		Service: "Extenzions", Operation: "CreateExtenzion",
		ResourceType: "extenzion", IsMutation: true,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	values := url.Values{}
	values.Set("extenzion[name]", params.Name)
	for _, m := range params.Members {
		values.Add("extenzion[members][]", m)
	}

	resp, err := s.client.PostForm(ctx, fmt.Sprintf("/accounts/%d/domains/extenzions", accountID), values)
	if err != nil {
		return 0, err
	}
	return resp.ExtractID()
}

// Update updates an existing extenzion.
func (s *ExtenzionsService) Update(ctx context.Context, accountID int64, extID int64, params UpdateExtenzionParams) (err error) {
	op := OperationInfo{
		Service: "Extenzions", Operation: "UpdateExtenzion",
		ResourceType: "extenzion", IsMutation: true, ResourceID: extID,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	values := url.Values{}
	if params.Name != "" {
		values.Set("extenzion[name]", params.Name)
	}
	if params.Members != nil {
		for _, m := range params.Members {
			values.Add("extenzion[members][]", m)
		}
	}

	_, err = s.client.PatchForm(ctx, fmt.Sprintf("/accounts/%d/domains/extenzions/%d", accountID, extID), values)
	return err
}

// Delete deletes an extenzion.
func (s *ExtenzionsService) Delete(ctx context.Context, accountID int64, extID int64) (err error) {
	op := OperationInfo{
		Service: "Extenzions", Operation: "DeleteExtenzion",
		ResourceType: "extenzion", IsMutation: true, ResourceID: extID,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	_, err = s.client.DeleteForm(ctx, fmt.Sprintf("/accounts/%d/domains/extenzions/%d", accountID, extID))
	return err
}

// extEditLinkRe matches edit links like /accounts/123/domains/extenzions/456/edit
var extEditLinkRe = regexp.MustCompile(`/accounts/\d+/domains/extenzions/(\d+)/edit`)

// parseExtenzionsHTML parses the extenzions list page HTML and extracts extenzions.
func parseExtenzionsHTML(htmlContent string) ([]Extenzion, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse extenzions HTML: %w", err)
	}

	var extenzions []Extenzion
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		// Look for edit links to discover extenzions and their IDs
		if n.Type == html.ElementNode && n.Data == "a" {
			href := getAttr(n, "href")
			matches := extEditLinkRe.FindStringSubmatch(href)
			if matches != nil {
				var extID int64
				if _, err := fmt.Sscanf(matches[1], "%d", &extID); err == nil {
					ext := Extenzion{ID: extID}

					// Walk up to find the containing element for this extenzion's info
					// Look for sibling/parent text content for name, email, members
					parent := n.Parent
					if parent != nil {
						ext.Name, ext.Email, ext.Members = extractExtenzionInfo(parent)
					}

					extenzions = append(extenzions, ext)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return extenzions, nil
}

// extractExtenzionInfo extracts extenzion details from a container node.
func extractExtenzionInfo(n *html.Node) (name, email string, members []string) {
	texts := collectTexts(n)
	for _, t := range texts {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if strings.Contains(t, "@") {
			if email == "" {
				email = t
				// Derive name from email (part before @)
				if name == "" {
					parts := strings.SplitN(t, "@", 2)
					name = parts[0]
				}
			} else {
				members = append(members, t)
			}
		}
	}
	return
}

// collectTexts collects all text content from a node tree.
func collectTexts(n *html.Node) []string {
	var texts []string
	if n.Type == html.TextNode {
		texts = append(texts, n.Data)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		texts = append(texts, collectTexts(c)...)
	}
	return texts
}

// getAttr returns the value of the named attribute on the node.
func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
