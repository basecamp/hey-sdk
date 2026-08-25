package hey

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// MessagesService handles message operations.
type MessagesService struct {
	client *Client
}

// NewMessagesService creates a new MessagesService.
func NewMessagesService(client *Client) *MessagesService {
	return &MessagesService{client: client}
}

// Get returns a specific message by ID.
func (s *MessagesService) Get(ctx context.Context, messageID int64) (result *generated.Message, err error) {
	op := OperationInfo{
		Service: "Messages", Operation: "GetMessage",
		ResourceType: "message", IsMutation: false, ResourceID: messageID,
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
	resp, err := s.client.gen.GetMessageWithResponse(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// Create creates a new message (starts a new thread) and delivers it.
// The acting sender ID is automatically resolved.
//
// Wire format (MessagesController#create): {acting_sender_id, message: {subject, content},
// entry: {addressed: {directly: [...], copied: [...], blindcopied: [...]}}}.
// Recipient lists are JSON arrays; haystack applies Array() to each kind.
func (s *MessagesService) Create(ctx context.Context, subject, content string, to, cc, bcc []string) (err error) {
	if len(to)+len(cc)+len(bcc) == 0 {
		return ErrUsage("a message needs at least one recipient (to, cc or bcc)")
	}
	op := OperationInfo{
		Service: "Messages", Operation: "CreateMessage",
		ResourceType: "message", IsMutation: true,
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

	body := generated.CreateMessageRequestContent{
		ActingSenderId: senderID,
		Message:        generated.MessagePayload{Subject: subject, Content: content},
		Entry:          entryPayload(to, cc, bcc),
	}

	resp, err := s.client.genClient().CreateMessageWithResponse(ctx, body)
	if err != nil {
		return err
	}
	return CheckResponse(resp.HTTPResponse)
}

// entryPayload builds entry.addressed for a message or reply. Callers guarantee at
// least one recipient; it still returns nil for none so the "entry" key is omitted
// rather than sent as an empty hash.
func entryPayload(to, cc, bcc []string) *generated.MessageEntryPayload {
	if len(to)+len(cc)+len(bcc) == 0 {
		return nil
	}
	return &generated.MessageEntryPayload{
		Addressed: &generated.MessageAddressed{Directly: to, Copied: cc, Blindcopied: bcc},
	}
}

// DraftContent is the whole of what a draft carries: the subject, the Trix HTML body,
// the recipients per kind and any scheduled delivery. HEY revises a draft from the
// whole of it — see UpdateDraft — so a caller edits by reading the draft (GetEdit),
// changing fields and sending everything back.
type DraftContent struct {
	Subject string
	Content string
	To      []string
	CC      []string
	BCC     []string

	// Schedule delivers the draft at an hour of a day. Nil means no scheduled
	// delivery — and on an update, clears one already set.
	Schedule *DraftSchedule
}

// DraftSchedule names a delivery time to the hour, read in the identity's time zone.
// HEY schedules to the hour; there are no minutes.
type DraftSchedule struct {
	// Date is YYYY-MM-DD, "today" or "tomorrow".
	Date string
	// Hour is 0 through 23.
	Hour int
}

// entryStatusDrafted is what entry.status carries to keep an entry a draft. Any other
// value — or omitting the status — has the server deliver the entry.
const entryStatusDrafted = "drafted"

// draftEntryPayload builds the entry payload that keeps a message drafted. Unlike
// entryPayload it always carries addressed, empty lists included: a draft with no
// recipients yet is the normal case, and on an update an empty addressed is how
// recipients are removed (HEY replaces the recipient set with what is sent).
func draftEntryPayload(draft DraftContent) *generated.MessageEntryPayload {
	entry := &generated.MessageEntryPayload{
		Status:    entryStatusDrafted,
		Addressed: &generated.MessageAddressed{Directly: draft.To, Copied: draft.CC, Blindcopied: draft.BCC},
	}
	if draft.Schedule != nil {
		entry.ScheduledDelivery = "true"
		entry.ScheduledDeliveryAtDate = draft.Schedule.Date
		entry.ScheduledDeliveryAtHour = strconv.Itoa(draft.Schedule.Hour)
	}
	return entry
}

// draftEntryIDFromLocation reads the draft's entry id out of the Location header a
// draft save answers with (204 No Content, Location: …/messages/{entry_id}) — the
// draft path serves no body, so the header is the only place the id is named.
func draftEntryIDFromLocation(resp *http.Response) (int64, error) {
	location := resp.Header.Get("Location")
	if location == "" {
		return 0, fmt.Errorf("draft saved but the response named no Location; cannot report the draft's id")
	}
	parsed, err := url.Parse(location)
	if err != nil {
		return 0, fmt.Errorf("draft saved but its Location %q is unreadable: %w", location, err)
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	id, err := strconv.ParseInt(segments[len(segments)-1], 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("draft saved but its Location %q names no entry id", location)
	}
	return id, nil
}

// CreateDraft saves a new message as a draft instead of delivering it, and answers the
// draft's entry id — the id GetEdit, UpdateDraft, SendDraft and DeleteDraft take. A
// draft needs no recipients; whatever it carries is kept for the send.
func (s *MessagesService) CreateDraft(ctx context.Context, draft DraftContent) (entryID int64, err error) {
	op := OperationInfo{
		Service: "Messages", Operation: "CreateMessage",
		ResourceType: "draft", IsMutation: true,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		senderID, serr := s.client.DefaultSenderID(ctx)
		if serr != nil {
			return serr
		}

		body := generated.CreateMessageRequestContent{
			ActingSenderId: senderID,
			Message:        generated.MessagePayload{Subject: draft.Subject, Content: draft.Content},
			Entry:          draftEntryPayload(draft),
		}

		resp, rerr := s.client.genClient().CreateMessageWithResponse(ctx, body)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		entryID, rerr = draftEntryIDFromLocation(resp.HTTPResponse)
		return rerr
	})
	return entryID, err
}

// UpdateDraft revises a draft in place from the whole of draft: the subject, the body
// and the recipients are replaced with what is sent (empty recipients remove them),
// and the scheduled delivery is rewritten too — a nil Schedule clears one already set.
// A trashed draft is silently restored by the revision.
func (s *MessagesService) UpdateDraft(ctx context.Context, entryID int64, draft DraftContent) error {
	op := OperationInfo{
		Service: "Messages", Operation: "UpdateMessage",
		ResourceType: "draft", IsMutation: true, ResourceID: entryID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		senderID, serr := s.client.DefaultSenderID(ctx)
		if serr != nil {
			return serr
		}

		body := generated.CreateMessageRequestContent{
			ActingSenderId: senderID,
			Message:        generated.MessagePayload{Subject: draft.Subject, Content: draft.Content},
			Entry:          draftEntryPayload(draft),
		}

		resp, rerr := s.client.genClient().UpdateMessageWithResponse(ctx, entryID, body)
		if rerr != nil {
			return rerr
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// SendDraft delivers a draft through HEY's undo-delay window. The revision and the
// delivery are one request, so the draft's final state rides along: subject, body and
// recipients are replaced with what is sent, exactly as UpdateDraft replaces them.
// Delivery needs somebody to deliver to, so at least one recipient is required.
//
// The request is never retried, despite the PUT: it triggers a delivery, and a retry
// after an ambiguous first attempt could send the message twice. An ambiguous failure
// is the caller's to resolve — read the draft (or the thread) before trying again.
func (s *MessagesService) SendDraft(ctx context.Context, entryID int64, draft DraftContent) error {
	if len(draft.To)+len(draft.CC)+len(draft.BCC) == 0 {
		return ErrUsage("sending a draft needs at least one recipient (to, cc or bcc)")
	}
	op := OperationInfo{
		Service: "Messages", Operation: "UpdateMessage",
		ResourceType: "message", IsMutation: true, ResourceID: entryID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		senderID, serr := s.client.DefaultSenderID(ctx)
		if serr != nil {
			return serr
		}

		body := generated.CreateMessageRequestContent{
			ActingSenderId: senderID,
			Message:        generated.MessagePayload{Subject: draft.Subject, Content: draft.Content},
			Entry:          entryPayload(draft.To, draft.CC, draft.BCC),
		}

		resp, rerr := s.client.genClient().UpdateMessageWithResponse(ctx, entryID, body)
		if rerr != nil {
			return rerr
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// GetEdit answers a draft's editable state — the subject, the Trix HTML body, the
// recipients per kind and any scheduled delivery, as the composer would load them.
func (s *MessagesService) GetEdit(ctx context.Context, entryID int64) (result *generated.MessageEditState, err error) {
	op := OperationInfo{
		Service: "Messages", Operation: "GetMessageEdit",
		ResourceType: "draft", IsMutation: false, ResourceID: entryID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetMessageEditWithResponse(ctx, entryID)
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
