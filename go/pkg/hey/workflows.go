package hey

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// WorkflowsService handles workflows — kanban-style boards of threads.
//
// Workflows have no JSON surface beyond the autocomplete endpoint that enumerates them, so
// writes are form posts and stage names are read off the workflow page.
type WorkflowsService struct {
	client *Client
}

// NewWorkflowsService creates a new WorkflowsService.
func NewWorkflowsService(client *Client) *WorkflowsService {
	return &WorkflowsService{client: client}
}

// Workflow is a board of threads.
type Workflow struct {
	ID          int64
	Name        string
	AccountName string
}

// WorkflowStageTopic is a thread card in a workflow stage.
// StagingID identifies the workflow membership; TopicID identifies the email thread.
type WorkflowStageTopic struct {
	StagingID  int64
	TopicID    int64
	Subject    string
	EntryCount int
}

// WorkflowStageView is the stage page HEY renders, including its thread cards.
type WorkflowStageView struct {
	ID     int64
	Name   string
	Topics []WorkflowStageTopic
}

// List returns the workflows on an account.
//
// The autocomplete endpoint answers bare [id, name, account name] arrays, and answers 304 to
// a conditional request — the SDK never sends one, so this always comes back populated.
func (s *WorkflowsService) List(ctx context.Context, accountID int64) (result []Workflow, err error) {
	op := OperationInfo{
		Service: "Workflows", Operation: "ListWorkflows",
		ResourceType: "workflow", IsMutation: false, ResourceID: accountID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.Get(ctx, fmt.Sprintf("/autocompletable/accounts/%d/workflows", accountID))
		if rerr != nil {
			return rerr
		}

		var rows [][]string
		if derr := json.Unmarshal(resp.Data, &rows); derr != nil {
			return fmt.Errorf("failed to decode the workflow list: %w", derr)
		}

		for _, row := range rows {
			if len(row) < 2 {
				continue
			}
			id, perr := strconv.ParseInt(row[0], 10, 64)
			if perr != nil {
				continue
			}
			workflow := Workflow{ID: id, Name: row[1]}
			if len(row) > 2 {
				workflow.AccountName = row[2]
			}
			result = append(result, workflow)
		}
		return nil
	})
	return result, err
}

// Get returns a workflow with its stages in position order.
func (s *WorkflowsService) Get(ctx context.Context, workflowID int64) (result *generated.Workflow, err error) {
	op := OperationInfo{
		Service: "Workflows", Operation: "GetWorkflow",
		ResourceType: "workflow", IsMutation: false, ResourceID: workflowID,
	}
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetWorkflowWithResponse(ctx, workflowID)
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

// Stages returns a workflow's stages in position order.
func (s *WorkflowsService) Stages(ctx context.Context, workflowID int64) ([]generated.WorkflowStage, error) {
	workflow, err := s.Get(ctx, workflowID)
	if err != nil || workflow == nil {
		return nil, err
	}
	return workflow.Stages, nil
}

// GetStage reads the threads in one workflow stage. HEY serves this resource as HTML.
func (s *WorkflowsService) GetStage(ctx context.Context, workflowID, stageID int64) (result *WorkflowStageView, err error) {
	op := OperationInfo{Service: "Workflows", Operation: "GetWorkflowStage", ResourceType: "workflow_stage", IsMutation: false, ResourceID: stageID}
	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetWorkflowStageWithResponse(ctx, workflowID, stageID, func(_ context.Context, req *http.Request) error {
			req.URL.Path = strings.TrimSuffix(req.URL.Path, ".json")
			if req.URL.RawPath != "" {
				req.URL.RawPath = strings.TrimSuffix(req.URL.RawPath, ".json")
			}
			req.Header.Del("Content-Type")
			req.Header.Set("Accept", "text/html")
			return nil
		})
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result, err = parseWorkflowStageHTML(resp.Body, stageID)
		return err
	})
	return result, err
}

