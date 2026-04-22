package hey

import (
	"context"
	"fmt"
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

// Create creates a new habit.
//
// The HEY API expects the body wrapped as {calendar_habit: {title, days}}.
// Pass nil or an empty slice for days to omit it from the request; the server
// then applies its default (all days). A non-empty slice is sent verbatim.
func (s *HabitsService) Create(ctx context.Context, title string, days []int32) (result *generated.Recording, err error) {
	op := OperationInfo{
		Service: "Habits", Operation: "CreateHabit",
		ResourceType: "calendar_habit", IsMutation: true,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	habit := map[string]any{"title": title}
	if len(days) > 0 {
		habit["days"] = days
	}
	body := map[string]any{"calendar_habit": habit}

	resp, err := s.client.Post(ctx, "/calendar/habits.json", body)
	if err != nil {
		return nil, err
	}
	var recording generated.Recording
	if err = resp.UnmarshalData(&recording); err != nil {
		return nil, fmt.Errorf("failed to decode habit response: %w", err)
	}
	return &recording, nil
}

// Delete deletes a habit.
func (s *HabitsService) Delete(ctx context.Context, habitID int64) (err error) {
	op := OperationInfo{
		Service: "Habits", Operation: "DeleteHabit",
		ResourceType: "calendar_habit", IsMutation: true, ResourceID: habitID,
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
	resp, err := s.client.gen.DeleteHabitWithResponse(ctx, habitID)
	if err != nil {
		return err
	}
	err = CheckResponse(resp.HTTPResponse)
	return err
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
