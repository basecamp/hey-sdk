package hey

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// DefaultUserAgent is the default User-Agent header value.
const DefaultUserAgent = "hey-sdk-go/" + Version + " (api:" + APIVersion + ")"

// Client is an HTTP client for the HEY API.
// Unlike the Basecamp SDK, HEY is user-scoped — services hang directly
// off Client with no AccountClient or ForAccount indirection.
//
// Client is safe for concurrent use after construction.
type Client struct {
	httpClient    *http.Client
	tokenProvider TokenProvider
	authStrategy  AuthStrategy
	cfg           *Config
	cache         *Cache
	userAgent     string
	logger        *slog.Logger
	httpOpts      HTTPOptions
	hooks         Hooks

	// Generated client (single shared instance)
	genOnce sync.Once
	gen     *generated.ClientWithResponses

	// Cached default sender ID (lazy-initialized, retries on error)
	senderMu   sync.Mutex
	senderID   int64
	senderDone bool

	// Services (lazy-initialized, protected by mu)
	mu            sync.Mutex
	identity      *IdentityService
	boxes         *BoxesService
	postings      *PostingsService
	topics        *TopicsService
	messages      *MessagesService
	entries       *EntriesService
	contacts      *ContactsService
	calendars     *CalendarsService
	calendarTodos *CalendarTodosService
	habits        *HabitsService
	timeTracks    *TimeTracksService
	journal       *JournalService
	search        *SearchService
}

// Response wraps an API response.
type Response struct {
	Data       json.RawMessage
	StatusCode int
	Headers    http.Header
	FromCache  bool
}

// UnmarshalData unmarshals the response data into the given value.
func (r *Response) UnmarshalData(v any) error {
	return json.Unmarshal(r.Data, v)
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(c *http.Client) ClientOption {
	return func(client *Client) {
		client.httpClient = c
	}
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) ClientOption {
	return func(client *Client) {
		client.userAgent = ua
	}
}

// WithLogger sets a custom slog logger for debug output.
func WithLogger(l *slog.Logger) ClientOption {
	return func(client *Client) {
		if l != nil {
			client.logger = l
		}
	}
}

// WithCache sets a custom cache.
func WithCache(cache *Cache) ClientOption {
	return func(client *Client) {
		client.cache = cache
	}
}

// WithAuthStrategy sets a custom authentication strategy.
func WithAuthStrategy(strategy AuthStrategy) ClientOption {
	return func(client *Client) {
		client.authStrategy = strategy
	}
}

// NewClient creates a new API client.
func NewClient(cfg *Config, tokenProvider TokenProvider, opts ...ClientOption) *Client {
	cfgCopy := *cfg
	c := &Client{
		tokenProvider: tokenProvider,
		cfg:           &cfgCopy,
		userAgent:     DefaultUserAgent,
		logger:        slog.New(discardHandler{}),
		hooks:         NoopHooks{},
		httpOpts:      DefaultHTTPOptions(),
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.authStrategy == nil {
		c.authStrategy = &BearerAuth{TokenProvider: c.tokenProvider}
	}

	if c.httpClient == nil {
		transport := c.httpOpts.Transport
		if transport == nil {
			transport = newDefaultTransport()
		}

		transport = &loggingTransport{inner: transport, client: c}

		c.httpClient = &http.Client{
			Timeout:   c.httpOpts.Timeout,
			Transport: transport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("stopped after 10 redirects")
				}
				if len(via) > 0 && !isSameOrigin(req.URL.String(), via[0].URL.String()) {
					req.Header.Del("Authorization")
				}
				return nil
			},
		}
	}

	if c.cfg.BaseURL != "" && !isLocalhost(c.cfg.BaseURL) {
		if err := requireHTTPS(c.cfg.BaseURL); err != nil {
			panic("hey: base URL must use HTTPS: " + c.cfg.BaseURL)
		}
	}
	if c.httpOpts.Timeout <= 0 {
		panic("hey: timeout must be positive")
	}
	if c.httpOpts.MaxRetries < 0 {
		panic("hey: max retries must be non-negative")
	}
	if c.httpOpts.MaxPages <= 0 {
		panic("hey: max pages must be positive")
	}

	if c.cache == nil && cfg.CacheEnabled {
		c.cache = NewCache(cfg.CacheDir)
	}

	return c
}

