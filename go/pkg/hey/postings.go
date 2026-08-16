package hey

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// Box kinds as reported by ListBoxes. Use with MoveToBox.
const (
	BoxKindImbox    = "imbox"
	BoxKindFeed     = "feedbox"
	BoxKindSetAside = "asidebox"
	BoxKindLater    = "laterbox"
	BoxKindTrail    = "trailbox"
	BoxKindBubbleUp = "bubblebox"
)

// PostingsService handles posting-level actions (seen, move, trash, mute).
//
// HEY exposes these as bulk endpoints that take a list of posting IDs, so every
// method here accepts one or more IDs.
type PostingsService struct {
	client *Client
}

// NewPostingsService creates a new PostingsService.
func NewPostingsService(client *Client) *PostingsService {
	return &PostingsService{client: client}
}

// MarkSeen marks one or more postings as seen/read.
func (s *PostingsService) MarkSeen(ctx context.Context, postingIDs []int64) (err error) {
	return s.bulkAction(ctx, "MarkPostingsSeen", postingIDs, func(ctx context.Context, ids []int64) error {
		resp, err := s.genClient().MarkPostingsSeenWithResponse(ctx, generated.MarkPostingsRequestContent{PostingIds: ids})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// MarkUnseen marks one or more postings as unseen/unread.
func (s *PostingsService) MarkUnseen(ctx context.Context, postingIDs []int64) (err error) {
	return s.bulkAction(ctx, "MarkPostingsUnseen", postingIDs, func(ctx context.Context, ids []int64) error {
		resp, err := s.genClient().MarkPostingsUnseenWithResponse(ctx, generated.MarkPostingsRequestContent{PostingIds: ids})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Move moves one or more postings to the box with the given ID
// (POST /postings/moves). Box IDs come from Boxes().List.
func (s *PostingsService) Move(ctx context.Context, boxID int64, postingIDs ...int64) (err error) {
	return s.bulkAction(ctx, "MovePostings", postingIDs, func(ctx context.Context, ids []int64) error {
		resp, err := s.genClient().MovePostingsWithResponse(ctx, generated.MovePostingsRequestContent{PostingIds: ids, BoxId: boxID})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// MoveToBox moves one or more postings to the box of the given kind
// (BoxKindImbox, BoxKindFeed, ...). The kind→ID mapping is resolved once via
// ListBoxes and cached on the client.
func (s *PostingsService) MoveToBox(ctx context.Context, kind string, postingIDs ...int64) error {
	boxID, err := s.client.BoxIDByKind(ctx, kind)
	if err != nil {
		return err
	}
	return s.Move(ctx, boxID, postingIDs...)
}

// MoveToImbox moves postings to the Imbox.
func (s *PostingsService) MoveToImbox(ctx context.Context, postingIDs ...int64) error {
	return s.MoveToBox(ctx, BoxKindImbox, postingIDs...)
}

// MoveToFeed moves postings to The Feed.
func (s *PostingsService) MoveToFeed(ctx context.Context, postingIDs ...int64) error {
	return s.MoveToBox(ctx, BoxKindFeed, postingIDs...)
}

// MoveToSetAside moves postings to Set Aside.
func (s *PostingsService) MoveToSetAside(ctx context.Context, postingIDs ...int64) error {
	return s.MoveToBox(ctx, BoxKindSetAside, postingIDs...)
}

// MoveToReplyLater moves postings to Reply Later.
func (s *PostingsService) MoveToReplyLater(ctx context.Context, postingIDs ...int64) error {
	return s.MoveToBox(ctx, BoxKindLater, postingIDs...)
}

// MoveToPaperTrail moves postings to the Paper Trail.
func (s *PostingsService) MoveToPaperTrail(ctx context.Context, postingIDs ...int64) error {
	return s.MoveToBox(ctx, BoxKindTrail, postingIDs...)
}

// MoveToTrash moves one or more postings to the trash (POST /postings/trash).
// For shared topics HEY removes your access rather than trashing for everyone.
func (s *PostingsService) MoveToTrash(ctx context.Context, postingIDs ...int64) (err error) {
	return s.bulkAction(ctx, "TrashPostings", postingIDs, func(ctx context.Context, ids []int64) error {
		resp, err := s.genClient().TrashPostingsWithResponse(ctx, generated.MarkPostingsRequestContent{PostingIds: ids})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Mute mutes one or more postings so their threads stop notifying
// (POST /postings/mutings).
func (s *PostingsService) Mute(ctx context.Context, postingIDs ...int64) (err error) {
	return s.bulkAction(ctx, "MutePostings", postingIDs, func(ctx context.Context, ids []int64) error {
		resp, err := s.genClient().MutePostingsWithResponse(ctx, generated.MarkPostingsRequestContent{PostingIds: ids})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Unmute unmutes one or more postings (DELETE /postings/mutings).
func (s *PostingsService) Unmute(ctx context.Context, postingIDs ...int64) (err error) {
	return s.bulkAction(ctx, "UnmutePostings", postingIDs, func(ctx context.Context, ids []int64) error {
		params := &generated.UnmutePostingsParams{PostingIds: joinIDs(ids)}
		resp, err := s.genClient().UnmutePostingsWithResponse(ctx, params)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// BoxIDByKind returns the ID of the caller's box with the given kind
// ("imbox", "feedbox", "asidebox", "laterbox", "trailbox", "bubblebox"),
// resolving via ListBoxes on first use and caching the result.
func (c *Client) BoxIDByKind(ctx context.Context, kind string) (int64, error) {
	c.boxMu.Lock()
	defer c.boxMu.Unlock()

	if id, ok := c.boxByKind[kind]; ok {
		return id, nil
	}

	boxes, err := c.Boxes().List(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list boxes to resolve %q: %w", kind, err)
	}
	if boxes == nil {
		return 0, ErrAPI(0, "could not list boxes")
	}
	byKind := make(map[string]int64, len(*boxes))
	for _, b := range *boxes {
		if b.Kind != "" {
			byKind[b.Kind] = b.Id
		}
	}
	c.boxByKind = byKind

	id, ok := byKind[kind]
	if !ok {
		return 0, ErrAPI(0, fmt.Sprintf("no box of kind %q", kind))
	}
	return id, nil
}

func (s *PostingsService) genClient() *generated.ClientWithResponses {
	s.client.initGeneratedClient()
	return s.client.gen
}

func (s *PostingsService) bulkAction(ctx context.Context, operation string, ids []int64, fn func(context.Context, []int64) error) (err error) {
	if len(ids) == 0 {
		return ErrUsage("at least one posting ID is required")
	}
	op := OperationInfo{
		Service: "Postings", Operation: operation,
		ResourceType: "posting", IsMutation: true,
	}
	if len(ids) == 1 {
		op.ResourceID = ids[0]
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	return fn(ctx, ids)
}

func joinIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ",")
}
