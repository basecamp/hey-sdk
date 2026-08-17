package hey

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// ContactsService handles contact operations.
type ContactsService struct {
	client *Client
}

// NewContactsService creates a new ContactsService.
func NewContactsService(client *Client) *ContactsService {
	return &ContactsService{client: client}
}

// List returns all contacts.
func (s *ContactsService) List(ctx context.Context, params *generated.ListContactsParams) (result *generated.ListContactsResponseContent, err error) {
	op := OperationInfo{
		Service: "Contacts", Operation: "ListContacts",
		ResourceType: "contact", IsMutation: false,
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
	resp, err := s.client.gen.ListContactsWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// Get returns a specific contact by ID.
func (s *ContactsService) Get(ctx context.Context, contactID int64) (result *generated.ContactDetail, err error) {
	op := OperationInfo{
		Service: "Contacts", Operation: "GetContact",
		ResourceType: "contact", IsMutation: false, ResourceID: contactID,
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
	resp, err := s.client.gen.GetContactWithResponse(ctx, contactID)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// --- Bundling and screening ---

// Bundle groups a contact's mail together in the box instead of listing every thread.
func (s *ContactsService) Bundle(ctx context.Context, contactID int64) error {
	op := OperationInfo{
		Service: "Contacts", Operation: "BundleContact",
		ResourceType: "contact", IsMutation: true, ResourceID: contactID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().BundleContactWithResponse(ctx, contactID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Unbundle stops grouping a contact's mail.
func (s *ContactsService) Unbundle(ctx context.Context, contactID int64) error {
	op := OperationInfo{
		Service: "Contacts", Operation: "UnbundleContact",
		ResourceType: "contact", IsMutation: true, ResourceID: contactID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().UnbundleContactWithResponse(ctx, contactID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// ClearanceApproved and ClearanceDenied are the two screener decisions the API accepts.
const (
	ClearanceApproved = "approved"
	ClearanceDenied   = "denied"
)

// Screen answers the screener for a contact. Status is ClearanceApproved or ClearanceDenied.
func (s *ContactsService) Screen(ctx context.Context, contactID int64, status string) error {
	op := OperationInfo{
		Service: "Contacts", Operation: "UpdateContactClearance",
		ResourceType: "clearance", IsMutation: true, ResourceID: contactID,
	}

	if status != ClearanceApproved && status != ClearanceDenied {
		return &Error{Code: CodeValidation, Message: fmt.Sprintf("clearance status must be %q or %q, got %q", ClearanceApproved, ClearanceDenied, status)}
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().UpdateContactClearanceWithResponse(ctx, contactID, generated.UpdateContactClearanceRequestContent{Status: status})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Clearances returns the screener summary — how many senders are waiting to be screened.
// The API does not expose the pending senders themselves here.
func (s *ContactsService) Clearances(ctx context.Context) (result *generated.ClearanceSummary, err error) {
	op := OperationInfo{
		Service: "Contacts", Operation: "GetClearances",
		ResourceType: "clearance", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetClearancesWithResponse(ctx)
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

// --- Contact CRUD and notes ---
//
// HEY has no JSON surface for writing contacts: each of these answers with a redirect.

// ContactParams describes a contact.
type ContactParams struct {
	// Name is the contact's display name.
	Name string
	// EmailAddress is their main address.
	EmailAddress string
	// AliasEmailAddresses are other addresses that belong to the same person.
	AliasEmailAddresses []string
}

// Create adds a contact and returns its id.
func (s *ContactsService) Create(ctx context.Context, params ContactParams) (id int64, err error) {
	op := OperationInfo{
		Service: "Contacts", Operation: "CreateContact",
		ResourceType: "contact", IsMutation: true,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.PostForm(ctx, "/contacts", contactForm(params))
		if rerr != nil {
			return rerr
		}
		id, rerr = resp.ExtractID()
		return rerr
	})
	return id, err
}

// Update edits a contact. Empty fields are left alone, except AliasEmailAddresses, which
// replaces the whole list when it is non-nil.
func (s *ContactsService) Update(ctx context.Context, contactID int64, params ContactParams) error {
	op := OperationInfo{
		Service: "Contacts", Operation: "UpdateContact",
		ResourceType: "contact", IsMutation: true, ResourceID: contactID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.PatchForm(ctx, fmt.Sprintf("/contacts/%d", contactID), contactForm(params))
		return err
	})
}

// Hide takes a contact out of the contact list. Nothing is deleted — Reveal brings them back.
func (s *ContactsService) Hide(ctx context.Context, contactID int64) error {
	op := OperationInfo{
		Service: "Contacts", Operation: "HideContact",
		ResourceType: "contact", IsMutation: true, ResourceID: contactID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.DeleteForm(ctx, fmt.Sprintf("/contacts/%d", contactID))
		return err
	})
}

// Reveal puts a hidden contact back in the contact list.
func (s *ContactsService) Reveal(ctx context.Context, contactID int64) error {
	op := OperationInfo{
		Service: "Contacts", Operation: "RevealContact",
		ResourceType: "contact", IsMutation: true, ResourceID: contactID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.PostForm(ctx, fmt.Sprintf("/contacts/%d/reveal", contactID), url.Values{})
		return err
	})
}

// SetNote writes the private note you keep on a contact, replacing whatever was there.
func (s *ContactsService) SetNote(ctx context.Context, contactID int64, note string) error {
	op := OperationInfo{
		Service: "Contacts", Operation: "UpdateContactNote",
		ResourceType: "note", IsMutation: true, ResourceID: contactID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		values := url.Values{}
		values.Set("contact[note]", note)

		_, err := s.client.PatchForm(ctx, fmt.Sprintf("/contacts/%d/note", contactID), values)
		return err
	})
}

// DeleteNote clears the private note on a contact.
func (s *ContactsService) DeleteNote(ctx context.Context, contactID int64) error {
	op := OperationInfo{
		Service: "Contacts", Operation: "DeleteContactNote",
		ResourceType: "note", IsMutation: true, ResourceID: contactID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.DeleteForm(ctx, fmt.Sprintf("/contacts/%d/note", contactID))
		return err
	})
}

// contactForm renders the contact params as the nested form the server expects. The contact
// key is always present because the server requires it, even on a partial update.
func contactForm(params ContactParams) url.Values {
	values := url.Values{}
	values.Set("contact[name]", params.Name)
	if params.EmailAddress != "" {
		values.Set("contact[email_address]", params.EmailAddress)
	}
	for _, alias := range params.AliasEmailAddresses {
		values.Add("contact[alias_email_addresses][]", alias)
	}
	return values
}
