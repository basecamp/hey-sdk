package hey

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// EntriesService handles draft and reply operations.
type EntriesService struct {
	client *Client
}

// NewEntriesService creates a new EntriesService.
func NewEntriesService(client *Client) *EntriesService {
	return &EntriesService{client: client}
}

// ListDrafts returns all draft messages.
func (s *EntriesService) ListDrafts(ctx context.Context, params *generated.ListDraftsParams) (result *generated.ListDraftsResponseContent, err error) {
	op := OperationInfo{
		Service: "Entries", Operation: "ListDrafts",
		ResourceType: "draft", IsMutation: false,
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
	resp, err := s.client.gen.ListDraftsWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// CreateReply creates a reply to an entry.
// The acting sender ID is automatically resolved.
func (s *EntriesService) CreateReply(ctx context.Context, entryID int64, content string, to, cc, bcc []string) (err error) {
	op := OperationInfo{
		Service: "Entries", Operation: "CreateReply",
		ResourceType: "reply", IsMutation: true, ResourceID: entryID,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	senderID, err := s.client.DefaultSenderID(ctx)
	if err != nil {
		return err
	}

	body := map[string]any{
		"acting_sender_id": senderID,
		"message": map[string]any{
			"content": content,
		},
	}
	addressed := map[string]any{}
	if len(to) > 0 {
		addressed["directly"] = strings.Join(to, ",")
	}
	if len(cc) > 0 {
		addressed["copied"] = strings.Join(cc, ",")
	}
	if len(bcc) > 0 {
		addressed["blindcopied"] = strings.Join(bcc, ",")
	}
	if len(addressed) > 0 {
		body["entry"] = map[string]any{
			"addressed": addressed,
		}
	}

	_, err = s.client.PostMutation(ctx, fmt.Sprintf("/entries/%d/replies.json", entryID), body)
	return err
}
