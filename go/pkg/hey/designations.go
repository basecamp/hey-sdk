package hey

import (
	"context"
	"fmt"
	"time"
)

// DesignationsService handles moving contacts between mailboxes.
type DesignationsService struct {
	client *Client
}

// NewDesignationsService creates a new DesignationsService.
func NewDesignationsService(client *Client) *DesignationsService {
	return &DesignationsService{client: client}
}

// Create moves a contact to the specified box.
// boxID is the ID of the target box (imbox, feedbox, trailbox, etc.).
// contactID is the ID of the contact to move.
func (s *DesignationsService) Create(ctx context.Context, boxID int64, contactID int64) (err error) {
	op := OperationInfo{
		Service: "Designations", Operation: "CreateDesignation",
		ResourceType: "designation", IsMutation: true, ResourceID: boxID,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	body := map[string]any{
		"contact_id": contactID,
	}

	_, err = s.client.PostMutation(ctx, fmt.Sprintf("/boxes/%d/designations.json", boxID), body)
	return err
}

// Destroy removes a contact's designation from a box.
// boxID is the ID of the box to remove the contact from.
// designationID is the ID of the designation to remove.
func (s *DesignationsService) Destroy(ctx context.Context, boxID int64, designationID int64) (err error) {
	op := OperationInfo{
		Service: "Designations", Operation: "DestroyDesignation",
		ResourceType: "designation", IsMutation: true, ResourceID: designationID,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	_, err = s.client.Delete(ctx, fmt.Sprintf("/boxes/%d/designations/%d.json", boxID, designationID))
	return err
}
