package hey

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// TimeTracksService handles time tracking operations.
type TimeTracksService struct {
	client *Client
}

// NewTimeTracksService creates a new TimeTracksService.
func NewTimeTracksService(client *Client) *TimeTracksService {
	return &TimeTracksService{client: client}
}

// GetOngoing returns the ongoing time track, or nil if none is active.
// Per ADR-004, a 404 response is treated as "no active track" rather than an error.
func (s *TimeTracksService) GetOngoing(ctx context.Context) (result *generated.Recording, err error) {
	op := OperationInfo{
		Service: "TimeTracks", Operation: "GetOngoingTimeTrack",
		ResourceType: "time_track", IsMutation: false,
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
	resp, err := s.client.gen.GetOngoingTimeTrackWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	// ADR-004: 404 means no ongoing time track — return nil, nil
	if err = checkResponseEmptyOn(resp.HTTPResponse, []int{http.StatusNotFound}); err != nil {
		return nil, err
	}
	if resp.HTTPResponse.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	return resp.JSON200, nil
}

// Start starts a new time track and returns it as a recording.
//
// It takes no parameters: haystack ignores the request body here and starts a
// track with defaults. Set title/notes/category afterwards with Update. A 409
// means a track is already ongoing.
func (s *TimeTracksService) Start(ctx context.Context) (result *generated.Recording, err error) {
	op := OperationInfo{
		Service: "TimeTracks", Operation: "StartTimeTrack",
		ResourceType: "time_track", IsMutation: true,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	resp, err := s.client.genClient().StartTimeTrackWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if resp.JSON409 != nil {
		// A track is already running; hand back the server's message so the
		// caller can branch on CodeConflict rather than parse a generic error.
		return nil, ErrConflict(resp.JSON409.Error)
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// Update updates an existing time track.
//
// The body already carries the {calendar_time_track: {...}} wrapper the API expects.
func (s *TimeTracksService) Update(ctx context.Context, timeTrackID int64, body generated.UpdateTimeTrackJSONRequestBody) (result *generated.Recording, err error) {
	op := OperationInfo{
		Service: "TimeTracks", Operation: "UpdateTimeTrack",
		ResourceType: "time_track", IsMutation: true, ResourceID: timeTrackID,
	}
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		result, err = s.update(ctx, timeTrackID, body)
		return err
	})
	return result, err
}

// Stop stops an ongoing time track by setting ends_at to the current time.
// It reports itself to hooks as StopTimeTrack, distinct from UpdateTimeTrack,
// so a gating policy can allow one without the other; the request itself is
// the same PUT that Update sends.
func (s *TimeTracksService) Stop(ctx context.Context, timeTrackID int64) error {
	op := OperationInfo{
		Service: "TimeTracks", Operation: "StopTimeTrack",
		ResourceType: "time_track", IsMutation: true, ResourceID: timeTrackID,
	}
	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		now := time.Now().UTC()
		_, err := s.update(ctx, timeTrackID, generated.UpdateTimeTrackJSONRequestBody{
			CalendarTimeTrack: generated.UpdateTimeTrackPayload{EndsAt: &now},
		})
		return err
	})
}

