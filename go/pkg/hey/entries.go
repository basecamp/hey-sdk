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

// DeleteDraftParams contains the browser form security fields used to discard
// an editable draft.
type DeleteDraftParams struct {
	AuthenticityToken string
}

// DraftDeletion describes the response returned after a draft is discarded.
// HEY may return a redirect to the surrounding topic, so Location is preserved
// without following it.
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

	body := generated.CreateReplyJSONRequestBody{
		ActingSenderId: senderID,
		Message: generated.ReplyMessagePayload{
			Content: content,
		},
	}
	addressed := generated.MessageAddressed{}
	if len(to) > 0 {
		addressed.Directly = strings.Join(to, ",")
	}
	if len(cc) > 0 {
		addressed.Copied = strings.Join(cc, ",")
	}
	if len(bcc) > 0 {
		addressed.Blindcopied = strings.Join(bcc, ",")
	}
	if len(to) > 0 || len(cc) > 0 || len(bcc) > 0 {
		body.Entry = &generated.MessageEntryPayload{
			Addressed: addressed,
		}
	}

	s.client.initGeneratedClient()
	resp, err := s.client.gen.CreateReplyWithResponse(ctx, entryID, body)
	if err != nil {
		return err
	}
	return CheckResponse(resp.HTTPResponse)
}

// CreateReplyDraft saves an editable reply draft without sending it.
// ActingSenderID should be copied from the reply composer when available. When
// it is zero, the acting sender ID is resolved from the current identity.
// Recipient slices are encoded as repeated Rails form fields and rich HTML
// content is preserved exactly.
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
		func(_ context.Context, req *http.Request) error {
			req.Header.Set("Accept", "*/*")
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

	if !replyDraftSuccess(resp.StatusCode) {
		return nil, CheckResponse(resp)
	}
	return replyDraftFromLocation(resp.Header.Get("Location"))
}

// DeleteDraft discards an editable draft without sending it. The generated
// operation reproduces HEY's browser form contract: POST to /messages/{id}
// with a Rails DELETE method override, drafted status, and CSRF header.
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

	generatedParams := &generated.DeleteDraftParams{
		XCSRFToken: params.AuthenticityToken,
	}
	body := generated.DeleteDraftFormdataRequestBody{
		UnderscoreMethod: "delete",
		Status:           "drafted",
	}

	s.client.initGeneratedClient()
	resp, err := s.client.gen.DeleteDraftWithFormdataBody(ctx, messageID, generatedParams, body,
		func(_ context.Context, req *http.Request) error {
			req.Header.Set("Accept", "*/*")
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
	return &DraftDeletion{
		Location:   resp.Header.Get("Location"),
		StatusCode: resp.StatusCode,
	}, nil
}

func replyDraftSuccess(status int) bool {
	return formMutationSuccess(status)
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
	segments := strings.Split(parsed.Path, "/")
	id, err := strconv.ParseInt(segments[len(segments)-1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("reply draft Location %q has no numeric draft ID", location)
	}
	if id <= 0 {
		return nil, fmt.Errorf("reply draft Location %q has an invalid draft ID", location)
	}

	parsed.RawQuery = ""
	parsed.Fragment = ""
	return &ReplyDraft{
		ID:       id,
		Location: location,
		EditURL:  parsed.String() + "/edit",
	}, nil
}
