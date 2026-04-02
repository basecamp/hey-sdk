package hey

import (
	"context"
	"fmt"
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

// Create creates a new message. The acting sender ID is automatically resolved.
//
// The HEY API expects a nested body with acting_sender_id, message (subject/content),
// and entry (addressed recipients). This method constructs the correct shape.
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

	body := map[string]any{
		"acting_sender_id": senderID,
		"message": map[string]any{
			"subject": subject,
			"content": content,
		},
		"entry": map[string]any{
			"addressed": addressed,
		},
	}

	_, err = s.client.PostMutation(ctx, "/messages.json", body)
	return err
}

// CreateTopicMessage creates a message within a topic (reply to a thread).
// The acting sender ID is automatically resolved.
func (s *MessagesService) CreateTopicMessage(ctx context.Context, topicID int64, content string) (err error) {
	op := OperationInfo{
		Service: "Messages", Operation: "CreateTopicMessage",
		ResourceType: "message", IsMutation: true, ResourceID: topicID,
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
			"content": content,
		},
	}

	_, err = s.client.PostMutation(ctx, fmt.Sprintf("/topics/%d/entries.json", topicID), body)
	return err
}
