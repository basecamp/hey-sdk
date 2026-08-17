package hey

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
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

// Publication is a thread's public link.
type Publication struct {
	Published bool
	URL       string
}

// Get reports whether a thread is published and, if it is, its public link.
func (s *PublicationsService) Get(ctx context.Context, topicID int64) (result *Publication, err error) {
	op := OperationInfo{
		Service: "Publications", Operation: "GetTopicPublication",
		ResourceType: "publication", IsMutation: false, ResourceID: topicID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.GetHTML(ctx, fmt.Sprintf("/topics/%d/publication/edit", topicID))
		if rerr != nil {
			return rerr
		}

		publicURL := parsePublicationURLHTML(string(resp.Data))
		result = &Publication{Published: publicURL != "", URL: publicURL}
		return nil
	})
	return result, err
}

// Create publishes a thread and returns its public link.
//
// Answers a forbidden error on accounts that aren't eligible to publish.
func (s *PublicationsService) Create(ctx context.Context, topicID int64) (result *Publication, err error) {
	op := OperationInfo{
		Service: "Publications", Operation: "CreateTopicPublication",
		ResourceType: "publication", IsMutation: true, ResourceID: topicID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		if _, perr := s.client.PostForm(ctx, fmt.Sprintf("/topics/%d/publication", topicID), url.Values{}); perr != nil {
			return perr
		}

		// The redirect lands on the sharing panel rather than carrying the link, so read it back.
		publication, gerr := s.Get(ctx, topicID)
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
func parsePublicationURLHTML(page string) string {
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return ""
	}

	var found string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if found != "" {
			return
		}
		if node.Type == html.ElementNode && nodeAttr(node, "data-copy-to-clipboard-target") == "copyable" {
			found = nodeText(node)
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)

	return found
}
