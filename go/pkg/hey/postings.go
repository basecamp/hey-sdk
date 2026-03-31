package hey

import (
	"context"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// PostingsService handles posting-level actions (move, seen, trash, etc.).
type PostingsService struct {
	client *Client
}

// NewPostingsService creates a new PostingsService.
func NewPostingsService(client *Client) *PostingsService {
	return &PostingsService{client: client}
}

// MarkSeen marks one or more postings as seen/read.
func (s *PostingsService) MarkSeen(ctx context.Context, postingIDs []int64) (err error) {
	op := OperationInfo{
		Service: "Postings", Operation: "MarkPostingsSeen",
		ResourceType: "posting", IsMutation: true,
	}
	return s.bulkAction(ctx, op, postingIDs, func(ctx context.Context, body generated.MarkPostingsRequestContent) error {
		resp, err := s.genClient().MarkPostingsSeenWithResponse(ctx, body)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// MarkUnseen marks one or more postings as unseen/unread.
func (s *PostingsService) MarkUnseen(ctx context.Context, postingIDs []int64) (err error) {
	op := OperationInfo{
		Service: "Postings", Operation: "MarkPostingsUnseen",
		ResourceType: "posting", IsMutation: true,
	}
	return s.bulkAction(ctx, op, postingIDs, func(ctx context.Context, body generated.MarkPostingsRequestContent) error {
		resp, err := s.genClient().MarkPostingsUnseenWithResponse(ctx, body)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// MoveToFeed moves a posting to The Feed.
func (s *PostingsService) MoveToFeed(ctx context.Context, postingID int64) error {
	return s.singleAction(ctx, "MovePostingToFeed", postingID, func(ctx context.Context, id int64) error {
		resp, err := s.genClient().MovePostingToFeedWithResponse(ctx, id)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// MoveToSetAside moves a posting to Set Aside.
func (s *PostingsService) MoveToSetAside(ctx context.Context, postingID int64) error {
	return s.singleAction(ctx, "MovePostingToSetAside", postingID, func(ctx context.Context, id int64) error {
		resp, err := s.genClient().MovePostingToSetAsideWithResponse(ctx, id)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// MoveToReplyLater moves a posting to Reply Later.
func (s *PostingsService) MoveToReplyLater(ctx context.Context, postingID int64) error {
	return s.singleAction(ctx, "MovePostingToReplyLater", postingID, func(ctx context.Context, id int64) error {
		resp, err := s.genClient().MovePostingToReplyLaterWithResponse(ctx, id)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// MoveToPaperTrail moves a posting to the Paper Trail.
func (s *PostingsService) MoveToPaperTrail(ctx context.Context, postingID int64) error {
	return s.singleAction(ctx, "MovePostingToPaperTrail", postingID, func(ctx context.Context, id int64) error {
		resp, err := s.genClient().MovePostingToPaperTrailWithResponse(ctx, id)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// MoveToTrash moves a posting to the trash.
func (s *PostingsService) MoveToTrash(ctx context.Context, postingID int64) error {
	return s.singleAction(ctx, "MovePostingToTrash", postingID, func(ctx context.Context, id int64) error {
		resp, err := s.genClient().MovePostingToTrashWithResponse(ctx, id)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Ignore ignores a posting (stops notifications).
func (s *PostingsService) Ignore(ctx context.Context, postingID int64) error {
	return s.singleAction(ctx, "IgnorePosting", postingID, func(ctx context.Context, id int64) error {
		resp, err := s.genClient().IgnorePostingWithResponse(ctx, id)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// --- Helpers ---

func (s *PostingsService) genClient() *generated.ClientWithResponses {
	s.client.initGeneratedClient()
	return s.client.gen
}

func (s *PostingsService) singleAction(ctx context.Context, operation string, postingID int64, fn func(context.Context, int64) error) (err error) {
	op := OperationInfo{
		Service: "Postings", Operation: operation,
		ResourceType: "posting", IsMutation: true, ResourceID: postingID,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	return fn(ctx, postingID)
}

func (s *PostingsService) bulkAction(ctx context.Context, op OperationInfo, ids []int64, fn func(context.Context, generated.MarkPostingsRequestContent) error) (err error) {
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	body := generated.MarkPostingsRequestContent{PostingIds: ids}
	return fn(ctx, body)
}
