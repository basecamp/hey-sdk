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

// TopicsService handles topic operations.
type TopicsService struct {
	client *Client
}

// NewTopicsService creates a new TopicsService.
func NewTopicsService(client *Client) *TopicsService {
	return &TopicsService{client: client}
}

// Get returns a specific topic by ID.
func (s *TopicsService) Get(ctx context.Context, topicID int64) (result *generated.Topic, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "GetTopic",
		ResourceType: "topic", IsMutation: false, ResourceID: topicID,
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
	resp, err := s.client.gen.GetTopicWithResponse(ctx, topicID)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// Rename changes a topic's name. HEY normalizes blank names to "No subject" and
// truncates names to 1024 characters. Get the topic afterwards to read its saved name.
func (s *TopicsService) Rename(ctx context.Context, topicID int64, name string) error {
	op := OperationInfo{
		Service: "Topics", Operation: "RenameTopic",
		ResourceType: "topic", IsMutation: true, ResourceID: topicID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().RenameTopicWithResponse(ctx, topicID, generated.RenameTopicRequestContent{
			Topic: generated.TopicNamePayload{Name: name},
		})
		if err != nil {
			return err
		}
		if err := CheckResponse(resp.HTTPResponse); err != nil {
			return err
		}
		// A merged topic redirects. net/http may follow that as a GET, so a
		// successful read is not acknowledgement that the name was changed.
		if resp.StatusCode() != http.StatusNoContent {
			err := ErrAPI(resp.StatusCode(), "topic rename was not acknowledged (expected HTTP 204)")
			err.RequestID = resp.HTTPResponse.Header.Get("X-Request-Id")
			return err
		}
		return nil
	})
}

// GetEntries returns entries for a specific topic.
//
// The entry index is paginated by geared_pagination, so the page in params is a cursor out
// of the previous answer's Link header rather than an offset — a number is ignored and
// answered with the first page. This throws that header away; use GetEntriesPage to walk
// the thread.
func (s *TopicsService) GetEntries(ctx context.Context, topicID int64, params *generated.GetTopicEntriesParams) (result *generated.GetTopicEntriesResponseContent, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "GetTopicEntries",
		ResourceType: "entry", IsMutation: false, ResourceID: topicID,
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
	resp, err := s.client.gen.GetTopicEntriesWithResponse(ctx, topicID, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// TopicEntryPage contains one page of a topic's entries and the cursor for the page after
// it. NextPage is empty on the last page, which is how a caller walking a thread is told
// it has read all of it.
type TopicEntryPage struct {
	Entries  []generated.Entry `json:"entries"`
	NextPage string            `json:"next_page"`
}

// GetEntriesPage answers the same entries as GetEntries along with the cursor for the page
// after it, so a caller walking a thread follows HEY's own ordering rather than guessing
// page numbers geared_pagination does not understand.
//
// Pass an empty page for the first page, then the NextPage of each answer.
func (s *TopicsService) GetEntriesPage(ctx context.Context, topicID int64, page string) (result *TopicEntryPage, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "GetTopicEntries",
		ResourceType: "entry", IsMutation: false, ResourceID: topicID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetTopicEntriesWithResponse(ctx, topicID, topicEntriesParams(page))
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = &TopicEntryPage{}
		if resp.JSON200 != nil {
			result.Entries = *resp.JSON200
		}
		if resp.HTTPResponse != nil {
			result.NextPage = gearedPageFromLink(resp.HTTPResponse.Header.Get("Link"))
		}
		return nil
	})
	return result, err
}

func topicEntriesParams(page string) *generated.GetTopicEntriesParams {
	params := &generated.GetTopicEntriesParams{}
	if page != "" {
		params.Page = &page
	}
	return params
}

// GetCommentAudience answers who a note on the topic would reach, as HEY's composer shows
// the person writing one, without adding anything: VisibleTo holds the other users with
// access to the thread, empty when the note would be for its author alone, and
// CollectionOnly is true on a collection's own thread, where the note is seen only on the
// collection.
func (s *TopicsService) GetCommentAudience(ctx context.Context, topicID int64) (audience *generated.TopicCommentAudience, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "GetTopicCommentAudience",
		ResourceType: "comment", IsMutation: false, ResourceID: topicID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetTopicCommentAudienceWithResponse(ctx, topicID)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		if resp.JSON200 == nil {
			return ErrAPI(resp.StatusCode(), fmt.Sprintf("HEY answered HTTP %d without the audience of topic %d", resp.StatusCode(), topicID))
		}
		audience = resp.JSON200
		return nil
	})
	return audience, err
}

