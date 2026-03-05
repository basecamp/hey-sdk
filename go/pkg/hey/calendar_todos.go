package hey

import (
	"context"
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
func (s *CalendarTodosService) Create(ctx context.Context, body generated.CreateCalendarTodoJSONRequestBody) (result *generated.Recording, err error) {
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

	s.client.initGeneratedClient()
	resp, err := s.client.gen.CreateCalendarTodoWithResponse(ctx, body)
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