// initGeneratedClient initializes the shared generated OpenAPI client.
func (c *Client) initGeneratedClient() {
	c.genOnce.Do(func() {
		serverURL := strings.TrimSuffix(c.cfg.BaseURL, "/")
		authEditor := func(ctx context.Context, req *http.Request) error {
			if err := c.authStrategy.Authenticate(ctx, req); err != nil {
				return err
			}
			req.Header.Set("User-Agent", c.userAgent)
			if req.Header.Get("Content-Type") == "" {
				req.Header.Set("Content-Type", "application/json")
			}
			req.Header.Set("Accept", "application/json")
			return nil
		}
		gen, err := generated.NewClientWithResponses(serverURL,
			generated.WithHTTPClient(c.httpClient),
			generated.WithRequestEditorFn(authEditor))
		if err != nil {
			panic(fmt.Sprintf("hey: failed to create generated client: %v", err))
		}
		c.gen = gen
	})
}

// discardHandler is a slog.Handler that discards all log records.
type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (h discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return h }
func (h discardHandler) WithGroup(string) slog.Handler           { return h }

// Get performs a GET request.
func (c *Client) Get(ctx context.Context, path string) (*Response, error) {
	return c.doRequest(ctx, "GET", path, nil)
}

// Post performs a POST request with a JSON body.
func (c *Client) Post(ctx context.Context, path string, body any) (*Response, error) {
	return c.doRequest(ctx, "POST", path, body)
}

// PostMutation performs a POST mutation with Accept: */*.
// Use this for endpoints where the server may not return JSON.
func (c *Client) PostMutation(ctx context.Context, path string, body any) (*Response, error) {
	return c.doRequest(contextWithAccept(ctx, "*/*"), "POST", path, body)
}

// Put performs a PUT request with a JSON body.
func (c *Client) Put(ctx context.Context, path string, body any) (*Response, error) {
	return c.doRequest(ctx, "PUT", path, body)
}

// Patch performs a PATCH request with a JSON body.
func (c *Client) Patch(ctx context.Context, path string, body any) (*Response, error) {
	return c.doRequest(ctx, "PATCH", path, body)
}

// PatchMutation performs a PATCH mutation with Accept: */*.
// Use this for endpoints where the server may not return JSON.
func (c *Client) PatchMutation(ctx context.Context, path string, body any) (*Response, error) {
	return c.doRequest(contextWithAccept(ctx, "*/*"), "PATCH", path, body)
}

// Delete performs a DELETE request.
func (c *Client) Delete(ctx context.Context, path string) (*Response, error) {
	return c.doRequest(ctx, "DELETE", path, nil)
}

type contextKeyAccept struct{}

func contextWithAccept(ctx context.Context, accept string) context.Context {
	return context.WithValue(ctx, contextKeyAccept{}, accept)
}

func acceptFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(contextKeyAccept{}).(string); ok {
		return v
	}
	return "application/json"
}

func (c *Client) doRequest(ctx context.Context, method, path string, body any) (*Response, error) {
	url, err := c.buildURL(path)
	if err != nil {
		return nil, err
	}
	return c.doRequestURL(ctx, method, url, body)
}

func (c *Client) doRequestURL(ctx context.Context, method, url string, body any) (*Response, error) {
	// Non-idempotent mutations: Don't retry on 429/5xx to avoid duplicating data.
	// Only retry once after successful 401 token refresh.
	if method == "POST" || method == "PATCH" {
		resp, err := c.singleRequest(ctx, method, url, body, 1)
		if err == nil {
			return resp, nil
		}
		if apiErr, ok := err.(*Error); ok && apiErr.Retryable && apiErr.Code == CodeAuth {
			c.logger.Debug("token refreshed, retrying mutation", "method", method)
			info := RequestInfo{Method: method, URL: url, Attempt: 1}
			c.hooks.OnRetry(ctx, info, 2, err)
			return c.singleRequest(ctx, method, url, body, 2)
		}
		return nil, err
	}

	// GET requests: Full retry with exponential backoff
	var attempt int
	var lastErr error

	for attempt = 1; attempt <= c.httpOpts.MaxRetries+1; attempt++ {
		resp, err := c.singleRequest(ctx, method, url, body, attempt)
		if err == nil {
			return resp, nil
		}

		var delay time.Duration
		if re, ok := err.(*retryableError); ok {
			lastErr = re.err
			if re.retryAfter > 0 {
				delay = re.retryAfter
			} else {
				delay = c.backoffDelay(attempt)
			}
		} else if apiErr, ok := err.(*Error); ok {
			if !apiErr.Retryable {
				return nil, err
			}
			lastErr = err
			delay = c.backoffDelay(attempt)
		} else {
			return nil, err
		}

		c.logger.Debug("retrying request", "attempt", attempt, "maxRetries", c.httpOpts.MaxRetries, "delay", delay, "error", lastErr)

		info := RequestInfo{Method: method, URL: url, Attempt: attempt}
		c.hooks.OnRetry(ctx, info, attempt+1, lastErr)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
			continue
		}
	}

	return nil, fmt.Errorf("request failed after %d retries: %w", c.httpOpts.MaxRetries, lastErr)
}

