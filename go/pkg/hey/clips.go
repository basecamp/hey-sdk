package hey

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// ClipsService handles clips — snippets of a message you saved for later.
//
// Clips have no JSON surface: writes answer with a Turbo Stream and the list is HTML, so the
// list is read off the page.
type ClipsService struct {
	client *Client
}

// NewClipsService creates a new ClipsService.
func NewClipsService(client *Client) *ClipsService {
	return &ClipsService{client: client}
}

// Clip is a saved excerpt of a message.
type Clip struct {
	ID      int64
	Content string
	Title   string
	AppURL  string
}

// List returns the saved clips, newest first.
func (s *ClipsService) List(ctx context.Context) (result []Clip, err error) {
	op := OperationInfo{
		Service: "Clips", Operation: "ListClips",
		ResourceType: "clip", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.GetHTML(ctx, "/clips")
		if rerr != nil {
			return rerr
		}

		result, rerr = parseClipsHTML(string(resp.Data))
		return rerr
	})
	return result, err
}

// Create saves an excerpt of an entry.
func (s *ClipsService) Create(ctx context.Context, entryID int64, content string) error {
	op := OperationInfo{
		Service: "Clips", Operation: "CreateClip",
		ResourceType: "clip", IsMutation: true, ResourceID: entryID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		values := url.Values{}
		values.Set("clip[entry_id]", strconv.FormatInt(entryID, 10))
		values.Set("clip[content]", content)

		_, err := s.client.PostForm(ctx, "/clips", values)
		return err
	})
}

// Delete throws a clip away.
func (s *ClipsService) Delete(ctx context.Context, clipID int64) error {
	op := OperationInfo{
		Service: "Clips", Operation: "DeleteClip",
		ResourceType: "clip", IsMutation: true, ResourceID: clipID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.DeleteForm(ctx, fmt.Sprintf("/clips/%d", clipID))
		return err
	})
}

// clipIDRe pulls the clip id out of the article id the clips page stamps, e.g. clip_9182.
var clipIDRe = regexp.MustCompile(`^clip_(\d+)$`)

// parseClipsHTML reads the clips off the clips page.
func parseClipsHTML(page string) ([]Clip, error) {
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return nil, fmt.Errorf("failed to parse the clips page: %w", err)
	}

	var clips []Clip
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			if parts := clipIDRe.FindStringSubmatch(nodeAttr(node, "id")); parts != nil {
				id, perr := strconv.ParseInt(parts[1], 10, 64)
				if perr == nil {
					clip := Clip{ID: id}
					if name := findNodeByClass(node, "clip-item__name"); name != nil {
						clip.Title = nodeText(name)
						clip.AppURL = nodeAttr(name, "href")
					}
					if content := findNodeByClass(node, "clip-item__content"); content != nil {
						clip.Content = nodeText(content)
					}
					clips = append(clips, clip)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)

	return clips, nil
}

// findNodeByClass returns the first descendant carrying the given class.
func findNodeByClass(node *html.Node, class string) *html.Node {
	if node.Type == html.ElementNode && hasClass(node, class) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findNodeByClass(child, class); found != nil {
			return found
		}
	}
	return nil
}

// hasClass reports whether a node carries the given class.
func hasClass(node *html.Node, class string) bool {
	for _, candidate := range strings.Fields(nodeAttr(node, "class")) {
		if candidate == class {
			return true
		}
	}
	return false
}
