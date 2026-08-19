package hey

import (
	"context"
	"crypto/md5" //nolint:gosec // Active Storage requires a base64-encoded MD5 integrity checksum.
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

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

// Upload reserves an Active Storage blob and uploads the supplied bytes to its
// self-authenticating storage URL. The returned attachment can be embedded in
// rich text with its AttachableSgid.
func (s *AttachmentsService) Upload(ctx context.Context, filename, contentType string, content io.ReadSeeker) (*generated.DirectUpload, error) {
	if filename == "" {
		return nil, ErrUsage("an attachment needs a filename")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if content == nil {
		return nil, ErrUsage("an attachment needs content")
	}

	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek attachment: %w", err)
	}
	hash := md5.New() //nolint:gosec // Active Storage requires MD5 for its Content-MD5 check.
	byteSize, err := io.Copy(hash, content)
	if err != nil {
		return nil, fmt.Errorf("checksum attachment: %w", err)
	}
	checksum := base64.StdEncoding.EncodeToString(hash.Sum(nil))
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind attachment: %w", err)
	}

	upload, err := s.CreateDirectUpload(ctx, generated.CreateDirectUploadRequestContent{
		Blob: generated.DirectUploadBlob{
			Filename:    filename,
			ByteSize:    byteSize,
			Checksum:    checksum,
			ContentType: contentType,
		},
	})
	if err != nil {
		return nil, err
	}
	if upload == nil || upload.DirectUpload.Url == "" {
		return nil, fmt.Errorf("HEY returned an empty attachment upload target")
	}
	if err := RequireSecureEndpoint(upload.DirectUpload.Url); err != nil {
		return nil, fmt.Errorf("unsafe attachment upload target: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, upload.DirectUpload.Url, content)
	if err != nil {
		return nil, fmt.Errorf("create attachment upload request: %w", err)
	}
	req.ContentLength = byteSize
	for name, value := range upload.DirectUpload.Headers {
		req.Header.Set(name, value)
	}

	resp, err := s.client.httpClient.Do(req)
	if err != nil {
		return nil, ErrNetwork(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := CheckResponse(resp); err != nil {
		return nil, err
	}
	return upload, nil
}
