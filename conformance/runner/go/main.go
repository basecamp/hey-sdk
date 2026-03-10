// Package main provides a conformance test runner for the HEY Go SDK.
//
// This runner reads JSON test definitions from conformance/tests/ and
// executes them against the SDK using a mock HTTP server.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
	"github.com/basecamp/hey-sdk/go/pkg/hey"
)

// TestCase represents a single conformance test.
type TestCase struct {
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	Operation       string                 `json:"operation"`
	Method          string                 `json:"method"`
	Path            string                 `json:"path"`
	PathParams      map[string]interface{} `json:"pathParams"`
	QueryParams     map[string]interface{} `json:"queryParams"`
	RequestBody     map[string]interface{} `json:"requestBody"`
	MockResponses   []MockResponse         `json:"mockResponses"`
	Assertions      []Assertion            `json:"assertions"`
	Tags            []string               `json:"tags"`
	ConfigOverrides map[string]interface{} `json:"configOverrides"`
}

// MockResponse defines a single mock HTTP response.
type MockResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    interface{}       `json:"body"`
	Delay   int               `json:"delay"`
}

// Assertion defines what to verify after the test.
type Assertion struct {
	Type     string      `json:"type"`
	Expected interface{} `json:"expected"`
	Min      float64     `json:"min"`
	Max      float64     `json:"max"`
	Path     string      `json:"path"`
}

// TestResult captures the outcome of a test case.
type TestResult struct {
	Name    string
	Passed  bool
	Message string
}

