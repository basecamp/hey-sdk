package hey

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// MessagesService handles message operations.
type MessagesService struct {
	client *Client
}

// NewMessagesService creates a new MessagesService.
func NewMessagesService(client *Client) *MessagesService {
	return &MessagesService{client: client}
}

// Get returns a specific message by ID.
func (s *MessagesService) Get(ctx context.Context, messageID int64) (result *generated.Message, err error) {
	op := OperationInfo{
		Service: "Messages", Operation: "GetMessage",
		ResourceType: "message", IsMutation: false, ResourceID: messageID,
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
	resp, err := s.client.gen.GetMessageWithResponse(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// Create creates a new message (starts a new thread) and delivers it.
// The acting sender ID is automatically resolved.
//
// Wire format (MessagesController#create): {acting_sender_id, message: {subject, content},
// entry: {addressed: {directly: [...], copied: [...], blindcopied: [...]}}}.
// Recipient lists are JSON arrays; haystack applies Array() to each kind.
func (s *MessagesService) Create(ctx context.Context, subject, content string, to, cc, bcc []string) (err error) {
	op := OperationInfo{
		Service: "Messages", Operation: "CreateMessage",
		ResourceType: "message", IsMutation: true,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	senderID, err := s.client.DefaultSenderID(ctx)
	if err != nil {
		return err
	}

	body := map[string]any{
		"acting_sender_id": senderID,
		"message": map[string]any{
			"subject": subject,
			"content": content,
		},
		"entry": map[string]any{
			"addressed": addressedPayload(to, cc, bcc),
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	s.client.initGeneratedClient()
	resp, err := s.client.gen.CreateMessageWithBodyWithResponse(ctx, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	return CheckResponse(resp.HTTPResponse)
}

// addressedPayload builds the entry.addressed map, omitting empty kinds.
func addressedPayload(to, cc, bcc []string) map[string]any {
	addressed := map[string]any{}
	if len(to) > 0 {
		addressed["directly"] = to
	}
	if len(cc) > 0 {
		addressed["copied"] = cc
	}
	if len(bcc) > 0 {
		addressed["blindcopied"] = bcc
	}
	return addressed
}
