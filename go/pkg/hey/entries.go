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

// EntriesService handles draft and reply operations.
type EntriesService struct {
	client *Client
}

// CreateReplyDraftParams contains the browser-composer fields used to save an
// editable reply draft. Content must already be rich ActionText HTML; the SDK
// transmits it without modification.
type CreateReplyDraftParams struct {
	ActingSenderID    int64
	Content           string
	Subject           string
	AutoQuoting       *bool
	To                []string
	CC                []string
	BCC               []string
	AuthenticityToken string
}

// ReplyDraft identifies a newly saved reply draft.
type ReplyDraft struct {
	ID       int64  `json:"id"`
	Location string `json:"location"`
	EditURL  string `json:"edit_url"`
}

// UpdateDraftParams contains the complete composer state used to update an
// editable draft. Callers should load these values from the draft edit page so
// fields that are not being changed remain intact.
type UpdateDraftParams struct {
	ActingSenderID    int64
	Content           string
	Subject           string
	To                []string
	CC                []string
	BCC               []string
	AuthenticityToken string
}

// DraftUpdate describes a saved draft after an update. HEY may return a
// redirect, so Location and StatusCode are preserved without following it.
type DraftUpdate struct {
	ID         int64  `json:"id"`
	Location   string `json:"location,omitempty"`
	EditURL    string `json:"edit_url"`
	StatusCode int    `json:"status_code"`
}

// DeleteDraftParams contains the browser form security fields used to discard
// an editable draft.
type DeleteDraftParams struct {
	AuthenticityToken string
}

