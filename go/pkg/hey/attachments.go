package hey

import (
	"context"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// AttachmentsService handles outgoing attachment uploads.
type AttachmentsService struct {
	client *Client
}

// NewAttachmentsService creates an AttachmentsService.
func NewAttachmentsService(client *Client) *AttachmentsService {
	return &AttachmentsService{client: client}
}

// CreateDirectUpload creates an Active Storage blob and returns the target for
// uploading its bytes. The target URL is self-authenticating and must be used
// with the exact headers returned by HEY.
func (s *AttachmentsService) CreateDirectUpload(ctx context.Context, body generated.CreateDirectUploadJSONRequestBody) (result *generated.DirectUpload, err error) {
	op := OperationInfo{
		Service: "Attachments", Operation: "CreateDirectUpload",
		ResourceType: "attachment", IsMutation: true,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, requestErr := s.client.genClient().CreateDirectUploadWithResponse(ctx, body)
		if requestErr != nil {
			return requestErr
		}
		if responseErr := CheckResponse(resp.HTTPResponse); responseErr != nil {
			return responseErr
		}
		result = resp.JSON200
		return nil
	})
	return result, err
}
