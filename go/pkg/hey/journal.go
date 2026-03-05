package hey

import (
	"context"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// JournalService handles journal entry operations.
type JournalService struct {
	client *Client
}

// NewJournalService creates a new JournalService.
func NewJournalService(client *Client) *JournalService {
	return &JournalService{client: client}
}

// Get returns a journal entry for a specific day (YYYY-MM-DD format).
func (s *JournalService) Get(ctx context.Context, day string) (result *generated.Recording, err error) {
	op := OperationInfo{
		Service: "Journal", Operation: "GetJournalEntry",
		ResourceType: "journal_entry", IsMutation: false,
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
	resp, err := s.client.gen.GetJournalEntryWithResponse(ctx, day)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// Update updates a journal entry for a specific day.
func (s *JournalService) Update(ctx context.Context, day string, body generated.UpdateJournalEntryJSONRequestBody) (result *generated.Recording, err error) {
	op := OperationInfo{
		Service: "Journal", Operation: "UpdateJournalEntry",
		ResourceType: "journal_entry", IsMutation: true,
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
	resp, err := s.client.gen.UpdateJournalEntryWithResponse(ctx, day, body)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}
