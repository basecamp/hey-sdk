package hey

import (
	"context"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// JournalService handles journal entry operations.
//
// A day has at most one journal entry. HEY answers the entry as a calendar recording
// carrying the full text (`content`) and the rich-text HTML (`content_html`); a day with
// no entry answers 204, which the SDK reports as a nil recording.
type JournalService struct {
	client *Client
}

// NewJournalService creates a new JournalService.
func NewJournalService(client *Client) *JournalService {
	return &JournalService{client: client}
}

// JournalPage contains one page of journal entries and the cursor for the page after it.
// NextPage is empty on the last page.
type JournalPage struct {
	Entries  []generated.Recording
	NextPage string
}

// ListPage returns journal entries newest first, optionally filtered by a search query.
// Pass an empty page for the first page, then the NextPage of each answer.
func (s *JournalService) ListPage(ctx context.Context, page, query string) (result *JournalPage, err error) {
	op := OperationInfo{
		Service: "Journal", Operation: "ListJournalEntries",
		ResourceType: "journal_entry", IsMutation: false,
	}
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().ListJournalEntriesWithResponse(ctx, journalListParams(page, query))
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = &JournalPage{}
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

func journalListParams(page, query string) *generated.ListJournalEntriesParams {
	params := &generated.ListJournalEntriesParams{}
	if page != "" {
		params.Page = &page
	}
	if query != "" {
		params.Q = &query
	}
	return params
}

// Get returns the journal entry for a day (YYYY-MM-DD), or nil when the day has none.
func (s *JournalService) Get(ctx context.Context, day string) (result *generated.Recording, err error) {
	op := OperationInfo{
		Service: "Journal", Operation: "GetJournalEntry",
		ResourceType: "journal_entry", IsMutation: false,
	}
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		result, err = s.get(ctx, day)
		return err
	})
	return result, err
}

// GetContent returns the rich-text HTML of the day's journal entry, or "" when there is none.
func (s *JournalService) GetContent(ctx context.Context, day string) (content string, err error) {
	op := OperationInfo{
		Service: "Journal", Operation: "GetJournalContent",
		ResourceType: "journal_entry", IsMutation: false,
	}
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		recording, err := s.get(ctx, day)
		if err != nil || recording == nil {
			return err
		}
		content = recording.ContentHtml
		if content == "" {
			content = recording.Content
		}
		return nil
	})
	return content, err
}

// Update writes the day's journal entry (creating it if needed) and returns it as a
// recording. Empty content removes the entry, in which case the result is nil.
//
// The HEY API expects the body wrapped as {calendar_journal_entry: {content: "..."}}.
func (s *JournalService) Update(ctx context.Context, day string, content string) (result *generated.Recording, err error) {
	op := OperationInfo{
		Service: "Journal", Operation: "UpdateJournalEntry",
		ResourceType: "journal_entry", IsMutation: true,
	}
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().UpdateJournalEntryWithResponse(ctx, day, generated.UpdateJournalEntryRequestContent{
			CalendarJournalEntry: generated.JournalEntryPayload{Content: content},
		})
		if err != nil {
			return err
		}
		if err = CheckResponse(resp.HTTPResponse); err != nil {
			return err
		}
		result = resp.JSON200 // nil on 204: empty content removed the entry
		return nil
	})
	return result, err
}

// get is the un-instrumented read shared by Get and GetContent.
func (s *JournalService) get(ctx context.Context, day string) (*generated.Recording, error) {
	resp, err := s.client.genClient().GetJournalEntryWithResponse(ctx, day)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil // nil on 204: no entry that day
}
