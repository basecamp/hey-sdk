package hey

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func gearedPageFromLink(header string) string {
	next := gearedNextLink(header)
	if next == "" {
		return ""
	}
	parsed, err := url.Parse(next)
	if err != nil {
		return ""
	}
	return parsed.Query().Get("page")
}

func gearedNextLink(header string) string {
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
		if gearedLinkIsNext(params) {
			return target
		}
		if nextStart < 0 {
			return ""
		}
		remainder = after[nextStart:]
	}
}

func gearedLinkIsNext(params string) bool {
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

// FollowPagination fetches additional pages following Link headers from an HTTP response.
// firstPageCount is the number of items already collected from the first page.
// limit is the maximum total items to return (0 = unlimited).
// Returns raw JSON items from subsequent pages only.
func (c *Client) FollowPagination(ctx context.Context, httpResp *http.Response, firstPageCount, limit int) ([]json.RawMessage, error) {
	if httpResp == nil {
		return nil, nil
	}

	if limit > 0 && firstPageCount >= limit {
		return nil, nil
	}

	nextLink := parseNextLink(httpResp.Header.Get("Link"))
	if nextLink == "" {
		return nil, nil
	}

	if httpResp.Request == nil || httpResp.Request.URL == nil {
		return nil, fmt.Errorf("cannot follow pagination: response has no request URL (required for same-origin validation)")
	}
	baseURL := httpResp.Request.URL.String()

	nextURL := resolveURL(baseURL, nextLink)

	parsedURL, err := url.Parse(nextURL)
	if err != nil || !parsedURL.IsAbs() {
		return nil, fmt.Errorf("failed to resolve Link header URL %q against %q", nextLink, baseURL)
	}

	if !isSameOrigin(baseURL, nextURL) {
		return nil, fmt.Errorf("pagination Link header points to different origin: %s", nextURL)
	}

	var allResults []json.RawMessage
	currentCount := firstPageCount
	var page int

	for page = 2; page <= c.httpOpts.MaxPages && nextURL != ""; page++ {
		currentPageURL := nextURL

		resp, err := c.doRequestURL(ctx, "GET", nextURL, nil)
		if err != nil {
			return nil, err
		}

		var items []json.RawMessage
		if err := json.Unmarshal(resp.Data, &items); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		allResults = append(allResults, items...)
		currentCount += len(items)

		if limit > 0 && currentCount >= limit {
			excess := currentCount - limit
			if excess > 0 && len(allResults) > excess {
				allResults = allResults[:len(allResults)-excess]
			}
			break
		}

		nextLink = parseNextLink(resp.Headers.Get("Link"))
		if nextLink == "" {
			break
		}
		nextURL = resolveURL(currentPageURL, nextLink)

		if !isSameOrigin(baseURL, nextURL) {
			return nil, fmt.Errorf("pagination Link header points to different origin: %s", nextURL)
		}
	}

	if page > c.httpOpts.MaxPages {
		c.logger.Warn("pagination capped", "maxPages", c.httpOpts.MaxPages)
	}

	return allResults, nil
}

// GetAll fetches all pages for a paginated resource.
func (c *Client) GetAll(ctx context.Context, path string) ([]json.RawMessage, error) {
	return c.GetAllWithLimit(ctx, path, 0)
}

// GetAllWithLimit fetches pages for a paginated resource up to a limit.
func (c *Client) GetAllWithLimit(ctx context.Context, path string, limit int) ([]json.RawMessage, error) {
	var allResults []json.RawMessage
	baseURL, err := c.buildURL(path)
	if err != nil {
		return nil, err
	}
	url := baseURL
	var page int

	for page = 1; page <= c.httpOpts.MaxPages; page++ {
		resp, err := c.doRequestURL(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}

		var items []json.RawMessage
		if err := json.Unmarshal(resp.Data, &items); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		allResults = append(allResults, items...)

		if limit > 0 && len(allResults) >= limit {
			allResults = allResults[:limit]
			break
		}

		nextURL := parseNextLink(resp.Headers.Get("Link"))
		if nextURL == "" {
			break
		}
		nextURL = resolveURL(url, nextURL)
		if !isSameOrigin(nextURL, baseURL) {
			return nil, fmt.Errorf("pagination Link header points to different origin: %s", nextURL)
		}
		url = nextURL
	}

	if page > c.httpOpts.MaxPages {
		c.logger.Warn("pagination capped", "maxPages", c.httpOpts.MaxPages)
	}

	return allResults, nil
}
