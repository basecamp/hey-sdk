package hey

import (
	"context"
	"fmt"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// ClearancesService works the Screener: who is waiting to be let in, and letting them in
// or turning them away.
type ClearancesService struct {
	client *Client
}

// NewClearancesService creates a new ClearancesService.
func NewClearancesService(client *Client) *ClearancesService {
	return &ClearancesService{client: client}
}

// PendingCount answers how many senders are waiting, without fetching them.
//
// This is the cheap read HEY's own apps sync for the Screener badge. Use Pending when you
// want the senders themselves, or Summary when you also want the stream to follow.
func (s *ClearancesService) PendingCount(ctx context.Context) (count int, err error) {
	summary, err := s.Summary(ctx)
	if err != nil {
		return 0, err
	}
	if summary == nil {
		return 0, nil
	}
	return int(summary.PendingClearancesCount), nil
}

// Summary answers everything HEY says about the Screener without the queue itself: how
// many senders are waiting, and the signed stream name to subscribe to on HEY's cable
// server to be told when that changes.
//
// It is the same read as PendingCount — the count alone, no queue dragged along.
func (s *ClearancesService) Summary(ctx context.Context) (summary *generated.ClearanceSummary, err error) {
	op := OperationInfo{
		Service: "Clearances", Operation: "GetClearances",
		ResourceType: "clearance", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetClearancesWithResponse(ctx, &generated.GetClearancesParams{})
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		summary = resp.JSON200
		return nil
	})
	return summary, err
}

// ClearancePage contains one page of clearances and the cursor for the page after it.
// PendingCount is what the Screener holds in total and is only answered by PendingPage.
type ClearancePage struct {
	Clearances   []generated.Clearance
	PendingCount int
	NextPage     string
}

// Pending answers the senders waiting to be screened, a page at a time.
//
// Each one carries the petitioner and the most recent entry they sent, so a caller can
// show who is asking and what they wrote without a second read. Pass the page token from
// a previous answer to walk the queue.
func (s *ClearancesService) Pending(ctx context.Context, page string) (summary *generated.ClearanceSummary, err error) {
	op := OperationInfo{
		Service: "Clearances", Operation: "GetClearances",
		ResourceType: "clearance", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetClearancesWithResponse(ctx, pendingClearancesParams(page))
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		summary = resp.JSON200
		return nil
	})
	return summary, err
}

// PendingPage answers the same queue as Pending along with the cursor for the page after
// it, so a caller walking the queue is told when it has reached the end of it.
func (s *ClearancesService) PendingPage(ctx context.Context, page string) (result *ClearancePage, err error) {
	op := OperationInfo{
		Service: "Clearances", Operation: "GetClearances",
		ResourceType: "clearance", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetClearancesWithResponse(ctx, pendingClearancesParams(page))
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = &ClearancePage{}
		if resp.JSON200 != nil {
			result.Clearances = resp.JSON200.Clearances
			result.PendingCount = int(resp.JSON200.PendingClearancesCount)
		}
		if resp.HTTPResponse != nil {
			result.NextPage = gearedPageFromLink(resp.HTTPResponse.Header.Get("Link"))
		}
		return nil
	})
	return result, err
}

// Screen answers the Screener for one sender. Status is ClearanceApproved or
// ClearanceDenied.
//
// The options are all optional: file everything they send into a box instead of the Imbox,
// mark what is already waiting as spam, or mark it seen so it does not arrive unread.
func (s *ClearancesService) Screen(ctx context.Context, clearanceID int64, status string, opts ScreenOptions) (clearance *generated.Clearance, err error) {
	op := OperationInfo{
		Service: "Clearances", Operation: "UpdateClearance",
		ResourceType: "clearance", IsMutation: true, ResourceID: clearanceID,
	}

	if err := validateClearanceStatus(status); err != nil {
		return nil, err
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		body := generated.UpdateClearanceJSONRequestBody{Status: status}
		if opts.DesignationBoxID != 0 {
			body.DesignationBoxId = opts.DesignationBoxID
		}
		if opts.Spam {
			spam := true
			body.Spam = &spam
		}
		if opts.MarkTopicsAsSeen {
			seen := true
			body.MarkTopicsAsSeen = &seen
		}

		resp, rerr := s.client.genClient().UpdateClearanceWithResponse(ctx, clearanceID, body)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		clearance = resp.JSON200
		return nil
	})
	return clearance, err
}

