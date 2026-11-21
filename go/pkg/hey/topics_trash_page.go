package hey

import (
	"context"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// TrashPage holds one page of trashed topics and its opaque continuation cursor.
type TrashPage struct {
	Topics   *generated.TopicListResponse `json:"topics"`
	NextPage string                       `json:"next_page"`
}

// GetTrashPage returns trashed topics without discarding geared pagination's Link header.
func (s *TopicsService) GetTrashPage(ctx context.Context, params *generated.GetTrashTopicsParams) (result *TrashPage, err error) {
	op := OperationInfo{Service: "Topics", Operation: "GetTrashTopics", ResourceType: "topic", IsMutation: false}
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetTrashTopicsWithResponse(ctx, params)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = &TrashPage{Topics: resp.JSON200}
		if resp.HTTPResponse != nil {
			result.NextPage = gearedPageFromLink(resp.HTTPResponse.Header.Get("Link"))
		}
		return nil
	})
	return result, err
}
