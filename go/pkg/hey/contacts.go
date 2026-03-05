package hey

import (
	"context"
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
