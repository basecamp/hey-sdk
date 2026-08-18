package hey

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

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

// StageTopic files a topic into a workflow stage.
func (s *WorkflowsService) StageTopic(ctx context.Context, topicID, workflowID, stageID int64) error {
	op := OperationInfo{
		Service: "Workflows", Operation: "CreateWorkflowStaging",
		ResourceType: "workflow_staging", IsMutation: true, ResourceID: topicID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		values := url.Values{}
		values.Set("workflow_stage_id", strconv.FormatInt(stageID, 10))

		_, err := s.client.PostForm(ctx, fmt.Sprintf("/topics/%d/workflows/%d/stagings", topicID, workflowID), values)
		return err
	})
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
