package hey

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// Box kinds as reported by ListBoxes. Use with MoveToBox.
const (
	BoxKindImbox    = "imbox"
	BoxKindFeed     = "feedbox"
	BoxKindSetAside = "asidebox"
	BoxKindLater    = "laterbox"
	BoxKindTrail    = "trailbox"
	BoxKindBubbleUp = "bubblebox"
)

// PostingsService handles posting-level actions (seen, move, trash, mute).
//
// HEY exposes these as bulk endpoints that take a list of posting IDs, so every
// method here accepts one or more IDs.
type PostingsService struct {
	client *Client
}

// NewPostingsService creates a new PostingsService.
func NewPostingsService(client *Client) *PostingsService {
	return &PostingsService{client: client}
}

// MarkSeen marks one or more postings as seen/read.
func (s *PostingsService) MarkSeen(ctx context.Context, postingIDs []int64) (err error) {
	return s.bulkAction(ctx, "MarkPostingsSeen", postingIDs, func(ctx context.Context, ids []int64) error {
		resp, err := s.client.genClient().MarkPostingsSeenWithResponse(ctx, generated.MarkPostingsRequestContent{PostingIds: ids})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// MarkUnseen marks one or more postings as unseen/unread.
func (s *PostingsService) MarkUnseen(ctx context.Context, postingIDs []int64) (err error) {
	return s.bulkAction(ctx, "MarkPostingsUnseen", postingIDs, func(ctx context.Context, ids []int64) error {
		resp, err := s.client.genClient().MarkPostingsUnseenWithResponse(ctx, generated.MarkPostingsRequestContent{PostingIds: ids})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Move moves one or more postings to the box with the given ID
// (POST /postings/moves). Box IDs come from Boxes().List.
func (s *PostingsService) Move(ctx context.Context, boxID int64, postingIDs ...int64) (err error) {
	return s.bulkAction(ctx, "MovePostings", postingIDs, func(ctx context.Context, ids []int64) error {
		resp, err := s.client.genClient().MovePostingsWithResponse(ctx, generated.MovePostingsRequestContent{PostingIds: ids, BoxId: boxID})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// MoveToBox moves one or more postings to the box of the given kind
// (BoxKindImbox, BoxKindFeed, ...). The kind→ID mapping is resolved once via
// ListBoxes and cached on the client.
func (s *PostingsService) MoveToBox(ctx context.Context, kind string, postingIDs ...int64) error {
	boxID, err := s.client.BoxIDByKind(ctx, kind)
	if err != nil {
		return err
	}
	return s.Move(ctx, boxID, postingIDs...)
}

// MoveToImbox moves postings to the Imbox.
func (s *PostingsService) MoveToImbox(ctx context.Context, postingIDs ...int64) error {
	return s.MoveToBox(ctx, BoxKindImbox, postingIDs...)
}

// MoveToFeed moves postings to The Feed.
func (s *PostingsService) MoveToFeed(ctx context.Context, postingIDs ...int64) error {
	return s.MoveToBox(ctx, BoxKindFeed, postingIDs...)
}

// MoveToSetAside moves postings to Set Aside.
func (s *PostingsService) MoveToSetAside(ctx context.Context, postingIDs ...int64) error {
	return s.MoveToBox(ctx, BoxKindSetAside, postingIDs...)
}

// MoveToReplyLater moves postings to Reply Later.
func (s *PostingsService) MoveToReplyLater(ctx context.Context, postingIDs ...int64) error {
	return s.MoveToBox(ctx, BoxKindLater, postingIDs...)
}

// MoveToPaperTrail moves postings to the Paper Trail.
func (s *PostingsService) MoveToPaperTrail(ctx context.Context, postingIDs ...int64) error {
	return s.MoveToBox(ctx, BoxKindTrail, postingIDs...)
}

// MoveToTrash moves one or more postings to the trash (POST /postings/trash).
// For shared topics HEY removes your access rather than trashing for everyone.
func (s *PostingsService) MoveToTrash(ctx context.Context, postingIDs ...int64) (err error) {
	return s.trash(ctx, "", postingIDs)
}

// TrashForEveryone moves one or more postings to the trash, and on shared topics trashes the
// thread for everyone on it instead of only dropping your own access.
func (s *PostingsService) TrashForEveryone(ctx context.Context, postingIDs ...int64) (err error) {
	return s.trash(ctx, "false", postingIDs)
}

// trash posts the bulk trash request. An empty removeAccess leaves the field out, which the
// server reads as "remove my access".
func (s *PostingsService) trash(ctx context.Context, removeAccess string, postingIDs []int64) error {
	return s.bulkAction(ctx, "TrashPostings", postingIDs, func(ctx context.Context, ids []int64) error {
		resp, err := s.client.genClient().TrashPostingsWithResponse(ctx, generated.TrashPostingsRequestContent{
			PostingIds:   ids,
			RemoveAccess: removeAccess,
		})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Mute mutes one or more postings so their threads stop notifying
// (POST /postings/mutings).
func (s *PostingsService) Mute(ctx context.Context, postingIDs ...int64) (err error) {
	return s.bulkAction(ctx, "MutePostings", postingIDs, func(ctx context.Context, ids []int64) error {
		resp, err := s.client.genClient().MutePostingsWithResponse(ctx, generated.MarkPostingsRequestContent{PostingIds: ids})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Unmute unmutes one or more postings (DELETE /postings/mutings).
func (s *PostingsService) Unmute(ctx context.Context, postingIDs ...int64) (err error) {
	return s.bulkAction(ctx, "UnmutePostings", postingIDs, func(ctx context.Context, ids []int64) error {
		params := &generated.UnmutePostingsParams{PostingIds: joinIDs(ids)}
		resp, err := s.client.genClient().UnmutePostingsWithResponse(ctx, params)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// MarkSpam marks one or more postings as spam (POST /postings/spam).
//
// Past ten postings the server hands the work to a background job, so the call returns
// before the postings have actually moved.
func (s *PostingsService) MarkSpam(ctx context.Context, postingIDs ...int64) (err error) {
	return s.bulkAction(ctx, "MarkPostingsSpam", postingIDs, func(ctx context.Context, ids []int64) error {
		resp, err := s.client.genClient().MarkPostingsSpamWithResponse(ctx, generated.MarkPostingsRequestContent{PostingIds: ids})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// AddToBoxGroup files one or more postings into an existing Set Aside group.
func (s *PostingsService) AddToBoxGroup(ctx context.Context, boxID, boxGroupID int64, postingIDs ...int64) (err error) {
	return s.bulkAction(ctx, "AddPostingsToBoxGroup", postingIDs, func(ctx context.Context, ids []int64) error {
		resp, err := s.client.genClient().AddPostingsToBoxGroupWithResponse(ctx, generated.AddPostingsToBoxGroupRequestContent{
			PostingIds: ids,
			BoxId:      boxID,
			BoxGroupId: boxGroupID,
		})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// RemoveFromBoxGroup takes one or more postings out of whatever Set Aside group they are in.
func (s *PostingsService) RemoveFromBoxGroup(ctx context.Context, postingIDs ...int64) (err error) {
	return s.bulkAction(ctx, "RemovePostingsFromBoxGroup", postingIDs, func(ctx context.Context, ids []int64) error {
		params := &generated.RemovePostingsFromBoxGroupParams{PostingIds: joinIDs(ids)}
		resp, err := s.client.genClient().RemovePostingsFromBoxGroupWithResponse(ctx, params)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// File labels one or more postings with an existing folder.
func (s *PostingsService) File(ctx context.Context, folderID int64, postingIDs ...int64) (err error) {
	return s.bulkAction(ctx, "FilePostings", postingIDs, func(ctx context.Context, ids []int64) error {
		resp, err := s.client.genClient().FilePostingsWithResponse(ctx, generated.FilePostingsRequestContent{
			PostingIds: ids,
			FolderId:   folderID,
		})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Unfile removes a label from one or more postings. A folderID of 0 removes every label.
func (s *PostingsService) Unfile(ctx context.Context, folderID int64, postingIDs ...int64) (err error) {
	return s.bulkAction(ctx, "UnfilePostings", postingIDs, func(ctx context.Context, ids []int64) error {
		// folder_id is only sent when set: 0 is not "all folders" to the server,
		// it is a lookup for a folder that does not exist.
		params := generated.UnfilePostingsParams{PostingIds: joinIDs(ids)}
		if folderID != 0 {
			params.FolderId = &folderID
		}
		resp, err := s.client.genClient().UnfilePostingsWithResponse(ctx, &params)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// CreateFolder creates a folder (label) and files one or more postings into it.
func (s *PostingsService) CreateFolder(ctx context.Context, name string, postingIDs ...int64) (err error) {
	return s.bulkAction(ctx, "CreateFolderForPostings", postingIDs, func(ctx context.Context, ids []int64) error {
		resp, err := s.client.genClient().CreateFolderForPostingsWithResponse(ctx, generated.CreateFolderForPostingsRequestContent{
			PostingIds: ids,
			Folder:     generated.FolderPayload{Name: name},
		})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// CancelBubbleUp drops the scheduled bubble up on one or more postings.
func (s *PostingsService) CancelBubbleUp(ctx context.Context, postingIDs ...int64) (err error) {
	return s.bulkAction(ctx, "CancelPostingsBubbleUp", postingIDs, func(ctx context.Context, ids []int64) error {
		params := &generated.CancelPostingsBubbleUpParams{PostingIds: joinIDs(ids)}
		resp, err := s.client.genClient().CancelPostingsBubbleUpWithResponse(ctx, params)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// BubbleUpNow bubbles one or more postings up right away.
func (s *PostingsService) BubbleUpNow(ctx context.Context, postingIDs ...int64) (err error) {
	return s.bulkAction(ctx, "BubbleUpPostingsNow", postingIDs, func(ctx context.Context, ids []int64) error {
		resp, err := s.client.genClient().BubbleUpPostingsNowWithResponse(ctx, generated.MarkPostingsRequestContent{PostingIds: ids})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// BubbleUpSlot is one of HEY's named bubble-up schedule slots — the web app's
// "Later today", "Tomorrow", "This weekend" and "Next week". Later today lands at
// HEY's evening hour of the current day, the others at its morning hour of their day
// (Saturday for the weekend, Monday for next week) — in UTC, like every hour HEY
// reads out of a JSON request.
type BubbleUpSlot string

const (
	BubbleUpLaterToday  BubbleUpSlot = "today"
	BubbleUpTomorrow    BubbleUpSlot = "tomorrow"
	BubbleUpThisWeekend BubbleUpSlot = "weekend"
	BubbleUpNextWeek    BubbleUpSlot = "next_week"
)

// ScheduleBubbleUp schedules one or more postings to bubble up on a date, written
// YYYY-MM-DD. HEY resurfaces them at its morning hour of that day — in UTC, like every
// hour HEY reads out of a JSON request. HEY does not refuse a past timestamp — the
// postings bubble up on the next scheduler run instead.
func (s *PostingsService) ScheduleBubbleUp(ctx context.Context, date string, postingIDs ...int64) (err error) {
	return s.scheduleBubbleUp(ctx, "custom", date, postingIDs)
}

// ScheduleBubbleUpFor schedules one or more postings to bubble up at one of HEY's
// named slots.
func (s *PostingsService) ScheduleBubbleUpFor(ctx context.Context, slot BubbleUpSlot, postingIDs ...int64) (err error) {
	return s.scheduleBubbleUp(ctx, string(slot), "", postingIDs)
}

func (s *PostingsService) scheduleBubbleUp(ctx context.Context, slot, date string, postingIDs []int64) error {
	return s.bulkAction(ctx, "SchedulePostingsBubbleUp", postingIDs, func(ctx context.Context, ids []int64) error {
		body := generated.SchedulePostingsBubbleUpRequestContent{PostingIds: ids, Slot: slot, Date: date}
		resp, err := s.client.genClient().SchedulePostingsBubbleUpWithResponse(ctx, body)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// PostingChangesCursor is where a read of a box's changes feed starts. Since is an ISO
// 8601 timestamp with milliseconds and is exclusive; Version is the contract version the
// caller speaks. A box's PostingChangesUrl carries the pair to begin with — read it with
// PostingChangesCursorFrom rather than picking the query apart.
type PostingChangesCursor struct {
	Since   string
	Version string
	Page    string
	PerPage string
}

// PostingChangesCursorFrom reads a cursor out of a changes URL the server issued, either
// a box's PostingChangesUrl or a Link header the feed answered with.
func PostingChangesCursorFrom(changesURL string) (PostingChangesCursor, error) {
	parsed, err := url.Parse(changesURL)
	if err != nil {
		return PostingChangesCursor{}, fmt.Errorf("failed to read changes URL %q: %w", changesURL, err)
	}

	query := parsed.Query()
	return PostingChangesCursor{
		Since:   query.Get("since"),
		Version: query.Get("v"),
		Page:    query.Get("page"),
		PerPage: query.Get("per_page"),
	}, nil
}

// PostingChanges is everything that happened to a box's postings since a cursor.
//
// NextPage is set while this increment has more pages to read now. NextCursor is set on
// the last page and is where the next read should resume; it is nil when nothing changed,
// in which case the cursor that produced this page still stands. FullSyncRequired is set
// when the cursor is too far behind for an increment to carry the difference, and the box
// has to be read in full instead.
type PostingChanges struct {
	Added            []generated.Posting
	Updated          []generated.Posting
	Deleted          []generated.DeletedPosting
	NextPage         *PostingChangesCursor
	NextCursor       *PostingChangesCursor
	FullSyncRequired bool
}

// AllChanges reads a box's posting changes feed from a cursor to its end.
func (s *PostingsService) AllChanges(ctx context.Context, boxID int64, cursor PostingChangesCursor) (*PostingChanges, error) {
	all := &PostingChanges{}

	for page := 1; page <= s.client.httpOpts.MaxPages; page++ {
		changes, err := s.Changes(ctx, boxID, cursor)
		if err != nil {
			return nil, err
		}
		if changes.FullSyncRequired {
			return changes, nil
		}

		all.Added = append(all.Added, changes.Added...)
		all.Updated = append(all.Updated, changes.Updated...)
		all.Deleted = append(all.Deleted, changes.Deleted...)
		all.NextCursor = changes.NextCursor

		if changes.NextPage == nil {
			return all, nil
		}
		cursor = *changes.NextPage
	}

	s.client.logger.Warn("posting changes pagination capped", "maxPages", s.client.httpOpts.MaxPages)
	return all, nil
}

// Changes returns one page of a box's posting changes feed.
func (s *PostingsService) Changes(ctx context.Context, boxID int64, cursor PostingChangesCursor) (result *PostingChanges, err error) {
	if cursor.Since == "" {
		return nil, ErrUsage("a since cursor is required — start from the box's posting_changes_url")
	}

	op := OperationInfo{
		Service: "Postings", Operation: "GetBoxPostingChanges",
		ResourceType: "posting", IsMutation: false, ResourceID: boxID,
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
	resp, err := s.client.gen.GetBoxPostingChangesWithResponse(ctx, boxID, cursor.params())
	if err != nil {
		return nil, err
	}

	// Too far behind for an increment: the server says so with a 409 and no body.
	if resp.StatusCode() == http.StatusConflict {
		return &PostingChanges{FullSyncRequired: true}, nil
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}

	changes := &PostingChanges{}
	if resp.JSON200 != nil {
		changes.Added = resp.JSON200.Added
		changes.Updated = resp.JSON200.Updated
		changes.Deleted = resp.JSON200.Deleted
	}

	next, err := nextPostingChangesCursor(resp.HTTPResponse)
	if err != nil {
		return nil, err
	}
	if next != nil {
		// The feed answers with a page link while an increment has more pages, and with a
		// fresh since cursor on the last one.
		if next.Page != "" {
			changes.NextPage = next
		} else {
			changes.NextCursor = next
		}
	}

	return changes, nil
}

func nextPostingChangesCursor(resp *http.Response) (*PostingChangesCursor, error) {
	link := parseNextLink(resp.Header.Get("Link"))
	if link == "" {
		return nil, nil
	}

	requested := resp.Request.URL.String()
	next := resolveURL(requested, link)
	if !isSameOrigin(next, requested) {
		return nil, fmt.Errorf("changes Link header points to a different origin: %s", next)
	}

	cursor, err := PostingChangesCursorFrom(next)
	if err != nil {
		return nil, err
	}
	return &cursor, nil
}

func (c PostingChangesCursor) params() *generated.GetBoxPostingChangesParams {
	params := &generated.GetBoxPostingChangesParams{Since: c.Since}
	if c.Version != "" {
		params.V = &c.Version
	}
	if c.Page != "" {
		params.Page = &c.Page
	}
	if c.PerPage != "" {
		params.PerPage = &c.PerPage
	}
	return params
}

// BoxIDByKind returns the ID of the caller's box with the given kind
// ("imbox", "feedbox", "asidebox", "laterbox", "trailbox", "bubblebox"),
// resolving via ListBoxes on first use and caching the result.
func (c *Client) BoxIDByKind(ctx context.Context, kind string) (int64, error) {
	c.boxMu.Lock()
	defer c.boxMu.Unlock()

	if id, ok := c.boxByKind[kind]; ok {
		return id, nil
	}

	boxes, err := c.Boxes().List(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list boxes to resolve %q: %w", kind, err)
	}
	if boxes == nil {
		return 0, ErrAPI(0, "could not list boxes")
	}
	byKind := make(map[string]int64, len(*boxes))
	for _, b := range *boxes {
		if b.Kind != "" {
			byKind[b.Kind] = b.Id
		}
	}
	c.boxByKind = byKind

	id, ok := byKind[kind]
	if !ok {
		return 0, ErrAPI(0, fmt.Sprintf("no box of kind %q", kind))
	}
	return id, nil
}

func (s *PostingsService) bulkAction(ctx context.Context, operation string, ids []int64, fn func(context.Context, []int64) error) (err error) {
	if len(ids) == 0 {
		return ErrUsage("at least one posting ID is required")
	}
	op := OperationInfo{
		Service: "Postings", Operation: operation,
		ResourceType: "posting", IsMutation: true,
	}
	if len(ids) == 1 {
		op.ResourceID = ids[0]
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	return fn(ctx, ids)
}
