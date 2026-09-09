package hey

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// ExtenzionsService handles email extenzion operations.
//
// Extenzions allow custom email addresses on custom-domain HEY accounts
// (e.g., sales@yourdomain.com). Create and Update take form-encoded requests but post to the
// .json path, so a current server answers the written extenzion; a server without the JSON
// branch redirects instead and hands nothing back. Delete is a plain JSON operation.
type ExtenzionsService struct {
	client *Client
}

// NewExtenzionsService creates a new ExtenzionsService.
func NewExtenzionsService(client *Client) *ExtenzionsService {
	return &ExtenzionsService{client: client}
}

// Extenzion represents an email extenzion.
//
// The id is the extenzion's contact id — the same id the write endpoints take.
type Extenzion struct {
	ID     int64
	Name   string
	AppURL string
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

// List returns the extenzions on the account.
//
// This reads the navigation payload rather than scraping the extenzions page, so it carries
// only what navigation carries: each extenzion's name and its contact URL.
func (s *ExtenzionsService) List(ctx context.Context) (result []Extenzion, err error) {
	op := OperationInfo{
		Service: "Extenzions", Operation: "ListExtenzions",
		ResourceType: "extenzion", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		navigation, nerr := s.client.Identity().GetNavigation(ctx)
		if nerr != nil {
			return nerr
		}
		result = extenzionsFromNavigation(navigation)
		return nil
	})
	return result, err
}

// Create creates a new extenzion and returns it. A server without the JSON create branch
// hands nothing back, and the result is then nil.
func (s *ExtenzionsService) Create(ctx context.Context, accountID int64, params CreateExtenzionParams) (extenzion *Extenzion, err error) {
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

	resp, err := s.client.PostForm(ctx, fmt.Sprintf("/accounts/%d/domains/extenzions.json", accountID), values)
	if err != nil {
		return nil, err
	}
	return extenzionFromFormResponse(resp)
}

// Update updates an existing extenzion and returns it. A server without the JSON update
// branch hands nothing back, and the result is then nil.
func (s *ExtenzionsService) Update(ctx context.Context, accountID int64, extID int64, params UpdateExtenzionParams) (extenzion *Extenzion, err error) {
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

	resp, err := s.client.PatchForm(ctx, fmt.Sprintf("/accounts/%d/domains/extenzions/%d.json", accountID, extID), values)
	if err != nil {
		return nil, err
	}
	return extenzionFromFormResponse(resp)
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

	resp, err := s.client.genClient().DeleteExtenzionWithResponse(ctx, accountID, extID)
	if err != nil {
		return err
	}
	return CheckResponse(resp.HTTPResponse)
}

// extenzionFromFormResponse reads the extenzion a JSON write answers with. A server without
// the JSON branch redirects to the extenzions page instead, which leaves nothing to read.
//
// The payload's own id is the Extenzion record's, while every write endpoint takes the
// contact's — so the id comes out of app_url, the way List reads it out of navigation.
func extenzionFromFormResponse(resp *FormResponse) (*Extenzion, error) {
	if len(resp.Body) == 0 {
		return nil, nil
	}

	var payload struct {
		Name   string `json:"name"`
		AppURL string `json:"app_url"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
		return nil, fmt.Errorf("failed to decode the extension: %w", err)
	}

	id, err := contactIDFromURL(payload.AppURL)
	if err != nil {
		return nil, err
	}
	return &Extenzion{ID: id, Name: payload.Name, AppURL: payload.AppURL}, nil
}

// contactIDFromURL pulls the contact id out of a contact URL, e.g. /contacts/4821.
func contactIDFromURL(contactURL string) (int64, error) {
	match := contactPathRe.FindStringSubmatch(contactURL)
	if match == nil {
		return 0, fmt.Errorf("no contact id in %q", contactURL)
	}
	return strconv.ParseInt(match[1], 10, 64)
}

// contactPathRe matches the contact URLs navigation gives each extenzion, e.g. /contacts/4821
var contactPathRe = regexp.MustCompile(`/contacts/(\d+)`)

// extenzionsFromNavigation pulls the extenzions out of the "Extensions" navigation group.
// The group's first entry is the "All Extensions"/"Manage Extensions" link, which has no
// contact URL and so falls out on its own.
func extenzionsFromNavigation(navigation *generated.NavigationResponse) []Extenzion {
	if navigation == nil {
		return nil
	}

	var extenzions []Extenzion
	for _, item := range navigation.Items {
		if item.Title != navigationExtenzionsTitle {
			continue
		}
		for _, entry := range item.MenuItems {
			match := contactPathRe.FindStringSubmatch(entry.AppUrl)
			if match == nil {
				continue
			}
			id, err := strconv.ParseInt(match[1], 10, 64)
			if err != nil {
				continue
			}
			extenzions = append(extenzions, Extenzion{ID: id, Name: entry.Title, AppURL: entry.AppUrl})
		}
	}
	return extenzions
}

// navigationExtenzionsTitle is the title HEY gives the extensions group in the navigation payload.
const navigationExtenzionsTitle = "Extensions"