// CreateComment adds a note to a topic and returns it as an entry of kind "comment", with
// its ID, TopicId and Content, and who it reaches: VisibleTo and CollectionOnly, as
// GetCommentAudience answers them. VisibleTo is nil from a HEY that does not say, and
// empty when the note is for its author alone.
//
// Everyone with access to the thread sees the note, so on a HEY for Domains thread
// teammates do, and HEY never emails it to anyone. On a thread no one else can reach it
// is a note to self. The content is HTML, as HEY's composer writes it, so escape plain
// text (html.EscapeString) before passing it. HEY refuses blank content with a
// validation error.
func (s *TopicsService) CreateComment(ctx context.Context, topicID int64, content string) (entry *generated.Entry, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "CreateTopicComment",
		ResourceType: "comment", IsMutation: true, ResourceID: topicID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		body := generated.CreateTopicCommentRequestContent{
			Comment: generated.TopicCommentPayload{Content: content},
		}

		resp, rerr := s.client.genClient().CreateTopicCommentWithResponse(ctx, topicID, body)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		if resp.JSON201 == nil {
			// A HEY that predates the JSON answer redirects to the box after adding the
			// note, so a success without the entry may still have written one.
			return ErrAPI(resp.StatusCode(), fmt.Sprintf(
				"HEY answered HTTP %d without the note it created; check topic %d before adding it again",
				resp.StatusCode(), topicID))
		}
		entry = resp.JSON201
		return nil
	})
	return entry, err
}

// SentPage is one page of sent topics and the opaque cursor for the next page.
type SentPage struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Topics      []generated.Topic `json:"topics"`
	NextPage    string            `json:"next_page"`
}

// GetSent returns sent topics.
func (s *TopicsService) GetSent(ctx context.Context, params *generated.GetSentTopicsParams) (result *generated.TopicListResponse, err error) {
	result, _, err = s.getSent(ctx, params)
	return result, err
}

// GetSentPage returns one page of sent topics and preserves the next-page cursor from
// the Link header. An empty page argument reads the first page.
func (s *TopicsService) GetSentPage(ctx context.Context, page string) (result *SentPage, err error) {
	params := &generated.GetSentTopicsParams{}
	if page != "" {
		params.Page = &page
	}

	response, nextPage, err := s.getSent(ctx, params)
	if err != nil {
		return nil, err
	}
	result = &SentPage{NextPage: nextPage}
	if response != nil {
		result.Title = response.Title
		result.Description = response.Description
		result.Topics = response.Topics
	}
	return result, nil
}

func (s *TopicsService) getSent(ctx context.Context, params *generated.GetSentTopicsParams) (result *generated.TopicListResponse, nextPage string, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "GetSentTopics",
		ResourceType: "topic", IsMutation: false,
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
	resp, err := s.client.gen.GetSentTopicsWithResponse(ctx, params)
	if err != nil {
		return nil, "", err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, "", err
	}
	if resp.HTTPResponse != nil {
		nextPage = gearedPageFromLink(resp.HTTPResponse.Header.Get("Link"))
	}
	return resp.JSON200, nextPage, nil
}

// TopicListPage is one page of the Spam, Trash or Everything view: its topics, the opaque
// cursor for the next page (empty on the last page) and the view's total topic count from
// X-Total-Count (zero when HEY does not send one).
type TopicListPage struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Topics      []generated.Topic `json:"topics"`
	NextPage    string            `json:"next_page"`
	TotalCount  int               `json:"total_count"`
}

// GetSpam returns spam topics.
func (s *TopicsService) GetSpam(ctx context.Context, params *generated.GetSpamTopicsParams) (*generated.TopicListResponse, error) {
	response, _, err := s.getSpam(ctx, params)
	return response, err
}

// GetSpamPage returns one page of spam topics with the next-page cursor from the Link
// header and the total count. An empty page argument reads the first page.
func (s *TopicsService) GetSpamPage(ctx context.Context, page string) (*TopicListPage, error) {
	response, header, err := s.getSpam(ctx, &generated.GetSpamTopicsParams{Page: topicListCursor(page)})
	if err != nil {
		return nil, err
	}
	return newTopicListPage(response, header), nil
}

func (s *TopicsService) getSpam(ctx context.Context, params *generated.GetSpamTopicsParams) (result *generated.TopicListResponse, header http.Header, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "GetSpamTopics",
		ResourceType: "topic", IsMutation: false,
	}
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetSpamTopicsWithResponse(ctx, params)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = resp.JSON200
		if resp.HTTPResponse != nil {
			header = resp.HTTPResponse.Header
		}
		return nil
	})
	return result, header, err
}