// update is the shared PUT for Update and Stop: same request, same response
// handling; only the announced operation differs.
func (s *TimeTracksService) update(ctx context.Context, timeTrackID int64, body generated.UpdateTimeTrackJSONRequestBody) (*generated.Recording, error) {
	resp, err := s.client.genClient().UpdateTimeTrackWithResponse(ctx, timeTrackID, body)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// Create records a stretch of time that has already finished.
//
// JSON callers send the fields flat; the server wraps them itself.
func (s *TimeTracksService) Create(ctx context.Context, body generated.CreateTimeTrackJSONRequestBody) (result *generated.Recording, err error) {
	op := OperationInfo{
		Service: "TimeTracks", Operation: "CreateTimeTrack",
		ResourceType: "time_track", IsMutation: true,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().CreateTimeTrackWithResponse(ctx, body)
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

// Delete throws a time track away. timeTrackID is the recording's id.
func (s *TimeTracksService) Delete(ctx context.Context, timeTrackID int64) error {
	op := OperationInfo{
		Service: "TimeTracks", Operation: "DeleteTimeTrack",
		ResourceType: "time_track", IsMutation: true, ResourceID: timeTrackID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().DeleteTimeTrackWithResponse(ctx, timeTrackID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// --- Categories and exports ---

// TimeTrackCategory is one of the labels you can file a time track under.
type TimeTrackCategory struct {
	ID    int64
	Title string
}

// categoryEditLinkRe matches the edit links on the categories page, which is the only place
// the ids appear — the page has no JSON representation.
var categoryEditLinkRe = regexp.MustCompile(`href="[^"]*/calendar/time_tracks/categories/(\d+)/edit"[^>]*>([^<]*)<`)

// Categories returns the time track categories with their ids.
//
// Read from the categories page: the autocomplete endpoint answers JSON but carries titles
// only, and the write endpoints need ids.
func (s *TimeTracksService) Categories(ctx context.Context) (result []TimeTrackCategory, err error) {
	op := OperationInfo{
		Service: "TimeTracks", Operation: "ListTimeTrackCategories",
		ResourceType: "category", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.GetHTML(ctx, "/calendar/time_tracks/categories")
		if rerr != nil {
			return rerr
		}

		for _, match := range categoryEditLinkRe.FindAllStringSubmatch(string(resp.Data), -1) {
			id, perr := strconv.ParseInt(match[1], 10, 64)
			if perr != nil {
				continue
			}
			result = append(result, TimeTrackCategory{ID: id, Title: strings.TrimSpace(match[2])})
		}
		return nil
	})
	return result, err
}

// CreateCategory adds a time track category.
func (s *TimeTracksService) CreateCategory(ctx context.Context, title string) error {
	op := OperationInfo{
		Service: "TimeTracks", Operation: "CreateTimeTrackCategory",
		ResourceType: "category", IsMutation: true,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.PostForm(ctx, "/calendar/time_tracks/categories", categoryForm(title))
		return err
	})
}

// UpdateCategory renames a time track category.
func (s *TimeTracksService) UpdateCategory(ctx context.Context, categoryID int64, title string) error {
	op := OperationInfo{
		Service: "TimeTracks", Operation: "UpdateTimeTrackCategory",
		ResourceType: "category", IsMutation: true, ResourceID: categoryID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.PatchForm(ctx, fmt.Sprintf("/calendar/time_tracks/categories/%d", categoryID), categoryForm(title))
		return err
	})
}

// DeleteCategory removes a time track category. The tracks filed under it stay, uncategorized.
func (s *TimeTracksService) DeleteCategory(ctx context.Context, categoryID int64) error {
	op := OperationInfo{
		Service: "TimeTracks", Operation: "DeleteTimeTrackCategory",
		ResourceType: "category", IsMutation: true, ResourceID: categoryID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.DeleteForm(ctx, fmt.Sprintf("/calendar/time_tracks/categories/%d", categoryID))
		return err
	})
}

// Export returns every completed time track as CSV, newest first, with the columns
// Start, End, Duration, Category and Notes.
func (s *TimeTracksService) Export(ctx context.Context) (result []byte, err error) {
	op := OperationInfo{
		Service: "TimeTracks", Operation: "ExportTimeTracks",
		ResourceType: "time_track", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.GetCSV(ctx, "/calendar/time_tracks/exports")
		if rerr != nil {
			return rerr
		}
		result = resp.Data
		return nil
	})
	return result, err
}

// categoryForm renders a category title as the nested form the server expects.
func categoryForm(title string) url.Values {
	values := url.Values{}
	values.Set("category[title]", title)
	return values
}
