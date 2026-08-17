package hey

import (
	"context"
	"fmt"
	"math"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// StickiesService handles the stickies board.
type StickiesService struct {
	client *Client
}

// NewStickiesService creates a new StickiesService.
func NewStickiesService(client *Client) *StickiesService {
	return &StickiesService{client: client}
}

// Sticky sizes the API accepts.
const (
	StickySmall  = "small"
	StickyMedium = "medium"
	StickyLarge  = "large"
)

// MaxStickiesLimit is the largest page the stickies index answers with. The server clamps
// anything above it, so List clamps too rather than sending a number it knows is ignored.
const MaxStickiesLimit = 100

// MaxStickyPosition is the highest board position Move accepts. The wire format carries the
// position as a 32-bit integer.
const MaxStickyPosition = math.MaxInt32

// List returns the stickies in board order. A limit of zero asks for the server default,
// which is also its maximum of 100.
func (s *StickiesService) List(ctx context.Context, limit int) (result *generated.ListStickiesResponseContent, err error) {
	op := OperationInfo{
		Service: "Stickies", Operation: "ListStickies",
		ResourceType: "sticky", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		// A nil params leaves limit out of the query entirely; sending limit=0 would be
		// clamped to one sticky rather than read as "no limit".
		var params *generated.ListStickiesParams
		if limit > 0 {
			if limit > MaxStickiesLimit {
				limit = MaxStickiesLimit
			}
			params = &generated.ListStickiesParams{Limit: int32(limit)}
		}

		resp, rerr := s.client.genClient().ListStickiesWithResponse(ctx, params)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = resp.JSON200
		return nil
	})
	return result, err
}

// Create writes a new sticky. An empty size leaves the server default in place.
func (s *StickiesService) Create(ctx context.Context, body string, size string) (result *generated.Sticky, err error) {
	op := OperationInfo{
		Service: "Stickies", Operation: "CreateSticky",
		ResourceType: "sticky", IsMutation: true,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().CreateStickyWithResponse(ctx, generated.StickyRequestContent{
			Sticky: generated.StickyPayload{Body: body, Size: size},
		})
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = resp.JSON200
		return nil
	})
	return result, err
}

// Update edits a sticky. Empty fields are left alone.
func (s *StickiesService) Update(ctx context.Context, stickyID int64, body string, size string) (result *generated.Sticky, err error) {
	op := OperationInfo{
		Service: "Stickies", Operation: "UpdateSticky",
		ResourceType: "sticky", IsMutation: true, ResourceID: stickyID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		payload := map[string]any{}
		if body != "" {
			payload["body"] = body
		}
		if size != "" {
			payload["size"] = size
		}

		resp, rerr := s.client.Patch(ctx, stickyPath(stickyID), map[string]any{"sticky": payload})
		if rerr != nil {
			return rerr
		}

		var sticky generated.Sticky
		if derr := resp.UnmarshalData(&sticky); derr != nil {
			return derr
		}
		result = &sticky
		return nil
	})
	return result, err
}

// Delete throws a sticky away.
func (s *StickiesService) Delete(ctx context.Context, stickyID int64) error {
	op := OperationInfo{
		Service: "Stickies", Operation: "DeleteSticky",
		ResourceType: "sticky", IsMutation: true, ResourceID: stickyID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.Delete(ctx, stickyPath(stickyID))
		return err
	})
}

// Move repositions a sticky on the board. Positions run from zero to MaxStickyPosition.
func (s *StickiesService) Move(ctx context.Context, stickyID int64, position int) error {
	op := OperationInfo{
		Service: "Stickies", Operation: "MoveSticky",
		ResourceType: "sticky", IsMutation: true, ResourceID: stickyID,
	}

	if position < 0 || position > MaxStickyPosition {
		return ErrUsage(fmt.Sprintf("sticky position must be between 0 and %d, got %d", MaxStickyPosition, position))
	}
	wirePosition := int32(position)

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().MoveStickyWithResponse(ctx, generated.MoveStickyRequestContent{
			Id:       stickyID,
			Position: wirePosition,
		})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// stickyPath builds the .json path Smithy cannot model, because a URI label may not carry a
// literal suffix.
func stickyPath(stickyID int64) string {
	return fmt.Sprintf("/stickies/%d.json", stickyID)
}
