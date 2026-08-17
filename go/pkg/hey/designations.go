package hey

import (
	"context"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// DesignationsService screens a contact into a box, so everything they send lands there.
type DesignationsService struct {
	client *Client
}

// NewDesignationsService creates a new DesignationsService.
func NewDesignationsService(client *Client) *DesignationsService {
	return &DesignationsService{client: client}
}

// Create designates a contact to a box.
//
// The server designates the contact's primary, so alias contacts fold into one designation
// whose id you cannot derive from contactID — read the box back if you need it.
func (s *DesignationsService) Create(ctx context.Context, boxID int64, contactID int64) error {
	op := OperationInfo{
		Service: "Designations", Operation: "CreateBoxDesignation",
		ResourceType: "designation", IsMutation: true, ResourceID: boxID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().CreateBoxDesignationWithResponse(ctx, boxID, generated.CreateBoxDesignationRequestContent{ContactId: contactID})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Destroy removes a designation from a box. designationID is the designation's own id, not
// the contact's.
func (s *DesignationsService) Destroy(ctx context.Context, boxID int64, designationID int64) error {
	op := OperationInfo{
		Service: "Designations", Operation: "DeleteBoxDesignation",
		ResourceType: "designation", IsMutation: true, ResourceID: designationID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().DeleteBoxDesignationWithResponse(ctx, boxID, designationID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}
