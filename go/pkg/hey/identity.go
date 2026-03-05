package hey

import (
	"context"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// IdentityService handles identity and navigation operations.
type IdentityService struct {
	client *Client
}

// NewIdentityService creates a new IdentityService.
func NewIdentityService(client *Client) *IdentityService {
	return &IdentityService{client: client}
}

// GetIdentity returns the current user's identity.
func (s *IdentityService) GetIdentity(ctx context.Context) (result *generated.Identity, err error) {
	op := OperationInfo{
		Service: "Identity", Operation: "GetIdentity",
		ResourceType: "identity", IsMutation: false,
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
	resp, err := s.client.gen.GetIdentityWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// GetNavigation returns the navigation structure for the current user.
func (s *IdentityService) GetNavigation(ctx context.Context) (result *generated.NavigationResponse, err error) {
	op := OperationInfo{
		Service: "Identity", Operation: "GetNavigation",
		ResourceType: "navigation", IsMutation: false,
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
	resp, err := s.client.gen.GetNavigationWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}
