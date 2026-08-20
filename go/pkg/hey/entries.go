package hey

import (
	"context"
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

// CreateReply replies to an entry (POST /entries/{entryId}/replies.json) and delivers it.
// The acting sender ID is automatically resolved.
//
// Recipients are required. HEY does not reply-all on the caller's behalf: a reply
// posted without entry.addressed is saved as a draft (the server answers with a
// redirect to the thread with the draft expanded) rather than delivered. Callers
// resolve the thread's recipients first — hey-cli reads them from the topic page.
func (s *EntriesService) CreateReply(ctx context.Context, entryID int64, content string, to, cc, bcc []string) (err error) {
	if len(to)+len(cc)+len(bcc) == 0 {
		return ErrUsage("a reply needs at least one recipient (to, cc or bcc); HEY saves an unaddressed reply as a draft")
	}
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

	body := generated.CreateReplyRequestContent{
		ActingSenderId: senderID,
		Message:        generated.ReplyMessagePayload{Content: content},
		Entry:          entryPayload(to, cc, bcc),
	}

	resp, err := s.client.genClient().CreateReplyWithResponse(ctx, entryID, body)
	if err != nil {
		return err
	}
	return CheckResponse(resp.HTTPResponse)
}

// MarkSpam marks an entry as spam. The server denies the sender outright when every thread
// from them is already spam.
func (s *EntriesService) MarkSpam(ctx context.Context, entryID int64) error {
	op := OperationInfo{
		Service: "Entries", Operation: "MarkEntrySpam",
		ResourceType: "entry", IsMutation: true, ResourceID: entryID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().MarkEntrySpamWithResponse(ctx, entryID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// NewReply returns the live reply envelope HEY prepared for an entry, including the
// resolved recipients. Fill in the content and send it with CreateReply.
func (s *EntriesService) NewReply(ctx context.Context, entryID int64) (result *generated.MessageDraft, err error) {
	op := OperationInfo{
		Service: "Entries", Operation: "NewEntryReply",
		ResourceType: "entry", IsMutation: false, ResourceID: entryID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().NewEntryReplyWithResponse(ctx, entryID)
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

// NewForward returns a prefilled forward of an entry: the "Fwd:" subject, the quoted body and
// blank recipients. Fill in the recipients and send it with MessagesService.Create.
func (s *EntriesService) NewForward(ctx context.Context, entryID int64) (result *generated.MessageDraft, err error) {
	op := OperationInfo{
		Service: "Entries", Operation: "NewEntryForward",
		ResourceType: "entry", IsMutation: false, ResourceID: entryID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().NewEntryForwardWithResponse(ctx, entryID)
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
