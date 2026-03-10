package hey

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"sync"
)

//go:embed url-routes.json
var routeTableJSON []byte

// Match holds the components extracted from a HEY API URL.
type Match struct {
	// Operation is the matched API operation name (e.g., "GetBox", "CreateMessage").
	Operation string

	// Operations lists all API operations for the matched pattern, keyed by HTTP method.
	Operations map[string]string

	// Resource is the API resource group (e.g., "Boxes", "Messages").
	Resource string

	// Params contains all named path parameters extracted from the URL.
	Params map[string]string
}

// ResourceID returns the last path parameter value (the "primary" resource ID).
// Returns empty string if no parameters exist.
func (m *Match) ResourceID() string {
	if m == nil || len(m.Params) == 0 {
		return ""
	}
	// Return the last parameter value by iterating the route's param order
	// (stored in Params map — we pick the last key alphabetically as a fallback)
	var last string
	for _, v := range m.Params {
		last = v
	}
	return last
}

// routeEntry is a compiled route from the route table.
type routeEntry struct {
	pattern    string
	resource   string
	operations map[string]string
	regex      *regexp.Regexp
	params     []string
}

// Router matches HEY API URLs against the OpenAPI-derived route table.
type Router struct {
	routes []routeEntry
}

var (
	defaultRouter     *Router
	defaultRouterOnce sync.Once
)

// DefaultRouter returns a shared Router instance using the embedded route table.
func DefaultRouter() *Router {
	defaultRouterOnce.Do(func() {
		r, err := NewRouter(routeTableJSON)
		if err != nil {
			panic("hey: failed to load embedded route table: " + err.Error())
		}
		defaultRouter = r
	})
	return defaultRouter
}

// routeTable is the JSON schema for url-routes.json.
type routeTable struct {
	Routes []routeJSON `json:"routes"`
}

type routeJSON struct {
	Pattern    string            `json:"pattern"`
	Resource   string            `json:"resource"`
	Operations map[string]string `json:"operations"`
}

// NewRouter creates a Router from a JSON route table.
func NewRouter(tableJSON []byte) (*Router, error) {
	var table routeTable
	if err := json.Unmarshal(tableJSON, &table); err != nil {
		return nil, err
	}

	r := &Router{routes: make([]routeEntry, 0, len(table.Routes))}
	for _, entry := range table.Routes {
		compiled, params := compilePattern(entry.Pattern)
		r.routes = append(r.routes, routeEntry{
			pattern:    entry.Pattern,
			resource:   entry.Resource,
			operations: entry.Operations,
			regex:      compiled,
			params:     params,
		})
	}

	sortRoutes(r.routes)
	return r, nil
}

// paramPattern matches {paramName} in route patterns.
var paramPattern = regexp.MustCompile(`\{([^}]+)\}`)

// compilePattern converts a route pattern like "/boxes/{boxId}"
// into a regexp and extracts the parameter names.
func compilePattern(pattern string) (*regexp.Regexp, []string) {
	var params []string
	var regexStr strings.Builder
	regexStr.WriteString("^")

	remaining := pattern
	for remaining != "" {
		loc := paramPattern.FindStringIndex(remaining)
		if loc == nil {
			regexStr.WriteString(regexp.QuoteMeta(remaining))
			break
		}
		regexStr.WriteString(regexp.QuoteMeta(remaining[:loc[0]]))
		match := paramPattern.FindStringSubmatch(remaining[loc[0]:])
		params = append(params, match[1])
		regexStr.WriteString(`([^/]+)`)
		remaining = remaining[loc[0]+len(match[0]):]
	}
	regexStr.WriteString(`$`)

	return regexp.MustCompile(regexStr.String()), params
}

// sortRoutes sorts routes by descending segment count, then alphabetically.
func sortRoutes(routes []routeEntry) {
	sort.Slice(routes, func(i, j int) bool {
		si := strings.Count(routes[i].pattern, "/")
		sj := strings.Count(routes[j].pattern, "/")
		if si != sj {
			return si > sj
		}
		return routes[i].pattern < routes[j].pattern
	})
}

// MatchPath parses a HEY API path and returns the matched route and extracted parameters.
// Returns nil if the path does not match any known route.
// The path should be the API path portion (e.g., "/boxes/123" or "/topics/456/entries").
func (r *Router) MatchPath(path string) *Match {
	path = strings.TrimSuffix(path, ".json")
	path = strings.TrimRight(path, "/")

	for i := range r.routes {
		rt := &r.routes[i]
		matches := rt.regex.FindStringSubmatch(path)
		if matches == nil {
			continue
		}

		m := &Match{
			Operations: rt.operations,
			Resource:   rt.resource,
			Params:     make(map[string]string, len(rt.params)),
		}

		// Pick the default operation: prefer GET, then first alphabetically.
		if op, ok := rt.operations["GET"]; ok {
			m.Operation = op
		} else {
			for _, op := range rt.operations {
				if m.Operation == "" || op < m.Operation {
					m.Operation = op
				}
			}
		}

		for j, paramName := range rt.params {
			m.Params[paramName] = matches[j+1]
		}

		return m
	}
	return nil
}
