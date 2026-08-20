package hey

import (
	"context"
	"net/url"
	"strconv"
	"strings"

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

// FolderPage contains one page of a folder and its pagination state.
type FolderPage struct {
	Folder     *generated.FolderWithPostings
	NextPage   string
	TotalCount int
}

// Get returns a folder and the postings filed in its requested page.
//
// Creating and deleting folders has no JSON surface; use PostingsService.CreateFolder to
// make one while filing a selection into it.
func (s *FoldersService) Get(ctx context.Context, folderID int64, params *generated.GetFolderParams) (*generated.FolderWithPostings, error) {
	page, err := s.GetPage(ctx, folderID, params)
	if err != nil || page == nil {
		return nil, err
	}
	return page.Folder, nil
}

// GetPage returns a folder page with its next cursor and total posting count.
func (s *FoldersService) GetPage(ctx context.Context, folderID int64, params *generated.GetFolderParams) (result *FolderPage, err error) {
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
		result = &FolderPage{Folder: resp.JSON200}
		if resp.HTTPResponse != nil {
			result.TotalCount, _ = strconv.Atoi(resp.HTTPResponse.Header.Get("X-Total-Count"))
			result.NextPage = folderPageFromLink(resp.HTTPResponse.Header.Get("Link"))
		}
		return nil
	})
	return result, err
}

func folderPageFromLink(header string) string {
	next := folderNextLink(header)
	if next == "" {
		return ""
	}
	parsed, err := url.Parse(next)
	if err != nil {
		return ""
	}
	return parsed.Query().Get("page")
}

func folderNextLink(header string) string {
	remainder := header
	for {
		start := strings.IndexByte(remainder, '<')
		if start < 0 {
			return ""
		}
		remainder = remainder[start+1:]
		end := strings.IndexByte(remainder, '>')
		if end < 0 {
			return ""
		}
		target := remainder[:end]
		after := remainder[end+1:]
		nextStart := strings.IndexByte(after, '<')
		params := after
		if nextStart >= 0 {
			params = after[:nextStart]
		}
		if folderLinkIsNext(params) {
			return target
		}
		if nextStart < 0 {
			return ""
		}
		remainder = after[nextStart:]
	}
}

func folderLinkIsNext(params string) bool {
	for _, param := range strings.Split(params, ";") {
		parts := strings.SplitN(strings.TrimSpace(strings.Trim(param, ",")), "=", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "rel") {
			continue
		}
		for _, relation := range strings.Fields(strings.Trim(parts[1], `"`)) {
			if strings.EqualFold(relation, "next") {
				return true
			}
		}
	}
	return false
}
