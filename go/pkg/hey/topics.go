package hey

import (
	"context"
	"fmt"
	"net/http"
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
//
// The entry index is paginated by geared_pagination, so the page in params is a cursor out
// of the previous answer's Link header rather than an offset — a number is ignored and
// answered with the first page. This throws that header away; use GetEntriesPage to walk
// the thread.
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

// TopicEntryPage contains one page of a topic's entries and the cursor for the page after
// it. NextPage is empty on the last page, which is how a caller walking a thread is told
// it has read all of it.
type TopicEntryPage struct {
	Entries  []generated.Entry
	NextPage string
}

// GetEntriesPage answers the same entries as GetEntries along with the cursor for the page
// after it, so a caller walking a thread follows HEY's own ordering rather than guessing
// page numbers geared_pagination does not understand.
//
// Pass an empty page for the first page, then the NextPage of each answer.
func (s *TopicsService) GetEntriesPage(ctx context.Context, topicID int64, page string) (result *TopicEntryPage, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "GetTopicEntries",
		ResourceType: "entry", IsMutation: false, ResourceID: topicID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetTopicEntriesWithResponse(ctx, topicID, topicEntriesParams(page))
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = &TopicEntryPage{}
		if resp.JSON200 != nil {
			result.Entries = *resp.JSON200
		}
		if resp.HTTPResponse != nil {
			result.NextPage = gearedPageFromLink(resp.HTTPResponse.Header.Get("Link"))
		}
		return nil
	})
	return result, err
}

func topicEntriesParams(page string) *generated.GetTopicEntriesParams {
	params := &generated.GetTopicEntriesParams{}
	if page != "" {
		params.Page = &page
	}
	return params
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
		if awaitingRemovalConfirmation(resp.HTTPResponse, topicID) {
			return ErrUsageHint(
				fmt.Sprintf("topic %d is shared; HEY wants confirmation before trashing it", topicID),
				"Call Trash with confirmDestroy=true to trash it and remove your access")
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// awaitingRemovalConfirmation reports whether HEY answered a trash request by
// redirecting to the shared-topic removal confirmation page for that topic
// (/topics/:id/removal/new). The client follows the redirect, so it shows up as a
// non-2xx response whose final request URL is that page.
func awaitingRemovalConfirmation(resp *http.Response, topicID int64) bool {
	if resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return false
	}
	return resp.StatusCode >= 300 && resp.Request.URL.Path == fmt.Sprintf("/topics/%d/removal/new", topicID)
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