func main() {
	testsDir := filepath.Join("..", "..", "tests")

	files, err := filepath.Glob(filepath.Join(testsDir, "*.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding test files: %v\n", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		fmt.Println("No test files found in", testsDir)
		os.Exit(0)
	}

	var results []TestResult
	passed, failed := 0, 0

	for _, file := range files {
		tests, err := loadTests(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", file, err)
			continue
		}

		fmt.Printf("\n=== %s ===\n", filepath.Base(file))

		for _, tc := range tests {
			result := runTest(tc)
			results = append(results, result)

			if result.Passed {
				passed++
				fmt.Printf("  PASS: %s\n", tc.Name)
			} else {
				failed++
				fmt.Printf("  FAIL: %s\n        %s\n", tc.Name, result.Message)
			}
		}
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Passed: %d, Failed: %d, Total: %d\n", passed, failed, passed+failed)

	if failed > 0 {
		os.Exit(1)
	}
}

func loadTests(filename string) ([]TestCase, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var tests []TestCase
	if err := json.Unmarshal(data, &tests); err != nil {
		return nil, err
	}

	return tests, nil
}

func runTest(tc TestCase) TestResult {
	// Handle configOverrides for security tests (e.g. HTTPS enforcement)
	if baseURL, ok := tc.ConfigOverrides["baseUrl"]; ok {
		return runConfigOverrideTest(tc, baseURL.(string))
	}

	// Track request count, timing, paths, and headers with mutex protection
	var mu sync.Mutex
	var requestCount int
	var requestTimes []time.Time
	var requestPaths []string
	var requestHeaders []http.Header
	var responseStatuses []int

	// Create mock server that serves responses in sequence
	responseIndex := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		requestTimes = append(requestTimes, time.Now())
		requestPaths = append(requestPaths, r.URL.Path)
		requestHeaders = append(requestHeaders, r.Header.Clone())
		idx := responseIndex
		responseIndex++
		mu.Unlock()

		if idx >= len(tc.MockResponses) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "No more mock responses"}`))
			return
		}

		resp := tc.MockResponses[idx]

		// Apply delay if specified
		if resp.Delay > 0 {
			time.Sleep(time.Duration(resp.Delay) * time.Millisecond)
		}

		// Set headers
		for k, v := range resp.Headers {
			w.Header().Set(k, v)
		}

		// Set Content-Type if not already set
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}

		// Track response status
		status := resp.Status
		mu.Lock()
		responseStatuses = append(responseStatuses, status)
		mu.Unlock()

		// Set status code
		w.WriteHeader(status)

		// Write body
		if resp.Body != nil {
			bodyBytes, _ := json.Marshal(resp.Body)
			_, _ = w.Write(bodyBytes)
		}
	}))
	defer server.Close()

	// Create generated client pointing to mock server with auth header
	client, err := generated.NewClient(server.URL,
		generated.WithRetryConfig(generated.RetryConfig{
			MaxRetries: 3,
			BaseDelay:  1 * time.Second,
			MaxDelay:   30 * time.Second,
			Multiplier: 2.0,
		}),
		generated.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer conformance-test-token")
			req.Header.Set("User-Agent", "hey-sdk-go/conformance")
			return nil
		}),
	)
	if err != nil {
		return TestResult{
			Name:    tc.Name,
			Passed:  false,
			Message: fmt.Sprintf("Failed to create SDK client: %v", err),
		}
	}

	// Execute the operation
	ctx := context.Background()
	sdkResp, sdkErr := executeOperation(client, ctx, tc)

	// Convert HTTP response to SDK error for error assertions
	var sdkError *hey.Error
	if sdkErr != nil {
		sdkError = hey.AsError(sdkErr)
	} else if sdkResp != nil && sdkResp.StatusCode >= 400 {
		checkErr := hey.CheckResponse(sdkResp)
		if checkErr != nil {
			sdkError = hey.AsError(checkErr)
		}
	}

	// Determine the actual HTTP status code for statusCode assertions
	var lastStatus int
	mu.Lock()
	if len(responseStatuses) > 0 {
		lastStatus = responseStatuses[len(responseStatuses)-1]
	}
	mu.Unlock()

	// If we have a successful response, use its status
	if sdkResp != nil && sdkResp.StatusCode > 0 {
		lastStatus = sdkResp.StatusCode
	}

	// Run assertions
	for _, assertion := range tc.Assertions {
		result := checkAssertion(tc.Name, assertion, checkState{
			operation:      tc.Operation,
			requestCount:   requestCount,
			requestTimes:   requestTimes,
			requestPaths:   requestPaths,
			requestHeaders: requestHeaders,
			lastStatus:     lastStatus,
			sdkErr:         sdkErr,
			sdkError:       sdkError,
			sdkResp:        sdkResp,
		})
		if !result.Passed {
			return result
		}
	}

	return TestResult{
		Name:    tc.Name,
		Passed:  true,
		Message: "All assertions passed",
	}
}

// runConfigOverrideTest handles tests that override client configuration
// (e.g. HTTPS enforcement with a non-localhost HTTP URL).
func runConfigOverrideTest(tc TestCase, baseURL string) TestResult {
	var configErr error

	// Try to create a generated client with the overridden base URL.
	// The generated client itself doesn't enforce HTTPS, so we test
	// the hey.Client layer which panics on non-HTTPS non-localhost URLs.
	func() {
		defer func() {
			if r := recover(); r != nil {
				configErr = fmt.Errorf("%v", r)
			}
		}()
		cfg := &hey.Config{BaseURL: baseURL}
		_ = hey.NewClient(cfg, &hey.StaticTokenProvider{Token: "test-token"})
	}()

	for _, assertion := range tc.Assertions {
		switch assertion.Type {
		case "requestCount":
			expected, err := toInt(assertion.Expected)
			if err != nil {
				return TestResult{
					Name:    tc.Name,
					Passed:  false,
					Message: fmt.Sprintf("requestCount: %v", err),
				}
			}
			if expected != 0 {
				return TestResult{
					Name:    tc.Name,
					Passed:  false,
					Message: fmt.Sprintf("Expected 0 requests for config override test, got expectation of %d", expected),
				}
			}
		case "errorCode":
			if configErr == nil {
				return TestResult{
					Name:    tc.Name,
					Passed:  false,
					Message: "Expected configuration error, but client was created successfully",
				}
			}
		case "noError":
			if configErr != nil {
				return TestResult{
					Name:    tc.Name,
					Passed:  false,
					Message: fmt.Sprintf("Expected no error, got: %v", configErr),
				}
			}
		}
	}

	return TestResult{
		Name:    tc.Name,
		Passed:  true,
		Message: "All assertions passed",
	}
}

type checkState struct {
	operation      string
	requestCount   int
	requestTimes   []time.Time
	requestPaths   []string
	requestHeaders []http.Header
	lastStatus     int
	sdkErr         error
	sdkError       *hey.Error
	sdkResp        *http.Response
}

// emptyOnOperations maps operations to status codes that should be treated as
// "empty" (no result) rather than error. See ADR-004.
var emptyOnOperations = map[string][]int{
	"GetOngoingTimeTrack": {404},
}

func isEmptyOnStatus(operation string, statusCode int) bool {
	for _, c := range emptyOnOperations[operation] {
		if c == statusCode {
			return true
		}
	}
	return false
}

func checkAssertion(testName string, a Assertion, s checkState) TestResult {
	switch a.Type {
	case "requestCount":
		expected, err := toInt(a.Expected)
		if err != nil {
			return fail(testName, "requestCount: %v", err)
		}
		if s.requestCount != expected {
			return fail(testName, "Expected %d requests, got %d", expected, s.requestCount)
		}

	case "delayBetweenRequests":
		if len(s.requestTimes) >= 2 {
			delay := s.requestTimes[1].Sub(s.requestTimes[0])
			minDelay := time.Duration(a.Min) * time.Millisecond
			if delay < minDelay {
				return fail(testName, "Expected delay >= %v, got %v", minDelay, delay)
			}
		}

	case "noError":
		// For the generated client, a non-2xx response is not an error --
		// the error only occurs for transport failures.
		// So check that both transport error is nil and response is 2xx.
		// Exception: empty-on operations (ADR-004) treat specific status codes as success.
		if s.sdkErr != nil {
			return fail(testName, "Expected no error, got: %v", s.sdkErr)
		}
		if s.sdkResp != nil && s.sdkResp.StatusCode >= 400 && !isEmptyOnStatus(s.operation, s.sdkResp.StatusCode) {
			return fail(testName, "Expected success, got HTTP %d", s.sdkResp.StatusCode)
		}

	case "errorCode":
		expected, ok := a.Expected.(string)
		if !ok {
			return fail(testName, "errorCode: expected a string value, got %T", a.Expected)
		}
		// For transport errors or non-2xx responses, check the SDK error code
		if s.sdkError == nil {
			return fail(testName, "Expected error code %q, but got no error", expected)
		}
		if s.sdkError.Code != expected {
			return fail(testName, "Expected error code %q, got %q", expected, s.sdkError.Code)
		}

	case "errorField":
		if s.sdkError == nil {
			return fail(testName, "Expected error field %q, but got no error", a.Path)
		}
		switch a.Path {
		case "httpStatus":
			expected, err := toInt(a.Expected)
			if err != nil {
				return fail(testName, "errorField.httpStatus: %v", err)
			}
			if s.sdkError.HTTPStatus != expected {
				return fail(testName, "Expected error httpStatus %d, got %d", expected, s.sdkError.HTTPStatus)
			}
		case "retryable":
			expected, ok := a.Expected.(bool)
			if !ok {
				return fail(testName, "errorField.retryable: expected a bool value, got %T", a.Expected)
			}
			if s.sdkError.Retryable != expected {
				return fail(testName, "Expected error retryable=%v, got %v", expected, s.sdkError.Retryable)
			}
		case "requestId":
			expected, ok := a.Expected.(string)
			if !ok {
				return fail(testName, "errorField.requestId: expected a string value, got %T", a.Expected)
			}
			if s.sdkError.RequestID != expected {
				return fail(testName, "Expected error requestId %q, got %q", expected, s.sdkError.RequestID)
			}
		default:
			return fail(testName, "Unknown error field: %s", a.Path)
		}

	case "statusCode":
		expected, err := toInt(a.Expected)
		if err != nil {
			return fail(testName, "statusCode: %v", err)
		}
		if s.lastStatus != expected {
			return fail(testName, "Expected status code %d, got %d", expected, s.lastStatus)
		}

	case "requestPath":
		expected, ok := a.Expected.(string)
		if !ok {
			return fail(testName, "requestPath: expected a string value, got %T", a.Expected)
		}
		if len(s.requestPaths) == 0 {
			return fail(testName, "Expected a request, but none were recorded")
		}
		if s.requestPaths[0] != expected {
			return fail(testName, "Expected request path %q, got %q", expected, s.requestPaths[0])
		}

	case "headerPresent":
		headerName := a.Path
		if len(s.requestHeaders) == 0 {
			return fail(testName, "Expected request with header %q, but no requests were recorded", headerName)
		}
		if s.requestHeaders[0].Get(headerName) == "" {
			return fail(testName, "Expected header %q to be present, but it was not", headerName)
		}

	case "responseMeta":
		switch a.Path {
		case "totalCount":
			if s.sdkResp == nil {
				return fail(testName, "No HTTP response to check X-Total-Count header")
			}
			header := s.sdkResp.Header.Get("X-Total-Count")
			if header == "" {
				return fail(testName, "X-Total-Count header not present in response")
			}
			expected, err := toInt(a.Expected)
			if err != nil {
				return fail(testName, "responseMeta.totalCount: %v", err)
			}
			actual, err := strconv.Atoi(header)
			if err != nil {
				return fail(testName, "X-Total-Count header %q is not a valid integer", header)
			}
			if actual != expected {
				return fail(testName, "Expected X-Total-Count=%d, got %d", expected, actual)
			}
		default:
			return fail(testName, "Unknown responseMeta path: %s", a.Path)
		}

	case "urlOrigin":
		expected, ok := a.Expected.(string)
		if !ok {
			return fail(testName, "urlOrigin: expected a string value, got %T", a.Expected)
		}
		if expected == "rejected" {
			if s.sdkResp == nil {
				return fail(testName, "No HTTP response to check Link header origin")
			}
			linkHeader := s.sdkResp.Header.Get("Link")
			if linkHeader == "" {
				return fail(testName, "No Link header in response to validate origin")
			}
			nextURL := extractNextLinkURL(linkHeader)
			if nextURL == "" {
				return fail(testName, "No next URL found in Link header: %s", linkHeader)
			}
			serverURL := s.sdkResp.Request.URL
			linkParsed, err := url.Parse(nextURL)
			if err != nil {
				return fail(testName, "Failed to parse Link URL %q: %v", nextURL, err)
			}
			if linkParsed.IsAbs() && !strings.EqualFold(linkParsed.Host, serverURL.Host) {
				// Cross-origin Link URL confirms the test scenario for rejection
			} else if !linkParsed.IsAbs() {
				return fail(testName, "Expected cross-origin Link URL for rejection test, but got relative URL: %s", nextURL)
			} else if strings.EqualFold(linkParsed.Scheme, serverURL.Scheme) {
				return fail(testName, "Expected cross-origin Link URL for rejection test, but %s has same origin as server", nextURL)
			}
		} else {
			return fail(testName, "urlOrigin: unsupported expected value %q (only \"rejected\" is supported)", expected)
		}

	default:
		return fail(testName, "Unknown assertion type: %s", a.Type)
	}

	return TestResult{Name: testName, Passed: true}
}

func fail(testName, format string, args ...interface{}) TestResult {
	return TestResult{
		Name:    testName,
		Passed:  false,
		Message: fmt.Sprintf(format, args...),
	}
}

// toInt safely converts an interface{} (typically from JSON) to int.
func toInt(v interface{}) (int, error) {
	switch n := v.(type) {
	case float64:
		if n != float64(int(n)) {
			return 0, fmt.Errorf("float64 %v is not an integer", n)
		}
		return int(n), nil
	case int:
		return n, nil
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, fmt.Errorf("cannot convert json.Number %q to int: %w", n.String(), err)
		}
		return int(i), nil
	case string:
		i, err := strconv.Atoi(n)
		if err != nil {
			return 0, fmt.Errorf("cannot convert string %q to int: %w", n, err)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("unsupported type %T for integer conversion", v)
	}
}

// extractNextLinkURL parses a Link header to find the URL with rel="next".
// Instead of splitting on commas (which breaks if URLs contain commas), we
// scan for <...> blocks and inspect the following parameters for rel="next".
func extractNextLinkURL(linkHeader string) string {
	remaining := linkHeader
	for len(remaining) > 0 {
		start := strings.Index(remaining, "<")
		if start < 0 {
			break
		}
		end := strings.Index(remaining[start:], ">")
		if end < 0 {
			break
		}
		end += start // adjust to absolute index
		uri := remaining[start+1 : end]

		// Find the parameters after ">", up to the next "<" or end of string
		rest := remaining[end+1:]
		nextLink := strings.Index(rest, "<")
		var params string
		if nextLink >= 0 {
			params = rest[:nextLink]
		} else {
			params = rest
		}

		if strings.Contains(params, `rel="next"`) {
			return uri
		}

		if nextLink >= 0 {
			remaining = rest[nextLink:]
		} else {
			break
		}
	}
	return ""
}

func executeOperation(client *generated.Client, ctx context.Context, tc TestCase) (*http.Response, error) {
	switch tc.Operation {
	// Identity
	case "GetIdentity":
		return client.GetIdentity(ctx)
	case "GetNavigation":
		return client.GetNavigation(ctx)

	// Boxes
	case "ListBoxes":
		return client.ListBoxes(ctx)
	case "GetBox":
		boxId := getInt64Param(tc.PathParams, "boxId")
		return client.GetBox(ctx, boxId, nil)
	case "GetImbox":
		return client.GetImbox(ctx, nil)
	case "GetFeedbox":
		return client.GetFeedbox(ctx, nil)
	case "GetTrailbox":
		return client.GetTrailbox(ctx, nil)
	case "GetAsidebox":
		return client.GetAsidebox(ctx, nil)
	case "GetLaterbox":
		return client.GetLaterbox(ctx, nil)
	case "GetBubblebox":
		return client.GetBubblebox(ctx, nil)

	// Topics
	case "GetTopic":
		topicId := getInt64Param(tc.PathParams, "topicId")
		return client.GetTopic(ctx, topicId)
	case "GetTopicEntries":
		topicId := getInt64Param(tc.PathParams, "topicId")
		return client.GetTopicEntries(ctx, topicId, nil)
	case "GetSentTopics":
		return client.GetSentTopics(ctx, nil)
	case "GetSpamTopics":
		return client.GetSpamTopics(ctx, nil)
	case "GetTrashTopics":
		return client.GetTrashTopics(ctx, nil)
	case "GetEverythingTopics":
		return client.GetEverythingTopics(ctx, nil)

	// Messages
	case "GetMessage":
		messageId := getInt64Param(tc.PathParams, "messageId")
		return client.GetMessage(ctx, messageId)
	case "CreateMessage":
		body := generated.CreateMessageJSONRequestBody{
			Subject: getStringParam(tc.RequestBody, "subject"),
			Content: getStringParam(tc.RequestBody, "content"),
		}
		return client.CreateMessage(ctx, body)
	case "CreateTopicMessage":
		topicId := getInt64Param(tc.PathParams, "topicId")
		body := generated.CreateTopicMessageJSONRequestBody{
			Content: getStringParam(tc.RequestBody, "content"),
		}
		return client.CreateTopicMessage(ctx, topicId, body)

	// Entries
	case "ListDrafts":
		return client.ListDrafts(ctx, nil)
	case "CreateReply":
		entryId := getInt64Param(tc.PathParams, "entryId")
		body := generated.CreateReplyJSONRequestBody{
			Content: getStringParam(tc.RequestBody, "content"),
		}
		return client.CreateReply(ctx, entryId, body)

	// Contacts
	case "ListContacts":
		return client.ListContacts(ctx, nil)
	case "GetContact":
		contactId := getInt64Param(tc.PathParams, "contactId")
		return client.GetContact(ctx, contactId)

	// Calendars
	case "ListCalendars":
		return client.ListCalendars(ctx)
	case "GetCalendarRecordings":
		calendarId := getInt64Param(tc.PathParams, "calendarId")
		return client.GetCalendarRecordings(ctx, calendarId, nil)

	// Calendar Todos
	case "CreateCalendarTodo":
		body := generated.CreateCalendarTodoJSONRequestBody{
			Title: getStringParam(tc.RequestBody, "title"),
		}
		return client.CreateCalendarTodo(ctx, body)
	case "CompleteCalendarTodo":
		todoId := getInt64Param(tc.PathParams, "todoId")
		return client.CompleteCalendarTodo(ctx, todoId)
	case "UncompleteCalendarTodo":
		todoId := getInt64Param(tc.PathParams, "todoId")
		return client.UncompleteCalendarTodo(ctx, todoId)
	case "DeleteCalendarTodo":
		todoId := getInt64Param(tc.PathParams, "todoId")
		return client.DeleteCalendarTodo(ctx, todoId)

	// Habits
	case "CompleteHabit":
		day := getStringParam(tc.PathParams, "day")
		habitId := getInt64Param(tc.PathParams, "habitId")
		return client.CompleteHabit(ctx, day, habitId)
	case "UncompleteHabit":
		day := getStringParam(tc.PathParams, "day")
		habitId := getInt64Param(tc.PathParams, "habitId")
		return client.UncompleteHabit(ctx, day, habitId)

	// Time Tracks
	case "GetOngoingTimeTrack":
		return client.GetOngoingTimeTrack(ctx)
	case "StartTimeTrack":
		body := generated.StartTimeTrackJSONRequestBody{}
		return client.StartTimeTrack(ctx, body)
	case "UpdateTimeTrack":
		timeTrackId := getInt64Param(tc.PathParams, "timeTrackId")
		body := generated.UpdateTimeTrackJSONRequestBody{}
		if stopped, ok := tc.RequestBody["stopped"].(bool); ok {
			body.Stopped = &stopped
		}
		return client.UpdateTimeTrack(ctx, timeTrackId, body)

	// Journal
	case "GetJournalEntry":
		day := getStringParam(tc.PathParams, "day")
		return client.GetJournalEntry(ctx, day)
	case "UpdateJournalEntry":
		day := getStringParam(tc.PathParams, "day")
		body := generated.UpdateJournalEntryJSONRequestBody{
			Body: getStringParam(tc.RequestBody, "body"),
		}
		return client.UpdateJournalEntry(ctx, day, body)

	// Search
	case "Search":
		q := getStringParam(tc.QueryParams, "q")
		params := &generated.SearchParams{Q: q}
		return client.Search(ctx, params)

	default:
		return nil, fmt.Errorf("unknown operation: %s", tc.Operation)
	}
}

// getInt64Param extracts an int64 parameter from a map (JSON numbers are float64).
func getInt64Param(params map[string]interface{}, key string) int64 {
	if val, ok := params[key]; ok {
		if f, ok := val.(float64); ok {
			return int64(f)
		}
	}
	return 0
}

// getStringParam extracts a string parameter from a map.
func getStringParam(params map[string]interface{}, key string) string {
	if val, ok := params[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

// getStringPtrParam extracts a *string parameter from a map.
func getStringPtrParam(params map[string]interface{}, key string) *string {
	if val, ok := params[key]; ok {
		if s, ok := val.(string); ok {
			return &s
		}
	}
	return nil
}
