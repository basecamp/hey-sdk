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

// --- Habit CRUD ---
//
// Create and Update answer the written habit as a recording: HEY renders it as JSON on
// create and update (the JSON branches added in haystack #8623). There is no redirect
// fallback: a non-2xx answer is an error, so a caller never sees a "success" that
// wrote nothing.

// HabitParams describes a habit. Days are 0 for Sunday through 6 for Saturday.
type HabitParams struct {
	Name  string
	Icon  string
	Color string
	Days  []int32
}

// Create starts a new habit and returns it as a recording.
func (s *HabitsService) Create(ctx context.Context, params HabitParams) (recording *generated.Recording, err error) {
	op := OperationInfo{
		Service: "Habits", Operation: "CreateHabit",
		ResourceType: "habit", IsMutation: true,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().CreateHabitWithResponse(ctx, habitBody(params))
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		recording = resp.JSON201
		return nil
	})
	return recording, err
}

// Update edits a habit and returns it as a recording. habitID is the recording's id.
// Empty fields are left alone.
func (s *HabitsService) Update(ctx context.Context, habitID int64, params HabitParams) (recording *generated.Recording, err error) {
	op := OperationInfo{
		Service: "Habits", Operation: "UpdateHabit",
		ResourceType: "habit", IsMutation: true, ResourceID: habitID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().UpdateHabitWithResponse(ctx, habitID, habitBody(params))
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		recording = resp.JSON200
		return nil
	})
	return recording, err
}

// Delete throws a habit away, along with its history. habitID is the recording's id.
func (s *HabitsService) Delete(ctx context.Context, habitID int64) error {
	op := OperationInfo{
		Service: "Habits", Operation: "DeleteHabit",
		ResourceType: "habit", IsMutation: true, ResourceID: habitID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().DeleteHabitWithResponse(ctx, habitID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Stop pauses a habit, keeping its history but taking it off the calendar.
func (s *HabitsService) Stop(ctx context.Context, habitID int64) error {
	op := OperationInfo{
		Service: "Habits", Operation: "StopHabit",
		ResourceType: "habit", IsMutation: true, ResourceID: habitID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().StopHabitWithResponse(ctx, habitID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Resume puts a paused habit back on the calendar.
func (s *HabitsService) Resume(ctx context.Context, habitID int64) error {
	op := OperationInfo{
		Service: "Habits", Operation: "ResumeHabit",
		ResourceType: "habit", IsMutation: true, ResourceID: habitID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().ResumeHabitWithResponse(ctx, habitID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// habitBody builds the {calendar_habit: {...}} wrapper, leaving out what wasn't set.
func habitBody(params HabitParams) generated.HabitRequestContent {
	return generated.HabitRequestContent{
		CalendarHabit: generated.HabitPayload{
			Name:  params.Name,
			Icon:  params.Icon,
			Color: params.Color,
			Days:  params.Days,
		},
	}
}
