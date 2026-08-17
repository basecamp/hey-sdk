package hey

import (
	"context"
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
	if len(to)+len(cc)+len(bcc) == 0 {
		return ErrUsage("a message needs at least one recipient (to, cc or bcc)")
	}
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

	body := generated.CreateMessageRequestContent{
		ActingSenderId: senderID,
		Message:        generated.MessagePayload{Subject: subject, Content: content},
		Entry:          entryPayload(to, cc, bcc),
	}

	resp, err := s.client.genClient().CreateMessageWithResponse(ctx, body)
	if err != nil {
		return err
	}
	return CheckResponse(resp.HTTPResponse)
}

// entryPayload builds entry.addressed for a message or reply. Callers guarantee at
// least one recipient; it still returns nil for none so the "entry" key is omitted
// rather than sent as an empty hash.
func entryPayload(to, cc, bcc []string) *generated.MessageEntryPayload {
	if len(to)+len(cc)+len(bcc) == 0 {
		return nil
	}
	return &generated.MessageEntryPayload{
		Addressed: &generated.MessageAddressed{Directly: to, Copied: cc, Blindcopied: bcc},
	}
}
