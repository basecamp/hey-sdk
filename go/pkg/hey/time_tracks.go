package hey

import (
	"context"
	"fmt"
	"net/http"
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

// Start starts a new time track.
//
// The HEY API expects the body wrapped as {calendar_time_track: {...}}.
func (s *TimeTracksService) Start(ctx context.Context, body generated.StartTimeTrackJSONRequestBody) (result *generated.Recording, err error) {
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

	wrapped := map[string]any{
		"calendar_time_track": body,
	}

	resp, err := s.client.Post(ctx, "/calendar/ongoing_time_track.json", wrapped)
	if err != nil {
		return nil, err
	}
	var recording generated.Recording
	if err = resp.UnmarshalData(&recording); err != nil {
		return nil, fmt.Errorf("failed to decode time track response: %w", err)
	}
	return &recording, nil
}

// Update updates an existing time track.
//
// The HEY API expects the body wrapped as {calendar_time_track: {...}}.
func (s *TimeTracksService) Update(ctx context.Context, timeTrackID int64, body generated.UpdateTimeTrackJSONRequestBody) (result *generated.Recording, err error) {
	op := OperationInfo{
		Service: "TimeTracks", Operation: "UpdateTimeTrack",
		ResourceType: "time_track", IsMutation: true, ResourceID: timeTrackID,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	wrapped := map[string]any{
		"calendar_time_track": body,
	}

	resp, err := s.client.Put(ctx, fmt.Sprintf("/calendar/time_tracks/%d.json", timeTrackID), wrapped)
	if err != nil {
		return nil, err
	}
	var recording generated.Recording
	if err = resp.UnmarshalData(&recording); err != nil {
		return nil, fmt.Errorf("failed to decode time track response: %w", err)
	}
	return &recording, nil
}

// Stop stops an ongoing time track by setting ends_at to the current time.
func (s *TimeTracksService) Stop(ctx context.Context, timeTrackID int64) (err error) {
	op := OperationInfo{
		Service: "TimeTracks", Operation: "StopTimeTrack",
		ResourceType: "time_track", IsMutation: true, ResourceID: timeTrackID,
	}
	if gater, ok := s.client.hooks.(GatingHooks); ok {
		if ctx, err = gater.OnOperationGate(ctx, op); err != nil {
			return
		}
	}
	start := time.Now()
	ctx = s.client.hooks.OnOperationStart(ctx, op)
	defer func() { s.client.hooks.OnOperationEnd(ctx, op, err, time.Since(start)) }()

	body := map[string]any{
		"calendar_time_track": map[string]any{
			"ends_at": time.Now().UTC().Format(time.RFC3339),
		},
	}

	_, err = s.client.Put(ctx, fmt.Sprintf("/calendar/time_tracks/%d.json", timeTrackID), body)
	return err
}
