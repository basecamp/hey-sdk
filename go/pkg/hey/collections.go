package hey

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// CollectionsService handles collections — shared threads gathered under one name.
type CollectionsService struct {
	client *Client
}

// NewCollectionsService creates a new CollectionsService.
func NewCollectionsService(client *Client) *CollectionsService {
	return &CollectionsService{client: client}
}

// CreateCollectionParams contains the parameters for creating a collection.
type CreateCollectionParams struct {
	// Name is what the collection is called.
	Name string
	// Summary is the optional blurb shown under the name.
	Summary string
	// AccountID picks which account owns it. Zero leaves the server to pick your first.
	AccountID int64
}

// UpdateCollectionParams contains the parameters for editing a collection.
// Empty fields are left alone.
type UpdateCollectionParams struct {
	Name    string
	Summary string
}

// CollectionPage contains one page of a collection and its pagination state.
type CollectionPage struct {
	Collection *generated.CollectionWithPostings
	NextPage   string
	TotalCount int
}

// List returns your collections.
func (s *CollectionsService) List(ctx context.Context) (result *generated.ListCollectionsResponseContent, err error) {
	op := OperationInfo{
		Service: "Collections", Operation: "ListCollections",
		ResourceType: "collection", IsMutation: false,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().ListCollectionsWithResponse(ctx)
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

// Get returns a collection and the active, accessible threads in its requested page.
func (s *CollectionsService) Get(ctx context.Context, collectionID int64, params *generated.GetCollectionParams) (*generated.CollectionWithPostings, error) {
	page, err := s.GetPage(ctx, collectionID, params)
	if err != nil || page == nil {
		return nil, err
	}
	return page.Collection, nil
}

// GetPage returns a collection page with its next cursor and total posting count.
func (s *CollectionsService) GetPage(ctx context.Context, collectionID int64, params *generated.GetCollectionParams) (result *CollectionPage, err error) {
	op := OperationInfo{
		Service: "Collections", Operation: "GetCollection",
		ResourceType: "collection", IsMutation: false, ResourceID: collectionID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetCollectionWithResponse(ctx, collectionID, params)
		if rerr != nil {
			return rerr
		}
		if cerr := CheckResponse(resp.HTTPResponse); cerr != nil {
			return cerr
		}
		result = &CollectionPage{Collection: resp.JSON200}
		if resp.HTTPResponse != nil {
			result.TotalCount, _ = strconv.Atoi(resp.HTTPResponse.Header.Get("X-Total-Count"))
			result.NextPage = gearedPageFromLink(resp.HTTPResponse.Header.Get("Link"))
		}
		return nil
	})
	return result, err
}

// Update renames a collection or changes its summary.
func (s *CollectionsService) Update(ctx context.Context, collectionID int64, params UpdateCollectionParams) error {
	op := OperationInfo{
		Service: "Collections", Operation: "UpdateCollection",
		ResourceType: "collection", IsMutation: true, ResourceID: collectionID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		// Empty strings are omitted by the payload's omitempty tags, so unset
		// fields are left alone server-side.
		resp, err := s.client.genClient().UpdateCollectionWithResponse(ctx, collectionID, generated.UpdateCollectionRequestContent{
			Collection: generated.CollectionPayload{Name: params.Name, Summary: params.Summary},
		})
		if err != nil {
			return err
		}
		return CheckResponse(resp.HTTPResponse)
	})
}

// Create makes a new collection.
//
// HEY has no JSON endpoint for this — the form post answers with a redirect to the
// collections index rather than the new collection, so the id is not returned. List
// afterwards to find it.
func (s *CollectionsService) Create(ctx context.Context, params CreateCollectionParams) error {
	op := OperationInfo{
		Service: "Collections", Operation: "CreateCollection",
		ResourceType: "collection", IsMutation: true,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		values := url.Values{}
		values.Set("collection[name]", params.Name)
		if params.Summary != "" {
			values.Set("collection[summary]", params.Summary)
		}
		if params.AccountID != 0 {
			values.Set("account_id", fmt.Sprintf("%d", params.AccountID))
		}

		_, err := s.client.PostForm(ctx, "/collections", values)
		return err
	})
}

// AddTopic files a topic into a collection.
//
// HEY has no JSON endpoint for this — the form post answers with a redirect to the topic.
func (s *CollectionsService) AddTopic(ctx context.Context, topicID int64, collectionID int64) error {
	op := OperationInfo{
		Service: "Collections", Operation: "CreateTopicCollecting",
		ResourceType: "collecting", IsMutation: true, ResourceID: topicID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.PostForm(ctx, fmt.Sprintf("/topics/%d/collecting?collection_id=%d", topicID, collectionID), url.Values{})
		return err
	})
}

// RemoveTopic takes a topic back out of a collection.
//
// HEY has no JSON endpoint for this — the form post answers with a redirect to the topic.
// Shadowed topics are silently left alone.
func (s *CollectionsService) RemoveTopic(ctx context.Context, topicID int64, collectionID int64) error {
	op := OperationInfo{
		Service: "Collections", Operation: "DeleteTopicCollecting",
		ResourceType: "collecting", IsMutation: true, ResourceID: topicID,
	}

	return s.client.instrument(ctx, op, func(ctx context.Context) error {
		_, err := s.client.DeleteForm(ctx, fmt.Sprintf("/topics/%d/collecting?collection_id=%d", topicID, collectionID))
		return err
	})
}
