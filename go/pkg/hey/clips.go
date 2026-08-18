package hey

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// ClipsService handles clips — snippets of a message you saved for later.
//
// Clips have no JSON surface: writes answer with a Turbo Stream and the list is HTML, so the
// list is read off the page.
type ClipsService struct {
	client *Client
}

// NewClipsService creates a new ClipsService.
func NewClipsService(client *Client) *ClipsService {
	return &ClipsService{client: client}
}

// List returns the clips, newest first, as HEY renders them (content, the entry
// they were taken from, and the topic).
func (s *ClipsService) List(ctx context.Context) (result []generated.Clip, err error) {
	op := OperationInfo{
		Service: "Clips", Operation: "ListClips",
		ResourceType: "clip", IsMutation: false,
	}
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().ListClipsWithResponse(ctx, nil)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		if resp.JSON200 != nil {
			result = *resp.JSON200
		}
		return nil
	})
	return result, err
}

func (s *ClipsService) Create(ctx context.Context, entryID int64, content string) error {
	op := OperationInfo{
		Service: "Clips", Operation: "CreateClip",
		ResourceType: "clip", IsMutation: true, ResourceID: entryID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		values := url.Values{}
		values.Set("clip[entry_id]", strconv.FormatInt(entryID, 10))
		values.Set("clip[content]", content)

		_, err := s.client.PostForm(ctx, "/clips", values)
		return err
	})
}

// Delete throws a clip away.
func (s *ClipsService) Delete(ctx context.Context, clipID int64) error {
	op := OperationInfo{
		Service: "Clips", Operation: "DeleteClip",
		ResourceType: "clip", IsMutation: true, ResourceID: clipID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.DeleteForm(ctx, fmt.Sprintf("/clips/%d", clipID))
		return err
	})
}

// clipIDRe pulls the clip id out of the article id the clips page stamps, e.g. clip_9182.
