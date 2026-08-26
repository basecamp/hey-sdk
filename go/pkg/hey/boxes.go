package hey

import (
	"context"
	"strconv"
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

// BoxPage contains one page of a box and its pagination state.
type BoxPage struct {
	Box        *generated.BoxShowResponse
	NextPage   string
	TotalCount int
}

// Get returns a specific mailbox by ID.
func (s *BoxesService) Get(ctx context.Context, boxID int64, params *generated.GetBoxParams) (*generated.BoxShowResponse, error) {
	page, err := s.GetPage(ctx, boxID, params)
	if err != nil || page == nil {
		return nil, err
	}
	return page.Box, nil
}

// GetPage returns a box page with its next cursor and total posting count.
func (s *BoxesService) GetPage(ctx context.Context, boxID int64, params *generated.GetBoxParams) (result *BoxPage, err error) {
	op := OperationInfo{
		Service: "Boxes", Operation: "GetBox",
		ResourceType: "box", IsMutation: false, ResourceID: boxID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetBoxWithResponse(ctx, boxID, params)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = &BoxPage{Box: resp.JSON200}
		if resp.HTTPResponse != nil {
			result.TotalCount, _ = strconv.Atoi(resp.HTTPResponse.Header.Get("X-Total-Count"))
			result.NextPage = gearedPageFromLink(resp.HTTPResponse.Header.Get("Link"))
		}
		return nil
	})
	return result, err
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

// GetImboxSeen returns the Imbox's Previously Seen postings, ordered by when
// they were seen (observed_at desc). The response's next_history_url names the
// /imbox route, but its page cursor belongs to the seen scope — extract the
// cursor and feed it back to GetImboxSeen, never to GetImbox.
func (s *BoxesService) GetImboxSeen(ctx context.Context, params *generated.GetImboxSeenParams) (result *generated.BoxShowResponse, err error) {
	op := OperationInfo{
		Service: "Boxes", Operation: "GetImboxSeen",
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
	resp, err := s.client.gen.GetImboxSeenWithResponse(ctx, params)
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

// --- Set Aside groups and observation ---

// ListGroups returns the Set Aside groups in a box. The API answers with ids only.
func (s *BoxesService) ListGroups(ctx context.Context, boxID int64) (result *generated.BoxGroupsResponse, err error) {
	op := OperationInfo{
		Service: "Boxes", Operation: "ListBoxGroups",
		ResourceType: "box_group", IsMutation: false, ResourceID: boxID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().ListBoxGroupsWithResponse(ctx, boxID)
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

// CreateGroup gathers a selection of postings into a new Set Aside group.
func (s *BoxesService) CreateGroup(ctx context.Context, boxID int64, postingIDs []int64) (result *generated.BoxGroup, err error) {
	op := OperationInfo{
		Service: "Boxes", Operation: "CreateBoxGroup",
		ResourceType: "box_group", IsMutation: true, ResourceID: boxID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().CreateBoxGroupWithResponse(ctx, boxID, generated.CreateBoxGroupRequestContent{PostingIds: postingIDs})
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

// DeleteGroup breaks up a Set Aside group, sending its postings back to Previously Seen.
func (s *BoxesService) DeleteGroup(ctx context.Context, boxID int64, groupID int64) error {
	op := OperationInfo{
		Service: "Boxes", Operation: "DeleteBoxGroup",
		ResourceType: "box_group", IsMutation: true, ResourceID: groupID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().DeleteBoxGroupWithResponse(ctx, boxID, groupID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// MarkSeen marks everything in a box as seen. The server queues the work, so postings can
// still read as unseen right after this returns.
func (s *BoxesService) MarkSeen(ctx context.Context, boxID int64) error {
	op := OperationInfo{
		Service: "Boxes", Operation: "MarkBoxSeen",
		ResourceType: "box", IsMutation: true, ResourceID: boxID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().MarkBoxSeenWithResponse(ctx, boxID)
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}
