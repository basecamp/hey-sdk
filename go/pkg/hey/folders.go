package hey

import (
	"context"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// FoldersService reads folders — the labels you file threads under.
type FoldersService struct {
	client *Client
}

// NewFoldersService creates a new FoldersService.
func NewFoldersService(client *Client) *FoldersService {
	return &FoldersService{client: client}
}

// Get returns a folder and the postings filed in it.
//
// Creating and deleting folders has no JSON surface; use PostingsService.CreateFolder to
// make one while filing a selection into it.
func (s *FoldersService) Get(ctx context.Context, folderID int64, params *generated.GetFolderParams) (result *generated.FolderWithPostings, err error) {
	op := OperationInfo{
		Service: "Folders", Operation: "GetFolder",
		ResourceType: "folder", IsMutation: false, ResourceID: folderID,
	}

	err = s.client.instrument(ctx, op, func(ctx context.Context) error {
		resp, rerr := s.client.genClient().GetFolderWithResponse(ctx, folderID, params)
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
