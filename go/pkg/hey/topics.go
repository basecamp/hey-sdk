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

// --- Status and moves ---

// Trash moves a topic to the trash.
//
// Shared topics redirect to a confirmation page unless confirmDestroy is set, so pass it
// whenever the topic might be shared.
func (s *TopicsService) Trash(ctx context.Context, topicID int64, confirmDestroy bool) error {
	op := OperationInfo{
		Service: "Topics", Operation: "TrashTopic",
		ResourceType: "topic", IsMutation: true, ResourceID: topicID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		// confirm_destroy is only sent when set: an empty value would read as
		// truthy on the server and skip the shared-topic confirmation.
		var params generated.TrashTopicParams
		if confirmDestroy {
			one := "1"
			params.ConfirmDestroy = &one
		}
		resp, err := s.client.genClient().TrashTopicWithResponse(ctx, topicID, &params)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Restore brings a topic back from the trash or the catch-all.
func (s *TopicsService) Restore(ctx context.Context, topicID int64) error {
	op := OperationInfo{
		Service: "Topics", Operation: "RestoreTopic",
		ResourceType: "topic", IsMutation: true, ResourceID: topicID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().RestoreTopicWithResponse(ctx, topicID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// MarkHam rescues a topic from spam. Every other spam topic from the same sender comes with it.
func (s *TopicsService) MarkHam(ctx context.Context, topicID int64) error {
	op := OperationInfo{
		Service: "Topics", Operation: "MarkTopicHam",
		ResourceType: "topic", IsMutation: true, ResourceID: topicID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().MarkTopicHamWithResponse(ctx, topicID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// EmptyTrash deletes everything in the trash. The server does this synchronously, so a large
// trash can take a while.
func (s *TopicsService) EmptyTrash(ctx context.Context) error {
	op := OperationInfo{
		Service: "Topics", Operation: "EmptyTrash",
		ResourceType: "topic", IsMutation: true,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().EmptyTrashWithResponse(ctx)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// EmptySpam deletes everything in the spam box. The server does this synchronously, so a large
// spam box can take a while.
func (s *TopicsService) EmptySpam(ctx context.Context) error {
	op := OperationInfo{
		Service: "Topics", Operation: "EmptySpam",
		ResourceType: "topic", IsMutation: true,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().EmptySpamWithResponse(ctx)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Move moves a topic to another box.
//
// The server answers 204 without moving anything when the acting user has no posting for
// the topic, so a success here is not proof the topic moved.
func (s *TopicsService) Move(ctx context.Context, topicID int64, boxID int64) error {
	op := OperationInfo{
		Service: "Topics", Operation: "MoveTopic",
		ResourceType: "topic", IsMutation: true, ResourceID: topicID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().MoveTopicWithResponse(ctx, topicID, generated.MoveTopicRequestContent{BoxId: boxID})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}
