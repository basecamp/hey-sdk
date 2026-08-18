package hey

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// BulkRepliesService sends one reply to many threads at once.
type BulkRepliesService struct {
	client *Client
}

// NewBulkRepliesService creates a new BulkRepliesService.
func NewBulkRepliesService(client *Client) *BulkRepliesService {
	return &BulkRepliesService{client: client}
}

// Draft works out which entries a bulk reply would answer, and how it starts.
//
// HEY replies to the last replyable entry of each thread and skips threads it has no
// reply address for, so the postings you hold are not the entries the reply goes to. Send
// the entries this returns — or a subset of them — to Send.
func (s *BulkRepliesService) Draft(ctx context.Context, postingIDs []int64) (draft *generated.BulkReplyDraft, err error) {
	op := OperationInfo{
		Service: "BulkReplies", Operation: "NewBulkReply",
		ResourceType: "bulk_reply", IsMutation: false,
	}

	if len(postingIDs) == 0 {
		return nil, ErrUsage("at least one posting is required")
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		params := &generated.NewBulkReplyParams{PostingIds: joinIDs(postingIDs)}

		resp, rerr := s.client.genClient().NewBulkReplyWithResponse(ctx, params)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		draft = resp.JSON200
		return nil
	})
	return draft, err
}

// Send replies to every entry with the same content, and answers what was sent.
//
// Delivery is queued. While the sender has undo enabled the send is held open, and the
// answer says so: Delayed is true and UndoSendUrl is where to call it back.
func (s *BulkRepliesService) Send(ctx context.Context, entryIDs []int64, content string) (delivery *generated.BulkReplyDelivery, err error) {
	op := OperationInfo{
		Service: "BulkReplies", Operation: "CreateBulkReply",
		ResourceType: "bulk_reply", IsMutation: true,
	}

	if len(entryIDs) == 0 {
		return nil, ErrUsage("at least one entry is required")
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		body := generated.CreateBulkReplyJSONRequestBody{
			EntryIds: entryIDs,
			Message:  generated.BulkReplyMessagePayload{Content: content},
		}

		resp, rerr := s.client.genClient().CreateBulkReplyWithResponse(ctx, body)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		delivery = resp.JSON201
		return nil
	})
	return delivery, err
}

// Undo calls back a delayed bulk reply before it goes out. It answers a usage error once
// the replies have been sent.
//
// HEY answers this one with a redirect rather than JSON — the same response its own apps
// read — so the SDK follows it instead of decoding a body.
func (s *BulkRepliesService) Undo(ctx context.Context, bulkReplyID int64) error {
	op := OperationInfo{
		Service: "BulkReplies", Operation: "UndoBulkReplySend",
		ResourceType: "bulk_reply", IsMutation: true, ResourceID: bulkReplyID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.PostForm(ctx, fmt.Sprintf("/bulk_replies/%d/undo_send", bulkReplyID), url.Values{})
		return err
	})
}

// UndoSendID reads the bulk reply id out of a delivery's undo URL, for callers holding
// the URL rather than the delivery.
func UndoSendID(undoSendURL string) (int64, error) {
	parsed, err := url.Parse(undoSendURL)
	if err != nil {
		return 0, ErrUsage(fmt.Sprintf("not a URL: %s", undoSendURL))
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "bulk_replies" {
		return 0, ErrUsage(fmt.Sprintf("not a bulk reply undo URL: %s", undoSendURL))
	}
	var id int64
	if _, err := fmt.Sscanf(parts[1], "%d", &id); err != nil {
		return 0, ErrUsage(fmt.Sprintf("not a bulk reply undo URL: %s", undoSendURL))
	}
	return id, nil
}
