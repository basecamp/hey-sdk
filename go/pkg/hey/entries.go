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
//
// ActingSenderID is the identity the reply goes out as. HEY resolves a reply's sender
// from the thread — on a shared or alternate address (a HEY for Work support address,
// an extension, a forwarded external account) that is not the account default — and
// hands it back as NewReply's Sender. Pass that prefill's Sender.Id; zero falls back
// to the account's default sender, which on such a thread is the wrong identity.
//
// Subject is the reply's own subject line — HEY does not derive one, so pass the
// prefilled "Re: …" that NewReply hands back. Empty leaves it off the wire.
//
// Recipients are required. HEY does not reply-all on the caller's behalf: a reply
// posted without entry.addressed is saved as a draft (the server answers with a
// redirect to the thread with the draft expanded) rather than delivered. Callers
// resolve the thread's recipients first — hey-cli reads them from the topic page.
func (s *EntriesService) CreateReply(ctx context.Context, entryID, actingSenderID int64, subject, content string, to, cc, bcc []string) (err error) {
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

	senderID, err := s.resolveActingSenderID(ctx, actingSenderID)
	if err != nil {
		return err
	}

	body := generated.CreateReplyRequestContent{
		ActingSenderId: senderID,
		Message:        generated.ReplyMessagePayload{Subject: subject, Content: content},
		Entry:          entryPayload(to, cc, bcc),
	}

	resp, err := s.client.genClient().CreateReplyWithResponse(ctx, entryID, body)
	if err != nil {
		return err
	}
	return CheckResponse(resp.HTTPResponse)
}

// resolveActingSenderID answers the acting sender a reply is sent as: the caller's
// choice when one was made, the account's default sender otherwise.
func (s *EntriesService) resolveActingSenderID(ctx context.Context, actingSenderID int64) (int64, error) {
	if actingSenderID > 0 {
		return actingSenderID, nil
	}
	return s.client.DefaultSenderID(ctx)
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

// DraftPage is one page of drafts along with the cursor for the page after it. The
// index pages by geared_pagination's opaque cursor out of the Link header — a page
// number is answered with the first page forever — so NextPage is the only way to walk
// it; empty means the last page.
type DraftPage struct {
	Drafts   []generated.DraftMessage
	NextPage string
}

// ListDraftsPage answers the same drafts as ListDrafts along with the cursor for the
// page after them.
func (s *EntriesService) ListDraftsPage(ctx context.Context, page string) (result *DraftPage, err error) {
	op := OperationInfo{
		Service: "Entries", Operation: "ListDrafts",
		ResourceType: "draft", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		params := &generated.ListDraftsParams{}
		if page != "" {
			params.Page = &page
		}

		resp, rerr := s.client.genClient().ListDraftsWithResponse(ctx, params)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = &DraftPage{}
		if resp.JSON200 != nil {
			result.Drafts = *resp.JSON200
		}
		if resp.HTTPResponse != nil {
			result.NextPage = gearedPageFromLink(resp.HTTPResponse.Header.Get("Link"))
		}
		return nil
	})
	return result, err
}

// DeleteDraft trashes a draft by its entry id, as ListDrafts reports it.
func (s *EntriesService) DeleteDraft(ctx context.Context, entryID int64) error {
	op := OperationInfo{
		Service: "Entries", Operation: "DeleteDraft",
		ResourceType: "draft", IsMutation: true, ResourceID: entryID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().DeleteDraftWithResponse(ctx, entryID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// CreateReplyDraft saves a reply to an entry as a draft instead of delivering it, and
// answers the draft's entry id. Unlike CreateReply it needs no recipients — HEY keeps
// whatever the draft carries.
//
// ActingSenderID works as it does on CreateReply: pass the prefill's Sender.Id from
// NewReply so the draft is saved under the identity HEY resolved for the thread; zero
// falls back to the account's default sender.
//
// Subject matters here: HEY does not derive one, and a reply draft saved without it
// shows as "No subject" in Drafts. Pass the prefilled "Re: …" from NewReply. Empty
// leaves it off the wire.
func (s *EntriesService) CreateReplyDraft(ctx context.Context, entryID, actingSenderID int64, subject, content string, to, cc, bcc []string) (draftEntryID int64, err error) {
	op := OperationInfo{
		Service: "Entries", Operation: "CreateReply",
		ResourceType: "draft", IsMutation: true, ResourceID: entryID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		senderID, serr := s.resolveActingSenderID(ctx, actingSenderID)
		if serr != nil {
			return serr
		}

		entry := &generated.MessageEntryPayload{
			Status:    entryStatusDrafted,
			Addressed: &generated.MessageAddressed{Directly: to, Copied: cc, Blindcopied: bcc},
		}
		body := generated.CreateReplyRequestContent{
			ActingSenderId: senderID,
			Message:        generated.ReplyMessagePayload{Subject: subject, Content: content},
			Entry:          entry,
		}

		resp, rerr := s.client.genClient().CreateReplyWithResponse(ctx, entryID, body)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		draftEntryID, rerr = draftEntryIDFromLocation(resp.HTTPResponse)
		return rerr
	})
	return draftEntryID, err
}

// NewReply returns a prefilled reply to an entry: the quoted body; in Addressed, the
// recipients a reply goes to as HEY computes them — the entry's sender moved onto the
// To line and the acting user's own addresses, aliases and catch-alls excluded; and in
// Sender, the identity HEY resolved for the reply from the thread's own to and from
// addresses, present only when it differs from the acting user. This is the recipient
// list and acting sender CreateReply should be handed.
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