// GetTrash returns trash topics.
func (s *TopicsService) GetTrash(ctx context.Context, params *generated.GetTrashTopicsParams) (*generated.TopicListResponse, error) {
	response, _, err := s.getTrash(ctx, params)
	return response, err
}

// GetTrashPage returns one page of trash topics with the next-page cursor from the Link
// header and the total count. An empty page argument reads the first page.
func (s *TopicsService) GetTrashPage(ctx context.Context, page string) (*TopicListPage, error) {
	response, header, err := s.getTrash(ctx, &generated.GetTrashTopicsParams{Page: topicListCursor(page)})
	if err != nil {
		return nil, err
	}
	return newTopicListPage(response, header), nil
}

func (s *TopicsService) getTrash(ctx context.Context, params *generated.GetTrashTopicsParams) (result *generated.TopicListResponse, header http.Header, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "GetTrashTopics",
		ResourceType: "topic", IsMutation: false,
	}
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetTrashTopicsWithResponse(ctx, params)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = resp.JSON200
		if resp.HTTPResponse != nil {
			header = resp.HTTPResponse.Header
		}
		return nil
	})
	return result, header, err
}

// GetEverything returns all topics.
func (s *TopicsService) GetEverything(ctx context.Context, params *generated.GetEverythingTopicsParams) (*generated.TopicListResponse, error) {
	response, _, err := s.getEverything(ctx, params)
	return response, err
}

// GetEverythingPage returns one page of the Everything view with the next-page cursor
// from the Link header and the total count. An empty page argument reads the first page.
func (s *TopicsService) GetEverythingPage(ctx context.Context, page string) (*TopicListPage, error) {
	response, header, err := s.getEverything(ctx, &generated.GetEverythingTopicsParams{Page: topicListCursor(page)})
	if err != nil {
		return nil, err
	}
	return newTopicListPage(response, header), nil
}

func (s *TopicsService) getEverything(ctx context.Context, params *generated.GetEverythingTopicsParams) (result *generated.TopicListResponse, header http.Header, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "GetEverythingTopics",
		ResourceType: "topic", IsMutation: false,
	}
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetEverythingTopicsWithResponse(ctx, params)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = resp.JSON200
		if resp.HTTPResponse != nil {
			header = resp.HTTPResponse.Header
		}
		return nil
	})
	return result, header, err
}

func topicListCursor(page string) *string {
	if page == "" {
		return nil
	}
	return &page
}

func newTopicListPage(response *generated.TopicListResponse, header http.Header) *TopicListPage {
	page := &TopicListPage{Topics: []generated.Topic{}}
	if response != nil {
		page.Title = response.Title
		page.Description = response.Description
		if response.Topics != nil {
			page.Topics = response.Topics
		}
	}
	page.NextPage = gearedPageFromLink(header.Get("Link"))
	page.TotalCount, _ = strconv.Atoi(header.Get("X-Total-Count"))
	return page
}

// --- Status and moves ---