// ScreenOptions carries what to do beyond setting the status.
type ScreenOptions struct {
	// DesignationBoxID files everything the sender sends into that box rather than the Imbox.
	DesignationBoxID int64
	// Spam marks the topics already waiting as spam and trains the filter on them.
	Spam bool
	// MarkTopicsAsSeen screens the sender in without their waiting mail arriving unread.
	MarkTopicsAsSeen bool
}

// ScreenMany screens several senders at once and answers the clearances it changed.
//
// HEY answers 404 when none of the ids belong to the caller. A partial match succeeds and
// answers only what it touched, so compare the answer against what you sent.
func (s *ClearancesService) ScreenMany(ctx context.Context, clearanceIDs []int64, status string, spam bool) (clearances []generated.Clearance, err error) {
	op := OperationInfo{
		Service: "Clearances", Operation: "BulkUpdateClearances",
		ResourceType: "clearance", IsMutation: true,
	}

	if len(clearanceIDs) == 0 {
		return nil, ErrUsage("at least one clearance is required")
	}
	if err := validateClearanceStatus(status); err != nil {
		return nil, err
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		body := generated.BulkUpdateClearancesJSONRequestBody{
			Ids:    joinIDs(clearanceIDs),
			Status: status,
		}
		if spam {
			body.Spam = &spam
		}

		resp, rerr := s.client.genClient().BulkUpdateClearancesWithResponse(ctx, body)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		if resp.JSON200 != nil {
			clearances = resp.JSON200.Clearances
		}
		return nil
	})
	return clearances, err
}

// Punt clears the Screener. Everyone waiting is dropped and reexamined the next time they
// write, so nothing is decided for them.
//
// The work is queued, so the senders are still pending when this returns.
func (s *ClearancesService) Punt(ctx context.Context) error {
	op := OperationInfo{
		Service: "Clearances", Operation: "PuntClearances",
		ResourceType: "clearance", IsMutation: true,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().PuntClearancesWithResponse(ctx)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Screened answers the senders already screened in or out, newest decision first, a page
// at a time.
func (s *ClearancesService) Screened(ctx context.Context, page string) (clearances []generated.Clearance, err error) {
	result, err := s.ScreenedPage(ctx, page)
	if err != nil || result == nil {
		return nil, err
	}
	return result.Clearances, nil
}

// ScreenedPage answers the same decisions as Screened along with the cursor for the page
// after it.
func (s *ClearancesService) ScreenedPage(ctx context.Context, page string) (result *ClearancePage, err error) {
	op := OperationInfo{
		Service: "Clearances", Operation: "GetMyClearances",
		ResourceType: "clearance", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		params := &generated.GetMyClearancesParams{}
		if page != "" {
			params.Page = &page
		}

		resp, rerr := s.client.genClient().GetMyClearancesWithResponse(ctx, params)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = &ClearancePage{}
		if resp.JSON200 != nil {
			result.Clearances = resp.JSON200.Clearances
		}
		if resp.HTTPResponse != nil {
			result.NextPage = gearedPageFromLink(resp.HTTPResponse.Header.Get("Link"))
		}
		return nil
	})
	return result, err
}

// Rescreen changes its mind about a sender already screened in or out.
//
// This is the decided list, not the queue: Screen is what answers a pending sender.
func (s *ClearancesService) Rescreen(ctx context.Context, clearanceID int64, status string) (clearance *generated.Clearance, err error) {
	op := OperationInfo{
		Service: "Clearances", Operation: "UpdateMyClearance",
		ResourceType: "clearance", IsMutation: true, ResourceID: clearanceID,
	}

	if err := validateClearanceStatus(status); err != nil {
		return nil, err
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		body := generated.UpdateMyClearanceJSONRequestBody{Status: status}

		resp, rerr := s.client.genClient().UpdateMyClearanceWithResponse(ctx, clearanceID, body)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		clearance = resp.JSON200
		return nil
	})
	return clearance, err
}

func pendingClearancesParams(page string) *generated.GetClearancesParams {
	include := true
	params := &generated.GetClearancesParams{IncludeClearances: &include}
	if page != "" {
		params.Page = &page
	}
	return params
}

func validateClearanceStatus(status string) error {
	if status != ClearanceApproved && status != ClearanceDenied {
		return &Error{Code: CodeValidation, Message: fmt.Sprintf("clearance status must be %q or %q, got %q", ClearanceApproved, ClearanceDenied, status)}
	}
	return nil
}
