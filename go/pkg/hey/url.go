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

	resourceID string // last path parameter value (deterministic)
}

// ResourceID returns the last path parameter value (the "primary" resource ID).
// Returns empty string if no parameters exist.
func (m *Match) ResourceID() string {
	if m == nil {
		return ""
	}
	return m.resourceID
}

// routeEntry is a compiled route from the route table.
type routeEntry struct {
	pattern    string
	resource   string
	operations map[string]string
	regex      *regexp.Regexp
	params     []string
}

type routeMatch struct {
	route     *routeEntry
	captures  []string
	alias     bool
	routeRank int
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
	path = strings.TrimRight(path, "/")
	type routeCandidate struct {
		path            string
		jsonSuffixRoute bool
		alias           bool
	}
	var candidates []routeCandidate
	if strings.HasSuffix(path, ".json") {
		candidates = []routeCandidate{
			{path: path, jsonSuffixRoute: true},
			{path: strings.TrimSuffix(path, ".json"), alias: true},
		}
	} else if path != "" && path != "/" {
		candidates = []routeCandidate{
			{path: path},
			{path: path + ".json", jsonSuffixRoute: true, alias: true},
		}
	} else {
		candidates = []routeCandidate{{path: path}}
	}

	var best *routeMatch
	for _, candidate := range candidates {
		for i := range r.routes {
			rt := &r.routes[i]
			if strings.HasSuffix(rt.pattern, ".json") != candidate.jsonSuffixRoute {
				continue
			}
			matches := rt.regex.FindStringSubmatch(candidate.path)
			if matches == nil {
				continue
			}
			match := &routeMatch{route: rt, captures: matches, alias: candidate.alias, routeRank: i}
			if best == nil || betterRouteMatch(match, best) {
				best = match
			}
		}
	}
	if best == nil {
		return nil
	}

	m := &Match{
		Operations: best.route.operations,
		Resource:   best.route.resource,
		Params:     make(map[string]string, len(best.route.params)),
	}

	// Pick the default operation: prefer GET, then first alphabetically.
	if op, ok := best.route.operations["GET"]; ok {
		m.Operation = op
	} else {
		for _, op := range best.route.operations {
			if m.Operation == "" || op < m.Operation {
				m.Operation = op
			}
		}
	}

	for j, paramName := range best.route.params {
		m.Params[paramName] = best.captures[j+1]
	}
	if len(best.route.params) > 0 {
		m.resourceID = best.captures[len(best.route.params)]
	}

	return m
}

// betterRouteMatch prefers more literal routes before considering whether the
// match used a .json alias. This lets /topics/sent resolve to the literal
// /topics/sent.json route instead of /topics/{topicId}, while an exact
// /entries/{id}/replies route still beats its equally-specific .json sibling.
func betterRouteMatch(candidate, current *routeMatch) bool {
	if len(candidate.route.params) != len(current.route.params) {
		return len(candidate.route.params) < len(current.route.params)
	}
	if candidate.alias != current.alias {
		return !candidate.alias
	}
	return candidate.routeRank < current.routeRank
}