// Trash moves a topic to the trash.
//
// Shared topics redirect to a confirmation page unless confirmDestroy is set, so pass it
// whenever the topic might be shared.
func (s *TopicsService) Trash(ctx context.Context, topicID int64, confirmDestroy bool) error {
	op := OperationInfo{
		Service: "Topics", Operation: "TrashTopic",
		ResourceType: "topic", IsMutation: true, ResourceID: topicID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		// confirm_destroy is only sent when set: an empty value would read as
		// truthy on the server and skip the shared-topic confirmation.
		var params generated.TrashTopicParams
		if confirmDestroy {
			one := "1"
			params.ConfirmDestroy = &one
		}
		resp, err := s.client.genClient().TrashTopicWithResponse(ctx, topicID, &params)
		if err != nil {
			return err
		}
		if awaitingRemovalConfirmation(resp.HTTPResponse, topicID) {
			return ErrUsageHint(
				fmt.Sprintf("topic %d is shared; HEY wants confirmation before trashing it", topicID),
				"Call Trash with confirmDestroy=true to trash it and remove your access")
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// awaitingRemovalConfirmation reports whether HEY answered a trash request by
// redirecting to the shared-topic removal confirmation page for that topic
// (/topics/:id/removal/new). The client follows the redirect, so it shows up as a
// non-2xx response whose final request URL is that page.
func awaitingRemovalConfirmation(resp *http.Response, topicID int64) bool {
	if resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return false
	}
	return resp.StatusCode >= 300 && resp.Request.URL.Path == fmt.Sprintf("/topics/%d/removal/new", topicID)
}

// Restore brings a topic back from the trash or the catch-all.
func (s *TopicsService) Restore(ctx context.Context, topicID int64) error {
	op := OperationInfo{
		Service: "Topics", Operation: "RestoreTopic",
		ResourceType: "topic", IsMutation: true, ResourceID: topicID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().RestoreTopicWithResponse(ctx, topicID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// UndoSend calls back the last message the caller sent on a topic, while Undo Send still
// holds it, and answers the entry HEY called back: a draft again, unsent.
//
// HEY picks the message, the caller's last delayed entry on the topic, so compare the
// entry answered with the one meant: two messages sent to one thread within the delay are
// called back last first. A reply that broke out into a thread of its own is called back
// on that thread, the one the send's TopicId names.
//
// HEY answers with a redirect to the draft rather than JSON, the answer its own apps read,
// and the redirect is the only place it names the entry: the SDK takes it as the answer
// rather than follow it. The entry is 0 when the redirect names none. Once the delay has
// run out and the message has gone, or when nothing was held, HEY answers 400: an API
// error whose HTTPStatus is 400, and nothing was changed. It is never sent twice.
func (s *TopicsService) UndoSend(ctx context.Context, topicID int64) (entryID int64, err error) {
	op := OperationInfo{
		Service: "Topics", Operation: "UndoTopicSend",
		ResourceType: "topic", IsMutation: true, ResourceID: topicID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().UndoTopicSendWithResponse(contextWithRedirectAsAnswer(ctx), topicID)
		if rerr != nil {
			return rerr
		}
		if location, redirected := redirectAnswer(resp.HTTPResponse); redirected {
			entryID, _ = UndoneEntryID(location)
			return nil
		}
		if resp.StatusCode() == http.StatusBadRequest {
			nothing := ErrAPI(http.StatusBadRequest, "nothing to undo: the message was already sent, or none was held")
			nothing.RequestID = resp.HTTPResponse.Header.Get("X-Request-Id")
			return nothing
		}
		return CheckResponse(resp.HTTPResponse)
	})
	return entryID, err
}

// UndoneEntryID reads the entry HEY called back out of the redirect an undo send answers
// with, for a caller holding that Location: /messages/{id}/edit for a message on a thread
// of its own, /topics/{topicId}?expanded_draft={id} for a reply. Anything else names no
// entry, and ok is false: the thread alone, which is where HEY's pages send a browser
// with nothing to undo, is not an undo.
func UndoneEntryID(location string) (entryID int64, ok bool) {
	parsed, err := url.Parse(location)
	if err != nil {
		return 0, false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	var named string
	switch {
	case len(parts) == 3 && parts[0] == "messages" && parts[2] == "edit":
		named = parts[1]
	case len(parts) == 2 && parts[0] == "topics":
		named = parsed.Query().Get("expanded_draft")
	}
	id, err := strconv.ParseInt(named, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// MarkHam rescues a topic from spam. Every other spam topic from the same sender comes with it.
func (s *TopicsService) MarkHam(ctx context.Context, topicID int64) error {
	op := OperationInfo{
		Service: "Topics", Operation: "MarkTopicHam",
		ResourceType: "topic", IsMutation: true, ResourceID: topicID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().MarkTopicHamWithResponse(ctx, topicID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// EmptyTrash deletes everything in the trash. The server does this synchronously, so a large
// trash can take a while.
func (s *TopicsService) EmptyTrash(ctx context.Context) error {
	op := OperationInfo{
		Service: "Topics", Operation: "EmptyTrash",
		ResourceType: "topic", IsMutation: true,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().EmptyTrashWithResponse(ctx)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// EmptySpam deletes everything in the spam box. The server does this synchronously, so a large
// spam box can take a while.
func (s *TopicsService) EmptySpam(ctx context.Context) error {
	op := OperationInfo{
		Service: "Topics", Operation: "EmptySpam",
		ResourceType: "topic", IsMutation: true,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().EmptySpamWithResponse(ctx)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Move moves a topic to another box.
//
// The server answers 204 without moving anything when the acting user has no posting for
// the topic, so a success here is not proof the topic moved.
func (s *TopicsService) Move(ctx context.Context, topicID int64, boxID int64) error {
	op := OperationInfo{
		Service: "Topics", Operation: "MoveTopic",
		ResourceType: "topic", IsMutation: true, ResourceID: topicID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().MoveTopicWithResponse(ctx, topicID, generated.MoveTopicRequestContent{BoxId: boxID})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}
