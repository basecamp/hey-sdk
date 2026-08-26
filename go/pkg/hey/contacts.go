package hey

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		result, err = s.get(ctx, contactID)
		return err
	})
	return result, err
}

// get is the un-instrumented read shared by Get and Update.
func (s *ContactsService) get(ctx context.Context, contactID int64) (*generated.ContactDetail, error) {
	resp, err := s.client.genClient().GetContactWithResponse(ctx, contactID, nil)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// ContactPage is one page of a contact and the threads they are on: the cursor for the
// page below is empty on the last page.
type ContactPage struct {
	Contact  *generated.ContactDetail
	NextPage string
}

// ThreadsPage reads a contact with one page of the threads they are on — what HEY heads
// "All threads with …" (the contact's entries_title). An empty cursor starts at the top;
// the next page's cursor comes back on the page before it.
func (s *ContactsService) ThreadsPage(ctx context.Context, contactID int64, cursor string) (result *ContactPage, err error) {
	op := OperationInfo{
		Service: "Contacts", Operation: "GetContact",
		ResourceType: "contact", IsMutation: false, ResourceID: contactID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		params := &generated.GetContactParams{}
		if cursor != "" {
			params.Page = &cursor
		}
		resp, rerr := s.client.genClient().GetContactWithResponse(ctx, contactID, params)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = &ContactPage{Contact: resp.JSON200}
		if resp.HTTPResponse != nil {
			result.NextPage = gearedPageFromLink(resp.HTTPResponse.Header.Get("Link"))
		}
		return nil
	})
	return result, err
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
//
// Deprecated: use Client.Clearances(). PendingCount answers the same count, and Pending
// answers the senders themselves.
func (s *ContactsService) Clearances(ctx context.Context) (result *generated.ClearanceSummary, err error) {
	op := OperationInfo{
		Service: "Contacts", Operation: "GetClearances",
		ResourceType: "clearance", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetClearancesWithResponse(ctx, &generated.GetClearancesParams{})
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

// ContactConflictError is returned when a contact write submits an email address that
// already belongs to another contact. HEY's web sends you to a merge form at that point;
// ConflictingContactIDs are the contacts it would have offered to merge with.
//
// A create that clashes still creates the contact — the merge happens afterwards — so
// ContactID is the contact that was written, not a contact that failed to be.
//
// It wraps the SDK's conflict error, so errors.As still finds a *hey.Error with
// CodeConflict for callers that only care that the write was refused.
type ContactConflictError struct {
	ContactID             int64
	ConflictingContactIDs []int64

	err *Error
}

// Error implements the error interface.
func (e *ContactConflictError) Error() string { return e.err.Error() }

// Unwrap returns the underlying conflict error.
func (e *ContactConflictError) Unwrap() error { return e.err }

// ContactParams describes a contact.
type ContactParams struct {
	// Name is the contact's display name.
	Name string
	// EmailAddress is their main address.
	EmailAddress string
	// AliasEmailAddresses are other addresses that belong to the same person.
	AliasEmailAddresses []string
	// AccountUserID picks the account the contact belongs to, on Create. One identity can
	// hold several accounts, each with its own contacts; this is the identity's user on
	// the one you mean, which Identity returns in all_users alongside its account_id.
	// Left zero, HEY files the contact under the first account. Update ignores it.
	AccountUserID int64
}

// Create adds a contact and returns it.
func (s *ContactsService) Create(ctx context.Context, params ContactParams) (contact *generated.Contact, err error) {
	op := OperationInfo{
		Service: "Contacts", Operation: "CreateContact",
		ResourceType: "contact", IsMutation: true,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		if accountID, scoped := s.client.AccountID(); scoped {
			accountUserID, rerr := s.client.AccountUserID(ctx)
			if rerr != nil {
				return rerr
			}
			if params.AccountUserID != 0 && params.AccountUserID != accountUserID {
				return ErrUsage(fmt.Sprintf("account user %d does not belong to selected account %d", params.AccountUserID, accountID))
			}
			params.AccountUserID = accountUserID
		}

		resp, rerr := s.client.genClient().CreateContactWithResponse(ctx, createContactBody(params))
		if rerr != nil {
			return rerr
		}
		if cerr := contactWriteError(resp.JSON409, resp.JSON422, resp.HTTPResponse); cerr != nil {
			return cerr
		}
		contact = resp.JSON201
		return nil
	})
	return contact, err
}

// Update edits a contact and returns it. Empty fields are left alone, except
// AliasEmailAddresses, which replaces the whole list when it is non-nil.
//
// HEY's update is a full replacement (Contact::Ingress::Revise rewrites name, email and
// removes any alias not submitted), so the current contact is read first and unset
// fields are filled in from it before the write. That read-then-write is not atomic: a
// change made to the contact in between is overwritten with what was read. Pass every
// field explicitly when that matters.
//
// The contact that comes back is not always the one addressed: promoting an alias to the
// main address makes the alias the primary contact, and that is the one returned.
func (s *ContactsService) Update(ctx context.Context, contactID int64, params ContactParams) (contact *generated.Contact, err error) {
	op := OperationInfo{
		Service: "Contacts", Operation: "UpdateContact",
		ResourceType: "contact", IsMutation: true, ResourceID: contactID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		current, rerr := s.get(ctx, contactID)
		if rerr != nil {
			return rerr
		}
		merged := params
		if merged.Name == "" {
			merged.Name = current.Name
		}
		if merged.EmailAddress == "" {
			merged.EmailAddress = current.EmailAddress
		}
		if merged.AliasEmailAddresses == nil {
			for _, alias := range current.Aliases {
				merged.AliasEmailAddresses = append(merged.AliasEmailAddresses, alias.EmailAddress)
			}
		}

		resp, rerr := s.client.genClient().UpdateContactWithResponse(ctx, contactID, contactBody(merged))
		if rerr != nil {
			return rerr
		}
		if cerr := contactWriteError(resp.JSON409, resp.JSON422, resp.HTTPResponse); cerr != nil {
			return cerr
		}
		contact = resp.JSON200
		return nil
	})
	return contact, err
}

// Hide takes a contact out of the contact list. Nothing is deleted — Reveal brings them back.
func (s *ContactsService) Hide(ctx context.Context, contactID int64) error {
	op := OperationInfo{
		Service: "Contacts", Operation: "HideContact",
		ResourceType: "contact", IsMutation: true, ResourceID: contactID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().HideContactWithResponse(ctx, contactID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Reveal puts a hidden contact back in the contact list and returns it.
func (s *ContactsService) Reveal(ctx context.Context, contactID int64) (contact *generated.Contact, err error) {
	op := OperationInfo{
		Service: "Contacts", Operation: "RevealContact",
		ResourceType: "contact", IsMutation: true, ResourceID: contactID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().RevealContactWithResponse(ctx, contactID)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		contact = resp.JSON200
		return nil
	})
	return contact, err
}

// Note returns the private note kept on a contact. Its fields are empty strings when
// there is no note.
func (s *ContactsService) Note(ctx context.Context, contactID int64) (note *generated.ContactNote, err error) {
	op := OperationInfo{
		Service: "Contacts", Operation: "GetContactNote",
		ResourceType: "note", IsMutation: false, ResourceID: contactID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetContactNoteWithResponse(ctx, contactID)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		note = resp.JSON200
		return nil
	})
	return note, err
}

// SetNote writes the private note you keep on a contact, replacing whatever was there,
// and returns the note as it now reads.
func (s *ContactsService) SetNote(ctx context.Context, contactID int64, note string) (result *generated.ContactNote, err error) {
	op := OperationInfo{
		Service: "Contacts", Operation: "UpdateContactNote",
		ResourceType: "note", IsMutation: true, ResourceID: contactID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		body := generated.UpdateContactNoteJSONRequestBody{
			Contact: generated.ContactNotePayload{Note: note},
		}

		resp, rerr := s.client.genClient().UpdateContactNoteWithResponse(ctx, contactID, body)
		if rerr != nil {
			return rerr
		}
		if cerr := contactWriteError(nil, resp.JSON422, resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = resp.JSON200
		return nil
	})
	return result, err
}

// DeleteNote clears the private note on a contact.
func (s *ContactsService) DeleteNote(ctx context.Context, contactID int64) error {
	op := OperationInfo{
		Service: "Contacts", Operation: "DeleteContactNote",
		ResourceType: "note", IsMutation: true, ResourceID: contactID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().DeleteContactNoteWithResponse(ctx, contactID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// contactBody renders the contact params as the nested body the server expects. The
// contact key is always present because the server requires it, even on a partial write.
func contactBody(params ContactParams) generated.ContactRequestContent {
	return generated.ContactRequestContent{
		Contact: contactPayload(params),
	}
}

// createContactBody is contactBody plus the account to file the contact under.
func createContactBody(params ContactParams) generated.CreateContactRequestContent {
	return generated.CreateContactRequestContent{
		ActingUserId: params.AccountUserID,
		Contact:      contactPayload(params),
	}
}

func contactPayload(params ContactParams) generated.ContactPayload {
	return generated.ContactPayload{
		Name:                params.Name,
		EmailAddress:        params.EmailAddress,
		AliasEmailAddresses: params.AliasEmailAddresses,
	}
}

// conflictMessage reads the server's own words out of a 409. Contact writes answer the
// {"errors": [...]} list the other error paths use; elsewhere a 409 is a single message.
func conflictMessage(conflict *generated.ConflictErrorResponseContent) string {
	if len(conflict.Errors) > 0 {
		return strings.Join(conflict.Errors, "; ")
	}
	if conflict.Error != "" {
		return conflict.Error
	}
	// A 409 whose body we do not recognise still has to read as something.
	return "the contact conflicts with one that already exists"
}

// contactWriteError turns the two refusals a contact write can answer with into typed
// errors — an email address that belongs to someone else, and a contact the model itself
// rejected — and falls back to the usual status handling.
func contactWriteError(conflict *generated.ConflictErrorResponseContent, invalid *generated.UnprocessableEntityErrorResponseContent, resp *http.Response) error {
	if conflict != nil {
		return &ContactConflictError{
			ContactID:             conflict.ContactId,
			ConflictingContactIDs: conflict.ConflictingContactIds,
			err:                   ErrConflict(conflictMessage(conflict)),
		}
	}
	if invalid != nil {
		return ErrValidation(invalid.Errors...)
	}
	return CheckResponse(resp)
}