// DraftDeletion describes the response returned after a draft is discarded.
type DraftDeletion struct {
	Location   string `json:"location"`
	StatusCode int    `json:"status_code"`
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

// CreateReplyDraft saves an editable reply draft without sending it.
// ActingSenderID should be copied from the reply composer when available. When
// it is zero, the acting sender ID is resolved from the current identity.
func (s *EntriesService) CreateReplyDraft(ctx context.Context, entryID int64, params CreateReplyDraftParams) (result *ReplyDraft, err error) {
	op := OperationInfo{
		Service: "Entries", Operation: "CreateReplyDraft",
		ResourceType: "draft", IsMutation: true, ResourceID: entryID,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return nil, err
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	senderID := params.ActingSenderID
	if senderID == 0 {
		senderID, err = s.client.DefaultSenderID(ctx)
		if err != nil {
			return nil, err
		}
	}

	body := generated.CreateReplyDraftFormdataRequestBody{
		ActingSenderId:            senderID,
		AuthenticityToken:         params.AuthenticityToken,
		EntryAddressedBlindcopied: append([]string(nil), params.BCC...),
		EntryAddressedCopied:      append([]string(nil), params.CC...),
		EntryAddressedDirectly:    append([]string(nil), params.To...),
		EntryStatus:               "drafted",
		MessageAutoQuoting:        params.AutoQuoting,
		MessageContent:            params.Content,
		MessageSubject:            params.Subject,
	}

	s.client.initGeneratedClient()
	resp, err := s.client.gen.CreateReplyDraftWithFormdataBody(ctx, entryID, body,
		useFormRepresentation,
		func(_ context.Context, req *http.Request) error {
			if params.AuthenticityToken != "" {
				req.Header.Set("X-CSRF-Token", params.AuthenticityToken)
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if !formMutationSuccess(resp.StatusCode) {
		return nil, CheckResponse(resp)
	}
	return replyDraftFromLocation(resp.Header.Get("Location"))
}

// UpdateDraft saves the complete current state of an editable draft without
// sending it. The operation uses HEY's canonical PATCH route and preserves
// redirect responses.
func (s *EntriesService) UpdateDraft(ctx context.Context, messageID int64, params UpdateDraftParams) (result *DraftUpdate, err error) {
	if messageID <= 0 {
		return nil, fmt.Errorf("draft message ID must be positive")
	}
	if params.ActingSenderID <= 0 {
		return nil, fmt.Errorf("draft update requires a positive acting sender ID")
	}
	if strings.TrimSpace(params.AuthenticityToken) == "" {
		return nil, fmt.Errorf("draft update requires an authenticity token")
	}

	op := OperationInfo{
		Service: "Entries", Operation: "UpdateDraft",
		ResourceType: "draft", IsMutation: true, ResourceID: messageID,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return nil, err
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	generatedParams := &generated.UpdateDraftParams{XCSRFToken: params.AuthenticityToken}
	body := generated.UpdateDraftFormdataRequestBody{
		ActingSenderId:            params.ActingSenderID,
		AuthenticityToken:         params.AuthenticityToken,
		EntryAddressedBlindcopied: append([]string(nil), params.BCC...),
		EntryAddressedCopied:      append([]string(nil), params.CC...),
		EntryAddressedDirectly:    append([]string(nil), params.To...),
		EntryStatus:               "drafted",
		MessageContent:            params.Content,
		MessageSubject:            params.Subject,
	}

	s.client.initGeneratedClient()
	resp, err := s.client.gen.UpdateDraftWithFormdataBody(ctx, messageID, generatedParams, body,
		useFormRepresentation,
		func(_ context.Context, req *http.Request) error {
			req.Header.Set("X-Requested-With", "XMLHttpRequest")
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if !formMutationSuccess(resp.StatusCode) {
		return nil, CheckResponse(resp)
	}
	return &DraftUpdate{
		ID:         messageID,
		Location:   resp.Header.Get("Location"),
		EditURL:    fmt.Sprintf("/messages/%d/edit", messageID),
		StatusCode: resp.StatusCode,
	}, nil
}

// DeleteDraft discards an editable draft without sending it. The operation
// reproduces HEY's browser form contract: POST to /messages/{id} with a Rails
// DELETE method override, drafted status, and CSRF header.
func (s *EntriesService) DeleteDraft(ctx context.Context, messageID int64, params DeleteDraftParams) (result *DraftDeletion, err error) {
	if messageID <= 0 {
		return nil, fmt.Errorf("draft message ID must be positive")
	}
	if strings.TrimSpace(params.AuthenticityToken) == "" {
		return nil, fmt.Errorf("draft deletion requires an authenticity token")
	}

	op := OperationInfo{
		Service: "Entries", Operation: "DeleteDraft",
		ResourceType: "draft", IsMutation: true, ResourceID: messageID,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return nil, err
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	generatedParams := &generated.DeleteDraftParams{XCSRFToken: params.AuthenticityToken}
	body := generated.DeleteDraftFormdataRequestBody{
		UnderscoreMethod: "delete",
		Status:           "drafted",
	}

	s.client.initGeneratedClient()
	resp, err := s.client.gen.DeleteDraftWithFormdataBody(ctx, messageID, generatedParams, body, useFormRepresentation)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if !formMutationSuccess(resp.StatusCode) {
		return nil, CheckResponse(resp)
	}
	return &DraftDeletion{
		Location:   resp.Header.Get("Location"),
		StatusCode: resp.StatusCode,
	}, nil
}

func formMutationSuccess(status int) bool {
	return status >= 200 && status < 300 || status == http.StatusFound || status == http.StatusSeeOther
}

func replyDraftFromLocation(location string) (*ReplyDraft, error) {
	if strings.TrimSpace(location) == "" {
		return nil, fmt.Errorf("reply draft response is missing Location header")
	}

	parsed, err := url.Parse(location)
	if err != nil {
		return nil, fmt.Errorf("invalid reply draft Location %q: %w", location, err)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path == "" {
		return nil, fmt.Errorf("reply draft Location %q has no draft ID", location)
	}
	segments := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
	var idSegment string
	var hasEditSuffix bool
	switch {
	case len(segments) == 3 && segments[0] == "entries" && segments[1] == "drafts":
		idSegment = segments[2]
	case len(segments) == 4 && segments[0] == "entries" && segments[1] == "drafts" && segments[3] == "edit":
		idSegment = segments[2]
		hasEditSuffix = true
	case len(segments) == 2 && segments[0] == "messages":
		idSegment = segments[1]
	case len(segments) == 3 && segments[0] == "messages" && segments[2] == "edit":
		idSegment = segments[1]
		hasEditSuffix = true
	default:
		return nil, fmt.Errorf("reply draft Location %q does not match /entries/drafts/{id}[/edit] or /messages/{id}[/edit]", location)
	}
	id, err := strconv.ParseInt(idSegment, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("reply draft Location %q has no numeric draft ID", location)
	}
	if id <= 0 {
		return nil, fmt.Errorf("reply draft Location %q has an invalid draft ID", location)
	}

	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	parsed.RawPath = ""
	if !hasEditSuffix {
		parsed.Path += "/edit"
	}
	return &ReplyDraft{
		ID:       id,
		Location: location,
		EditURL:  parsed.String(),
	}, nil
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
