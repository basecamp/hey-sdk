package hey

import (
	"context"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// TopicsService handles topic operations.
type TopicsService struct {
	client *Client
}

// NewTopicsService creates a new TopicsService.
func NewTopicsService(client *Client) *TopicsService {
	return &TopicsService{client: client}
}

// Get returns a specific topic by ID.
func (s *TopicsService) Get(ctx context.Context, topicID int64) (result *generated.Topic, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "GetTopic",
		ResourceType: "topic", IsMutation: false, ResourceID: topicID,
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
	resp, err := s.client.gen.GetTopicWithResponse(ctx, topicID)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// GetEntries returns entries for a specific topic.
func (s *TopicsService) GetEntries(ctx context.Context, topicID int64, params *generated.GetTopicEntriesParams) (result *generated.GetTopicEntriesResponseContent, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "GetTopicEntries",
		ResourceType: "entry", IsMutation: false, ResourceID: topicID,
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
	resp, err := s.client.gen.GetTopicEntriesWithResponse(ctx, topicID, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// GetSent returns sent topics.
func (s *TopicsService) GetSent(ctx context.Context, params *generated.GetSentTopicsParams) (result *generated.TopicListResponse, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "GetSentTopics",
		ResourceType: "topic", IsMutation: false,
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
	resp, err := s.client.gen.GetSentTopicsWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// GetSpam returns spam topics.
func (s *TopicsService) GetSpam(ctx context.Context, params *generated.GetSpamTopicsParams) (result *generated.TopicListResponse, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "GetSpamTopics",
		ResourceType: "topic", IsMutation: false,
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
	resp, err := s.client.gen.GetSpamTopicsWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// GetTrash returns trash topics.
func (s *TopicsService) GetTrash(ctx context.Context, params *generated.GetTrashTopicsParams) (result *generated.TopicListResponse, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "GetTrashTopics",
		ResourceType: "topic", IsMutation: false,
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
	resp, err := s.client.gen.GetTrashTopicsWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// GetEverything returns all topics.
func (s *TopicsService) GetEverything(ctx context.Context, params *generated.GetEverythingTopicsParams) (result *generated.TopicListResponse, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "GetEverythingTopics",
		ResourceType: "topic", IsMutation: false,
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
	resp, err := s.client.gen.GetEverythingTopicsWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}