func (c *Client) singleRequest(ctx context.Context, method, url string, body any, attempt int) (*Response, error) {
	ctx = contextWithAttempt(ctx, attempt)

	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = strings.NewReader(string(bodyBytes))
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	if err := c.authStrategy.Authenticate(ctx, req); err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptFromContext(ctx))

	var cacheKey string
	if method == "GET" && c.cache != nil {
		cacheKey = c.cache.Key(url, req.Header.Get("Authorization"))
		if etag := c.cache.GetETag(cacheKey); etag != "" {
			req.Header.Set("If-None-Match", etag)
			c.logger.Debug("cache conditional request", "etag", etag)
		}
	}

	c.logger.Debug("http request", "method", method, "url", url, "attempt", attempt)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ErrNetwork(err)
	}
	defer func() { _ = resp.Body.Close() }()

	c.logger.Debug("http response", "status", resp.StatusCode)

	switch resp.StatusCode {
	case http.StatusNotModified:
		if cacheKey != "" {
			c.logger.Debug("cache hit", "status", 304)
			cached := c.cache.GetBody(cacheKey)
			if cached != nil {
				return &Response{
					Data:       cached,
					StatusCode: http.StatusOK,
					Headers:    resp.Header,
					FromCache:  true,
				}, nil
			}
		}
		return nil, ErrAPI(304, "304 received but no cached response available")

	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		respBody, err := limitedReadAll(resp.Body, MaxResponseBodyBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		if resp.StatusCode == http.StatusNoContent {
			respBody = json.RawMessage("null")
		}

		if method == "GET" && cacheKey != "" {
			if etag := resp.Header.Get("ETag"); etag != "" {
				_ = c.cache.Set(cacheKey, respBody, etag)
				c.logger.Debug("cache stored", "etag", etag)
			}
		}

		return &Response{
			Data:       respBody,
			StatusCode: resp.StatusCode,
			Headers:    resp.Header,
		}, nil

	case http.StatusTooManyRequests:
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		rateErr := ErrRateLimit(retryAfter)
		return nil, &retryableError{err: rateErr, retryAfter: time.Duration(retryAfter) * time.Second}

	case http.StatusUnauthorized:
		if attempt == 1 {
			if authMgr, ok := c.tokenProvider.(*AuthManager); ok {
				if err := authMgr.Refresh(ctx); err == nil {
					return nil, &Error{
						Code:      CodeAuth,
						Message:   "Token refreshed",
						Retryable: true,
					}
				}
			}
		}
		return nil, ErrAuth("Authentication failed")

	case http.StatusForbidden:
		if method != "GET" {
			return nil, ErrForbiddenScope()
		}
		return nil, ErrForbidden("Access denied")

	case http.StatusNotFound:
		return nil, ErrNotFound("Resource", url)

	case http.StatusInternalServerError:
		return nil, ErrAPI(500, "Server error (500)")

	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return nil, &Error{
			Code:       CodeAPI,
			Message:    fmt.Sprintf("Gateway error (%d)", resp.StatusCode),
			HTTPStatus: resp.StatusCode,
			Retryable:  true,
		}

	default:
		respBody, _ := limitedReadAll(resp.Body, MaxErrorBodyBytes)
		var apiErr struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &apiErr) == nil {
			msg := apiErr.Error
			if msg == "" {
				msg = apiErr.Message
			}
			if msg != "" {
				return nil, ErrAPI(resp.StatusCode, truncateString(msg, MaxErrorMessageBytes))
			}
		}
		return nil, ErrAPI(resp.StatusCode, fmt.Sprintf("Request failed (HTTP %d)", resp.StatusCode))
	}
}

func (c *Client) buildURL(path string) (string, error) {
	if strings.HasPrefix(path, "https://") {
		return path, nil
	}
	if strings.HasPrefix(path, "http://") {
		return "", fmt.Errorf("URL must use HTTPS, got: %s", path)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	base := strings.TrimSuffix(c.cfg.BaseURL, "/")
	return base + path, nil
}

func (c *Client) backoffDelay(attempt int) time.Duration {
	delay := c.httpOpts.BaseDelay * time.Duration(1<<(attempt-1))
	jitter := time.Duration(rand.Int63n(int64(c.httpOpts.MaxJitter))) // #nosec G404 -- jitter doesn't need cryptographic randomness
	return delay + jitter
}

// parseNextLink extracts the next URL from a Link header.
func parseNextLink(linkHeader string) string {
	if linkHeader == "" {
		return ""
	}

	for part := range strings.SplitSeq(linkHeader, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, `rel="next"`) {
			start := strings.Index(part, "<")
			end := strings.Index(part, ">")
			if start >= 0 && end > start {
				return part[start+1 : end]
			}
		}
	}

	return ""
}

