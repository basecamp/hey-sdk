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

// Create creates a new message.
func (s *MessagesService) Create(ctx context.Context, body generated.CreateMessageJSONRequestBody) (result *generated.SentResponse, err error) {
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

	s.client.initGeneratedClient()
	resp, err := s.client.gen.CreateMessageWithResponse(ctx, body)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// CreateTopicMessage creates a message within a topic.
func (s *MessagesService) CreateTopicMessage(ctx context.Context, topicID int64, body generated.CreateTopicMessageJSONRequestBody) (result *generated.SentResponse, err error) {
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

	s.client.initGeneratedClient()
	resp, err := s.client.gen.CreateTopicMessageWithResponse(ctx, topicID, body)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}