func parseWorkflowStageHTML(source []byte, wantStageID int64) (*WorkflowStageView, error) {
	doc, err := html.Parse(strings.NewReader(string(source)))
	if err != nil {
		return nil, fmt.Errorf("parse workflow stage: %w", err)
	}
	stage := findNode(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode && attr(n, "id") == fmt.Sprintf("container_workflow_stage_%d", wantStageID)
	})
	if stage == nil {
		return nil, fmt.Errorf("workflow stage %d not found", wantStageID)
	}
	result := &WorkflowStageView{ID: wantStageID}
	if heading := findNode(stage, func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == "h2" }); heading != nil {
		result.Name = visibleNodeText(heading)
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.HasPrefix(attr(n, "id"), "topic_") {
			topicID, topicErr := strconv.ParseInt(strings.TrimPrefix(attr(n, "id"), "topic_"), 10, 64)
			if topicErr != nil || topicID <= 0 {
				return
			}
			stagingID, stagingErr := strconv.ParseInt(attr(n, "data-identifier"), 10, 64)
			if stagingErr != nil || stagingID <= 0 {
				return
			}
			topic := WorkflowStageTopic{StagingID: stagingID, TopicID: topicID}
			if title := findNode(n, func(x *html.Node) bool { return x.Type == html.ElementNode && x.Data == "h3" }); title != nil {
				topic.Subject = visibleNodeText(title)
			}
			if detail := findNode(n, func(x *html.Node) bool {
				return x.Type == html.ElementNode && x.Data == "p" && strings.Contains(attr(x, "class"), "card__detail")
			}); detail != nil {
				fields := strings.Fields(visibleNodeText(detail))
				if len(fields) == 0 {
					return
				}
				entryCount, countErr := strconv.Atoi(fields[0])
				if countErr != nil || entryCount < 0 {
					return
				}
				topic.EntryCount = entryCount
			}
			result.Topics = append(result.Topics, topic)
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(stage)
	return result, nil
}

func findNode(node *html.Node, match func(*html.Node) bool) *html.Node {
	if match(node) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findNode(child, match); found != nil {
			return found
		}
	}
	return nil
}
func attr(node *html.Node, name string) string {
	for _, value := range node.Attr {
		if value.Key == name {
			return value.Val
		}
	}
	return ""
}
func visibleNodeText(node *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && isVisuallyHidden(n) {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(b.String()), " ")
}

func isVisuallyHidden(node *html.Node) bool {
	for _, class := range strings.Fields(attr(node, "class")) {
		switch class {
		case "sr-only", "screen-reader-only", "u-for-screen-reader", "visually-hidden":
			return true
		}
	}
	return false
}

// Create adds a workflow. accountID of zero leaves the server to pick your first account.
func (s *WorkflowsService) Create(ctx context.Context, name string, accountID int64) error {
	op := OperationInfo{
		Service: "Workflows", Operation: "CreateWorkflow",
		ResourceType: "workflow", IsMutation: true,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		values := url.Values{}
		values.Set("workflow[name]", name)
		if accountID != 0 {
			values.Set("account_id", strconv.FormatInt(accountID, 10))
		}

		_, err := s.client.PostForm(ctx, "/workflows", values)
		return err
	})
}

// Update renames a workflow.
func (s *WorkflowsService) Update(ctx context.Context, workflowID int64, name string) error {
	op := OperationInfo{
		Service: "Workflows", Operation: "UpdateWorkflow",
		ResourceType: "workflow", IsMutation: true, ResourceID: workflowID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		values := url.Values{}
		values.Set("workflow[name]", name)

		_, err := s.client.PatchForm(ctx, fmt.Sprintf("/workflows/%d", workflowID), values)
		return err
	})
}

// Delete throws a workflow away.
func (s *WorkflowsService) Delete(ctx context.Context, workflowID int64) error {
	op := OperationInfo{
		Service: "Workflows", Operation: "DeleteWorkflow",
		ResourceType: "workflow", IsMutation: true, ResourceID: workflowID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.DeleteForm(ctx, fmt.Sprintf("/workflows/%d", workflowID))
		return err
	})
}