// parseRetryAfter parses the Retry-After header value.
func parseRetryAfter(header string) int {
	if header == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(header); err == nil && seconds > 0 {
		return seconds
	}
	if t, err := http.ParseTime(header); err == nil {
		seconds := int(time.Until(t).Seconds())
		if seconds > 0 {
			return seconds
		}
	}
	return 0
}

// Config returns a copy of the client configuration.
func (c *Client) Config() Config {
	return *c.cfg
}

// Service accessors — lazy initialization, protected by mutex.

// Identity returns the IdentityService.
func (c *Client) Identity() *IdentityService {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.identity == nil {
		c.identity = NewIdentityService(c)
	}
	return c.identity
}

// Boxes returns the BoxesService.
func (c *Client) Boxes() *BoxesService {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.boxes == nil {
		c.boxes = NewBoxesService(c)
	}
	return c.boxes
}

// Postings returns the PostingsService.
func (c *Client) Postings() *PostingsService {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.postings == nil {
		c.postings = NewPostingsService(c)
	}
	return c.postings
}

// Topics returns the TopicsService.
func (c *Client) Topics() *TopicsService {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.topics == nil {
		c.topics = NewTopicsService(c)
	}
	return c.topics
}

// Messages returns the MessagesService.
func (c *Client) Messages() *MessagesService {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.messages == nil {
		c.messages = NewMessagesService(c)
	}
	return c.messages
}

// Entries returns the EntriesService.
func (c *Client) Entries() *EntriesService {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = NewEntriesService(c)
	}
	return c.entries
}

// Contacts returns the ContactsService.
func (c *Client) Contacts() *ContactsService {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.contacts == nil {
		c.contacts = NewContactsService(c)
	}
	return c.contacts
}

// Calendars returns the CalendarsService.
func (c *Client) Calendars() *CalendarsService {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.calendars == nil {
		c.calendars = NewCalendarsService(c)
	}
	return c.calendars
}

// CalendarTodos returns the CalendarTodosService.
func (c *Client) CalendarTodos() *CalendarTodosService {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.calendarTodos == nil {
		c.calendarTodos = NewCalendarTodosService(c)
	}
	return c.calendarTodos
}

// Habits returns the HabitsService.
func (c *Client) Habits() *HabitsService {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.habits == nil {
		c.habits = NewHabitsService(c)
	}
	return c.habits
}

// TimeTracks returns the TimeTracksService.
func (c *Client) TimeTracks() *TimeTracksService {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.timeTracks == nil {
		c.timeTracks = NewTimeTracksService(c)
	}
	return c.timeTracks
}

// Journal returns the JournalService.
func (c *Client) Journal() *JournalService {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.journal == nil {
		c.journal = NewJournalService(c)
	}
	return c.journal
}

// Search returns the SearchService.
func (c *Client) Search() *SearchService {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.search == nil {
		c.search = NewSearchService(c)
	}
	return c.search
}

// DefaultSenderID returns the current user's default sender contact ID.
// The result is cached after the first successful call. Transient errors
// are not cached, so subsequent calls will retry the identity fetch.
// This is required for mutation operations that need an acting_sender_id.
func (c *Client) DefaultSenderID(ctx context.Context) (int64, error) {
	c.senderMu.Lock()
	defer c.senderMu.Unlock()

	if c.senderDone {
		return c.senderID, nil
	}

	identity, err := c.Identity().GetIdentity(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch identity for sender ID: %w", err)
	}
	if identity == nil {
		return 0, ErrAPI(0, "could not fetch identity")
	}
	for _, s := range identity.Senders {
		if s.Default {
			c.senderID = s.Id
			c.senderDone = true
			return c.senderID, nil
		}
	}
	if len(identity.Senders) > 0 {
		c.senderID = identity.Senders[0].Id
		c.senderDone = true
		return c.senderID, nil
	}
	if identity.PrimaryContact.Id > 0 {
		c.senderID = identity.PrimaryContact.Id
		c.senderDone = true
		return c.senderID, nil
	}
	return 0, ErrAPI(0, "no sender found in identity")
}
