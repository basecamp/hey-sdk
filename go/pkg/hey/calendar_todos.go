package hey

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// CalendarTodosService handles calendar todo operations.
type CalendarTodosService struct {
	client *Client
}

// NewCalendarTodosService creates a new CalendarTodosService.
func NewCalendarTodosService(client *Client) *CalendarTodosService {
	return &CalendarTodosService{client: client}
}

// Create creates a new calendar todo.
//
// The HEY API expects the body wrapped as {calendar_todo: {title, starts_at}}.
// If startsAt is empty, it defaults to today.
func (s *CalendarTodosService) Create(ctx context.Context, title string, startsAt string) (result *generated.Recording, err error) {
	op := OperationInfo{
		Service: "CalendarTodos", Operation: "CreateCalendarTodo",
		ResourceType: "calendar_todo", IsMutation: true,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	if startsAt == "" {
		startsAt = time.Now().Format("2006-01-02")
	}

	// starts_at goes on the wire as a bare date (YYYY-MM-DD): the server casts it
	// in the user's time zone, whereas an RFC 3339 instant at UTC midnight could
	// land on the previous day. The generated payload types it as time.Time, so
	// send the exact body through the generated route instead.
	body := map[string]any{
		"calendar_todo": map[string]any{
			"title":     title,
			"starts_at": startsAt,
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.genClient().CreateCalendarTodoWithBodyWithResponse(ctx, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// Complete marks a calendar todo as complete.
func (s *CalendarTodosService) Complete(ctx context.Context, todoID int64) (result *generated.Recording, err error) {
	op := OperationInfo{
		Service: "CalendarTodos", Operation: "CompleteCalendarTodo",
		ResourceType: "calendar_todo", IsMutation: true, ResourceID: todoID,
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
	resp, err := s.client.gen.CompleteCalendarTodoWithResponse(ctx, todoID)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// Uncomplete marks a calendar todo as incomplete.
func (s *CalendarTodosService) Uncomplete(ctx context.Context, todoID int64) (result *generated.Recording, err error) {
	op := OperationInfo{
		Service: "CalendarTodos", Operation: "UncompleteCalendarTodo",
		ResourceType: "calendar_todo", IsMutation: true, ResourceID: todoID,
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
	resp, err := s.client.gen.UncompleteCalendarTodoWithResponse(ctx, todoID)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// Delete deletes a calendar todo.
func (s *CalendarTodosService) Delete(ctx context.Context, todoID int64) (err error) {
	op := OperationInfo{
		Service: "CalendarTodos", Operation: "DeleteCalendarTodo",
		ResourceType: "calendar_todo", IsMutation: true, ResourceID: todoID,
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
	resp, err := s.client.gen.DeleteCalendarTodoWithResponse(ctx, todoID)
	if err != nil {
		return err
	}
	return CheckResponse(resp.HTTPResponse)
}
