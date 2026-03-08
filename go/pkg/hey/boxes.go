package hey

import (
	"context"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// BoxesService handles mailbox operations.
type BoxesService struct {
	client *Client
}

// NewBoxesService creates a new BoxesService.
func NewBoxesService(client *Client) *BoxesService {
	return &BoxesService{client: client}
}

// List returns all mailboxes.
func (s *BoxesService) List(ctx context.Context) (result *generated.ListBoxesResponseContent, err error) {
	op := OperationInfo{
		Service: "Boxes", Operation: "ListBoxes",
		ResourceType: "box", IsMutation: false,
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
	resp, err := s.client.gen.ListBoxesWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// Get returns a specific mailbox by ID.
func (s *BoxesService) Get(ctx context.Context, boxID int64, params *generated.GetBoxParams) (result *generated.BoxShowResponse, err error) {
	op := OperationInfo{
		Service: "Boxes", Operation: "GetBox",
		ResourceType: "box", IsMutation: false, ResourceID: boxID,
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
	resp, err := s.client.gen.GetBoxWithResponse(ctx, boxID, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// GetImbox returns the Imbox.
func (s *BoxesService) GetImbox(ctx context.Context, params *generated.GetImboxParams) (result *generated.BoxShowResponse, err error) {
	op := OperationInfo{
		Service: "Boxes", Operation: "GetImbox",
		ResourceType: "box", IsMutation: false,
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
	resp, err := s.client.gen.GetImboxWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// GetFeedbox returns the Feed.
func (s *BoxesService) GetFeedbox(ctx context.Context, params *generated.GetFeedboxParams) (result *generated.BoxShowResponse, err error) {
	op := OperationInfo{
		Service: "Boxes", Operation: "GetFeedbox",
		ResourceType: "box", IsMutation: false,
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
	resp, err := s.client.gen.GetFeedboxWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// GetTrailbox returns the Paper Trail.
func (s *BoxesService) GetTrailbox(ctx context.Context, params *generated.GetTrailboxParams) (result *generated.BoxShowResponse, err error) {
	op := OperationInfo{
		Service: "Boxes", Operation: "GetTrailbox",
		ResourceType: "box", IsMutation: false,
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
	resp, err := s.client.gen.GetTrailboxWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// GetAsidebox returns the Set Aside box.
func (s *BoxesService) GetAsidebox(ctx context.Context, params *generated.GetAsideboxParams) (result *generated.BoxShowResponse, err error) {
	op := OperationInfo{
		Service: "Boxes", Operation: "GetAsidebox",
		ResourceType: "box", IsMutation: false,
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
	resp, err := s.client.gen.GetAsideboxWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// GetLaterbox returns the Reply Later box.
func (s *BoxesService) GetLaterbox(ctx context.Context, params *generated.GetLaterboxParams) (result *generated.BoxShowResponse, err error) {
	op := OperationInfo{
		Service: "Boxes", Operation: "GetLaterbox",
		ResourceType: "box", IsMutation: false,
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
	resp, err := s.client.gen.GetLaterboxWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// GetBubblebox returns the Bubbled Up box.
func (s *BoxesService) GetBubblebox(ctx context.Context, params *generated.GetBubbleboxParams) (result *generated.BoxShowResponse, err error) {
	op := OperationInfo{
		Service: "Boxes", Operation: "GetBubblebox",
		ResourceType: "box", IsMutation: false,
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
	resp, err := s.client.gen.GetBubbleboxWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}