// CreateStage adds a column to a workflow. The server names it "Untitled"; rename it with
// UpdateStage.
func (s *WorkflowsService) CreateStage(ctx context.Context, workflowID int64) error {
	op := OperationInfo{
		Service: "Workflows", Operation: "CreateWorkflowStage",
		ResourceType: "workflow_stage", IsMutation: true, ResourceID: workflowID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.PostForm(ctx, fmt.Sprintf("/workflows/%d/stages", workflowID), url.Values{})
		return err
	})
}

// UpdateStage renames a workflow column.
func (s *WorkflowsService) UpdateStage(ctx context.Context, workflowID, stageID int64, name string) error {
	op := OperationInfo{
		Service: "Workflows", Operation: "UpdateWorkflowStage",
		ResourceType: "workflow_stage", IsMutation: true, ResourceID: stageID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		values := url.Values{}
		values.Set("workflow_stage[name]", name)

		_, err := s.client.PatchForm(ctx, fmt.Sprintf("/workflows/%d/stages/%d", workflowID, stageID), values)
		return err
	})
}

// DeleteStage removes a workflow column.
func (s *WorkflowsService) DeleteStage(ctx context.Context, workflowID, stageID int64) error {
	op := OperationInfo{
		Service: "Workflows", Operation: "DeleteWorkflowStage",
		ResourceType: "workflow_stage", IsMutation: true, ResourceID: stageID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.DeleteForm(ctx, fmt.Sprintf("/workflows/%d/stages/%d", workflowID, stageID))
		return err
	})
}

// StageTopic adds a topic to a workflow in the selected stage. HEY creates the workflow
// membership before selecting the stage, so a stage-selection error leaves the topic in the
// workflow's first stage.
func (s *WorkflowsService) StageTopic(ctx context.Context, topicID, workflowID, stageID int64) error {
	op := OperationInfo{
		Service: "Workflows", Operation: "CreateWorkflowStaging",
		ResourceType: "workflow_staging", IsMutation: true, ResourceID: topicID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, err := s.client.genClient().CreateWorkflowStagingWithResponse(ctx, topicID, workflowID, useFormRepresentation)
		if err != nil {
			return err
		}
		if err := CheckResponse(resp.HTTPResponse); err != nil {
			return err
		}
		return s.moveTopic(ctx, topicID, workflowID, stageID)
	})
}

// MoveTopic moves a staged topic to another workflow stage.
func (s *WorkflowsService) MoveTopic(ctx context.Context, topicID, workflowID, stageID int64) error {
	op := OperationInfo{
		Service: "Workflows", Operation: "MoveWorkflowStaging",
		ResourceType: "workflow_staging", IsMutation: true, ResourceID: topicID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		return s.moveTopic(ctx, topicID, workflowID, stageID)
	})
}

func (s *WorkflowsService) moveTopic(ctx context.Context, topicID, workflowID, stageID int64) error {
	values := url.Values{}
	values.Set("workflow_staging[workflow_stage_id]", strconv.FormatInt(stageID, 10))

	resp, err := s.client.genClient().MoveWorkflowStagingWithBodyWithResponse(
		ctx, topicID, workflowID, "application/x-www-form-urlencoded", strings.NewReader(values.Encode()), useFormRepresentation,
	)
	if err != nil {
		return err
	}
	return CheckResponse(resp.HTTPResponse)
}

// UnstageTopic takes a topic back off a workflow.
func (s *WorkflowsService) UnstageTopic(ctx context.Context, topicID, workflowID int64) error {
	op := OperationInfo{
		Service: "Workflows", Operation: "DeleteWorkflowStaging",
		ResourceType: "workflow_staging", IsMutation: true, ResourceID: topicID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.DeleteForm(ctx, fmt.Sprintf("/topics/%d/workflows/%d/stagings", topicID, workflowID))
		return err
	})
}

// workflowStageIDRe pulls the stage id out of the heading id a workflow page stamps, e.g.
// name_workflow_stage_5512.
