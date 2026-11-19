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
// hands it back as NewReply's Sender, or names none when the reply goes out as the
// acting user's own contact, the prefill's Creator. Pass ReplySentAs(prefill).Id, which
// reads both; zero falls back to the account's default sender, which on such a thread
// is the wrong identity, and need not be the person's own address either.
//
// Subject is the reply's own subject line — HEY does not derive one, so pass the
// prefilled "Re: …" that NewReply hands back. Empty leaves it off the wire.
//
// Recipients are required. HEY does not reply-all on the caller's behalf: a reply
// posted without entry.addressed is saved as a draft (the server answers with a
// redirect to the thread with the draft expanded) rather than delivered. Callers
// resolve the thread's recipients first — hey-cli reads them from the topic page.
//
// It answers what HEY said about the delivery, as MessagesService.Send does. TopicId is
// the thread the reply landed on: the one replied to, or — for a reply that breaks out
// into a thread of its own on a Domains account — the new one.
func (s *EntriesService) CreateReply(ctx context.Context, entryID, actingSenderID int64, subject, content string, to, cc, bcc []string) (sent *generated.SentMessage, err error) {
	if len(to)+len(cc)+len(bcc) == 0 {
		return nil, ErrUsage("a reply needs at least one recipient (to, cc or bcc); HEY saves an unaddressed reply as a draft")
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

	senderID, err := s.client.resolveActingSenderID(ctx, actingSenderID)
	if err != nil {
		return nil, err
	}

	body := generated.CreateReplyRequestContent{
		ActingSenderId: senderID,
		Message:        generated.ReplyMessagePayload{Subject: subject, Content: content},
		Entry:          entryPayload(to, cc, bcc),
	}

	resp, err := s.client.genClient().CreateReply(ctx, entryID, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := CheckResponse(resp); err != nil {
		return nil, err
	}
	return readSentMessage(resp)
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

// Restore brings an entry back from the trash: a trashed draft is a draft again, and a
// trashed message returns to its thread. An entry that is not in the trash is left as it
// is. HEY first delivers an entry Undo Send still holds, so this calls no send back;
// Topics().UndoSend does.
//
// Only the entry is restored. A draft that was its topic's only entry took the topic to
// the trash with it (DeleteDraft); restore that one with Topics().Restore, which brings
// back the topic and the draft together, since the entry alone would be a draft on a
// trashed topic, emptied from the trash with the rest. This is for a draft trashed out of
// a thread that stayed, which Topics().Restore leaves in the trash.
//
// HEY answers with a redirect to the entry's topic rather than JSON. The SDK takes the
// redirect as the answer rather than follow it, so a read of the thread that fails cannot
// report a restore that went through as one that did not.
func (s *EntriesService) Restore(ctx context.Context, entryID int64) error {
	op := OperationInfo{
		Service: "Entries", Operation: "RestoreEntry",
		ResourceType: "entry", IsMutation: true, ResourceID: entryID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().RestoreEntryWithResponse(contextWithRedirectAsAnswer(ctx), entryID)
		if err != nil {
			return err
		}
		if _, redirected := redirectAnswer(resp.HTTPResponse); redirected {
			return nil
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// DraftPage is one page of drafts along with the cursor for the page after it. The
// index pages by geared_pagination's opaque cursor out of the Link header — a page
// number is answered with the first page forever — so NextPage is the only way to walk
// it; empty means the last page.
type DraftPage struct {
	Drafts   []generated.DraftMessage `json:"drafts"`
	NextPage string                   `json:"next_page"`
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
// ActingSenderID works as it does on CreateReply: pass ReplySentAs of NewReply's
// prefill so the draft is saved under the identity HEY resolved for the thread; zero
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
		senderID, serr := s.client.resolveActingSenderID(ctx, actingSenderID)
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
// addresses, present only when it differs from the acting user, whose contact is
// Creator. This is the recipient list CreateReply should be handed, and ReplySentAs
// reads its acting sender.
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
