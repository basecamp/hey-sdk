package hey

import (
	"context"
	"fmt"
	"net/url"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// PublicationsService turns a thread into a public web page.
//
// Publishing has no JSON surface: both writes redirect, and the public link only appears on
// the sharing panel, so Get reads it from there.
type PublicationsService struct {
	client *Client
}

// NewPublicationsService creates a new PublicationsService.
func NewPublicationsService(client *Client) *PublicationsService {
	return &PublicationsService{client: client}
}

// Get reports whether a thread is published and, if it is, its public link.
func (s *PublicationsService) Get(ctx context.Context, topicID int64) (result *generated.TopicPublication, err error) {
	op := OperationInfo{
		Service: "Publications", Operation: "GetTopicPublication",
		ResourceType: "publication", IsMutation: false, ResourceID: topicID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		result, err = s.get(ctx, topicID)
		return err
	})
	return result, err
}

// get is the un-instrumented read shared by Get and Create.
func (s *PublicationsService) get(ctx context.Context, topicID int64) (*generated.TopicPublication, error) {
	resp, err := s.client.genClient().GetTopicPublicationWithResponse(ctx, topicID)
	if err != nil {
		return nil, err
	}
	if err = CheckResponse(resp.HTTPResponse); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// Create publishes a thread and returns its public link.
//
// Answers a forbidden error on accounts that aren't eligible to publish.
func (s *PublicationsService) Create(ctx context.Context, topicID int64) (result *generated.TopicPublication, err error) {
	op := OperationInfo{
		Service: "Publications", Operation: "CreateTopicPublication",
		ResourceType: "publication", IsMutation: true, ResourceID: topicID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		if _, perr := s.client.PostForm(ctx, fmt.Sprintf("/topics/%d/publication", topicID), url.Values{}); perr != nil {
			return perr
		}

		// The redirect lands on the sharing panel rather than carrying the link, so read it
		// back — inside this same operation, not as a second instrumented Get.
		publication, gerr := s.get(ctx, topicID)
		if gerr != nil {
			return gerr
		}
		result = publication
		return nil
	})
	return result, err
}

// Delete unpublishes a thread, breaking its public link.
func (s *PublicationsService) Delete(ctx context.Context, topicID int64) error {
	op := OperationInfo{
		Service: "Publications", Operation: "DeleteTopicPublication",
		ResourceType: "publication", IsMutation: true, ResourceID: topicID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.DeleteForm(ctx, fmt.Sprintf("/topics/%d/publication", topicID))
		return err
	})
}

// parsePublicationURLHTML reads the public link off the sharing panel. The panel only renders
// the copyable link once the thread is published, so an empty answer means unpublished.
