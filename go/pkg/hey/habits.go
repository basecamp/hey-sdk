package hey

import (
	"context"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// HabitsService handles habit tracking operations.
type HabitsService struct {
	client *Client
}

// NewHabitsService creates a new HabitsService.
func NewHabitsService(client *Client) *HabitsService {
	return &HabitsService{client: client}
}

// Complete marks a habit as complete for a given day.
func (s *HabitsService) Complete(ctx context.Context, day string, habitID int64) (result *generated.Recording, err error) {
	op := OperationInfo{
		Service: "Habits", Operation: "CompleteHabit",
		ResourceType: "habit", IsMutation: true, ResourceID: habitID,
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
	resp, err := s.client.gen.CompleteHabitWithResponse(ctx, day, habitID)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// Uncomplete marks a habit as incomplete for a given day.
func (s *HabitsService) Uncomplete(ctx context.Context, day string, habitID int64) (result *generated.Recording, err error) {
	op := OperationInfo{
		Service: "Habits", Operation: "UncompleteHabit",
		ResourceType: "habit", IsMutation: true, ResourceID: habitID,
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
	resp, err := s.client.gen.UncompleteHabitWithResponse(ctx, day, habitID)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}
