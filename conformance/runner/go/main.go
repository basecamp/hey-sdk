// Package main provides a conformance test runner for the HEY Go SDK.
//
// This runner reads JSON test definitions from conformance/tests/ and
// executes them against the SDK using a mock HTTP server.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
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
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var tests []TestCase
	dec := json.NewDecoder(f)
	dec.UseNumber()
	if err := dec.Decode(&tests); err != nil {
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
	var requestMethods []string
	var requestQueries []url.Values
	var requestBodies [][]byte
	var requestReadErr error
	var requestHeaders []http.Header
	var responseStatuses []int

	// Create mock server that serves responses in sequence
	responseIndex := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		requestTimes = append(requestTimes, time.Now())
		requestPaths = append(requestPaths, r.URL.Path)
		requestMethods = append(requestMethods, r.Method)
		requestQueries = append(requestQueries, r.URL.Query())
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil && requestReadErr == nil {
			requestReadErr = fmt.Errorf("reading request body: %w", readErr)
		}
		requestBodies = append(requestBodies, body)
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

	// Execute the operation. Account-scoped cases exercise the hand-written
	// client layer because account scope is an SDK behavior rather than a
	// generated Smithy operation.
	ctx := context.Background()
	var sdkResp *http.Response
	var sdkErr error
	if _, scoped := tc.ConfigOverrides["accountId"]; scoped {
		accountID := getInt64Param(tc.ConfigOverrides, "accountId")
		rootClient := hey.NewClient(
			&hey.Config{BaseURL: server.URL},
			&hey.StaticTokenProvider{Token: "conformance-test-token"},
			hey.WithMaxRetries(0),
		)
		scopedClient, accountErr := rootClient.ForAccount(ctx, accountID)
		if accountErr != nil {
			sdkErr = accountErr
		} else {
			sdkErr = executeAccountScopedOperation(scopedClient, ctx, tc)
		}
	} else {
		sdkResp, sdkErr = executeOperation(client, ctx, tc)
	}

	// Capture response body for responseBody assertions
	var responseBodyBytes []byte
	if sdkResp != nil && sdkResp.Body != nil {
		var readErr error
		responseBodyBytes, readErr = io.ReadAll(sdkResp.Body)
		if readErr != nil {
			responseBodyBytes = nil
		}
		_ = sdkResp.Body.Close()
		sdkResp.Body = io.NopCloser(bytes.NewReader(responseBodyBytes))
	}

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

	// A request body that failed to read would make body assertions lie, so fail
	// the case outright rather than assert against a partial body.
	if requestReadErr != nil {
		result := fail(tc.Name, "%v", requestReadErr)
		fmt.Printf("  FAIL: %s\n        %s\n", tc.Name, result.Message)
		return result
	}

	// Run assertions
	for _, assertion := range tc.Assertions {
		result := checkAssertion(tc.Name, assertion, checkState{
			operation:         tc.Operation,
			requestCount:      requestCount,
			requestTimes:      requestTimes,
			requestPaths:      requestPaths,
			requestMethods:    requestMethods,
			requestQueries:    requestQueries,
			requestBodies:     requestBodies,
			requestHeaders:    requestHeaders,
			lastStatus:        lastStatus,
			sdkErr:            sdkErr,
			sdkError:          sdkError,
			sdkResp:           sdkResp,
			responseBodyBytes: responseBodyBytes,
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
	operation         string
	requestCount      int
	requestTimes      []time.Time
	requestPaths      []string
	requestMethods    []string
	requestQueries    []url.Values
	requestBodies     [][]byte
	requestHeaders    []http.Header
	lastStatus        int
	sdkErr            error
	sdkError          *hey.Error
	sdkResp           *http.Response
	responseBodyBytes []byte
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

	case "lastRequestPath":
		expected, ok := a.Expected.(string)
		if !ok {
			return fail(testName, "lastRequestPath: expected a string value, got %T", a.Expected)
		}
		if len(s.requestPaths) == 0 {
			return fail(testName, "Expected a request, but none were recorded")
		}
		if got := s.requestPaths[len(s.requestPaths)-1]; got != expected {
			return fail(testName, "Expected last request path %q, got %q", expected, got)
		}

	case "requestMethod":
		expected, ok := a.Expected.(string)
		if !ok {
			return fail(testName, "requestMethod: expected a string value, got %T", a.Expected)
		}
		if len(s.requestMethods) == 0 {
			return fail(testName, "Expected a request, but none were recorded")
		}
		if s.requestMethods[0] != expected {
			return fail(testName, "Expected request method %q, got %q", expected, s.requestMethods[0])
		}

	case "requestQuery":
		// expected: {"param": "value", "absent": null} — a null value asserts the
		// parameter is NOT sent (guards optional params that must be omitted).
		expected, ok := a.Expected.(map[string]interface{})
		if !ok {
			return fail(testName, "requestQuery: expected an object, got %T", a.Expected)
		}
		if len(s.requestQueries) == 0 {
			return fail(testName, "Expected a request, but none were recorded")
		}
		q := s.requestQueries[0]
		for k, v := range expected {
			if v == nil {
				if q.Has(k) {
					return fail(testName, "Expected query param %q to be absent, got %q", k, q.Get(k))
				}
				continue
			}
			if got := q.Get(k); got != fmt.Sprint(v) {
				return fail(testName, "Expected query param %s=%v, got %q", k, v, got)
			}
		}

	case "lastRequestQuery":
		expected, ok := a.Expected.(map[string]interface{})
		if !ok {
			return fail(testName, "lastRequestQuery: expected an object, got %T", a.Expected)
		}
		if len(s.requestQueries) == 0 {
			return fail(testName, "Expected a request, but none were recorded")
		}
		q := s.requestQueries[len(s.requestQueries)-1]
		for k, v := range expected {
			if v == nil {
				if q.Has(k) {
					return fail(testName, "Expected last query param %q to be absent, got %q", k, q.Get(k))
				}
				continue
			}
			if got := q.Get(k); got != fmt.Sprint(v) {
				return fail(testName, "Expected last query param %s=%v, got %q", k, v, got)
			}
		}

	case "requestBody":
		// expected: {"json.path": value, ...}; a null value asserts the key is absent.
		// Paths are dot-separated; array elements by index (e.g. "posting_ids.0").
		expected, ok := a.Expected.(map[string]interface{})
		if !ok {
			return fail(testName, "requestBody: expected an object, got %T", a.Expected)
		}
		if len(s.requestBodies) == 0 {
			return fail(testName, "Expected a request, but none were recorded")
		}
		var body interface{}
		if len(s.requestBodies[0]) > 0 {
			if err := json.Unmarshal(s.requestBodies[0], &body); err != nil {
				return fail(testName, "requestBody: request body is not JSON: %v", err)
			}
		}
		for path, want := range expected {
			got, present := lookupJSONPath(body, path)
			if want == nil {
				if present {
					return fail(testName, "Expected body key %q to be absent, got %v", path, got)
				}
				continue
			}
			if !present {
				return fail(testName, "Expected body key %q = %v, but it is absent", path, want)
			}
			if fmt.Sprint(got) != fmt.Sprint(want) {
				return fail(testName, "Expected body key %q = %v, got %v", path, want, got)
			}
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

	case "responseBody":
		fieldPath := a.Path
		if len(s.responseBodyBytes) == 0 {
			return fail(testName, "Expected responseBody.%s, but no response body captured", fieldPath)
		}
		var resultMap map[string]interface{}
		dec := json.NewDecoder(bytes.NewReader(s.responseBodyBytes))
		dec.UseNumber()
		if err := dec.Decode(&resultMap); err != nil {
			return fail(testName, "Failed to decode response body for responseBody assertion: %v", err)
		}
		actual, ok := resultMap[fieldPath]
		if !ok {
			return fail(testName, "Expected responseBody.%s, but field not present", fieldPath)
		}
		if result := compareValues(testName, fmt.Sprintf("responseBody.%s", fieldPath), a.Expected, actual); result != nil {
			return *result
		}

	default:
		return fail(testName, "Unknown assertion type: %s", a.Type)
	}

	return TestResult{Name: testName, Passed: true}
}

// compareValues compares expected and actual values with precision-safe numeric handling.
func compareValues(testName, label string, expected, actual interface{}) *TestResult {
	switch exp := expected.(type) {
	case json.Number:
		if expInt, err := exp.Int64(); err == nil {
			switch act := actual.(type) {
			case json.Number:
				if actInt, err := act.Int64(); err == nil {
					if actInt != expInt {
						r := fail(testName, "Expected %s = %d, got %d", label, expInt, actInt)
						return &r
					}
					return nil
				}
			case int64:
				if act != expInt {
					r := fail(testName, "Expected %s = %d, got %d", label, expInt, act)
					return &r
				}
				return nil
			case float64:
				if int64(act) != expInt {
					r := fail(testName, "Expected %s = %d, got %v", label, expInt, act)
					return &r
				}
				return nil
			}
		}
		if expFloat, err := exp.Float64(); err == nil {
			switch act := actual.(type) {
			case json.Number:
				if actFloat, err := act.Float64(); err == nil {
					if actFloat != expFloat {
						r := fail(testName, "Expected %s = %v, got %v", label, expFloat, actFloat)
						return &r
					}
					return nil
				}
			case float64:
				if act != expFloat {
					r := fail(testName, "Expected %s = %v, got %v", label, expFloat, act)
					return &r
				}
				return nil
			}
		}
		if fmt.Sprintf("%v", actual) != exp.String() {
			r := fail(testName, "Expected %s = %s, got %v", label, exp.String(), actual)
			return &r
		}
	case float64:
		switch act := actual.(type) {
		case json.Number:
			if actFloat, err := act.Float64(); err == nil {
				if actFloat != exp {
					r := fail(testName, "Expected %s = %v, got %v", label, exp, actFloat)
					return &r
				}
				return nil
			}
		case float64:
			if act != exp {
				r := fail(testName, "Expected %s = %v, got %v", label, exp, act)
				return &r
			}
			return nil
		case int64:
			if float64(act) != exp {
				r := fail(testName, "Expected %s = %v, got %d", label, exp, act)
				return &r
			}
			return nil
		}
	case bool:
		if actual != exp {
			r := fail(testName, "Expected %s = %v, got %v", label, exp, actual)
			return &r
		}
	case string:
		if fmt.Sprintf("%v", actual) != exp {
			r := fail(testName, "Expected %s = %q, got %q", label, exp, actual)
			return &r
		}
	default:
		r := fail(testName, "Unsupported type combination for %s: expected %T, actual %T", label, expected, actual)
		return &r
	}
	return nil
}

func fail(testName, format string, args ...interface{}) TestResult {
	return TestResult{
		Name:    testName,
		Passed:  false,
		Message: fmt.Sprintf(format, args...),
	}
}

// lookupJSONPath walks a decoded JSON value by a dot-separated path, using
// integer segments as array indexes. It reports whether the path resolved.
func lookupJSONPath(v interface{}, path string) (interface{}, bool) {
	cur := v
	for _, seg := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]interface{}:
			next, ok := node[seg]
			if !ok {
				return nil, false
			}
			cur = next
		case []interface{}:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		default:
			return nil, false
		}
	}
	return cur, true
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
	case "GetBoxPostingChanges":
		boxId := getInt64Param(tc.PathParams, "boxId")
		params := &generated.GetBoxPostingChangesParams{Since: getStringParam(tc.QueryParams, "since")}
		if version := getStringParam(tc.QueryParams, "v"); version != "" {
			params.V = &version
		}
		if page := getStringParam(tc.QueryParams, "page"); page != "" {
			params.Page = &page
		}
		return client.GetBoxPostingChanges(ctx, boxId, params)
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
			Message: generated.MessagePayload{
				Subject: getStringParam(tc.RequestBody, "subject"),
				Content: getStringParam(tc.RequestBody, "content"),
			},
		}
		return client.CreateMessage(ctx, body)
	case "CreateDirectUpload":
		body := generated.CreateDirectUploadJSONRequestBody{
			Blob: generated.DirectUploadBlob{
				Filename:    getStringParam(tc.RequestBody, "filename"),
				ByteSize:    getInt64Param(tc.RequestBody, "byte_size"),
				Checksum:    getStringParam(tc.RequestBody, "checksum"),
				ContentType: getStringParam(tc.RequestBody, "content_type"),
			},
		}
		return client.CreateDirectUpload(ctx, body)
	case "ListDrafts":
		return client.ListDrafts(ctx, nil)
	case "CreateReply":
		entryId := getInt64Param(tc.PathParams, "entryId")
		body := generated.CreateReplyJSONRequestBody{
			Message: generated.ReplyMessagePayload{
				Content: getStringParam(tc.RequestBody, "content"),
			},
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
			CalendarTodo: generated.CalendarTodoPayload{
				Title: getStringParam(tc.RequestBody, "title"),
			},
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
		return client.StartTimeTrack(ctx)
	case "UpdateTimeTrack":
		timeTrackId := getInt64Param(tc.PathParams, "timeTrackId")
		body := generated.UpdateTimeTrackJSONRequestBody{
			CalendarTimeTrack: generated.UpdateTimeTrackPayload{},
		}
		return client.UpdateTimeTrack(ctx, timeTrackId, body)

	// Journal
	case "GetJournalEntry":
		day := getStringParam(tc.PathParams, "day")
		return client.GetJournalEntry(ctx, day)
	case "UpdateJournalEntry":
		day := getStringParam(tc.PathParams, "day")
		body := generated.UpdateJournalEntryJSONRequestBody{
			CalendarJournalEntry: generated.JournalEntryPayload{
				Content: getStringParam(tc.RequestBody, "body"),
			},
		}
		return client.UpdateJournalEntry(ctx, day, body)

	// Search
	case "GetAdvancedSearchFilters":
		return client.GetAdvancedSearchFilters(ctx)
	case "AdvancedSearch":
		params := &generated.AdvancedSearchParams{}
		if q := getStringParam(tc.QueryParams, "q"); q != "" {
			params.Q = &q
		}
		if from := getStringParam(tc.QueryParams, "refine[from]"); from != "" {
			params.RefineFrom = &from
		}
		return client.AdvancedSearch(ctx, params)

	// Reads that used to be scraped
	case "ListClips":
		return client.ListClips(ctx, nil)
	case "ListSnippets":
		return client.ListSnippets(ctx)
	case "GetWorkflow":
		return client.GetWorkflow(ctx, getInt64Param(tc.PathParams, "workflowId"))
	case "ListTimeTrackCategories":
		return client.ListTimeTrackCategories(ctx)
	case "GetTopicPublication":
		return client.GetTopicPublication(ctx, getInt64Param(tc.PathParams, "topicId"))

	// Postings
	case "MarkPostingsSeen":
		body := generated.MarkPostingsSeenJSONRequestBody{
			PostingIds: getInt64SliceParam(tc.RequestBody, "posting_ids"),
		}
		return client.MarkPostingsSeen(ctx, body)
	case "MarkPostingsUnseen":
		body := generated.MarkPostingsUnseenJSONRequestBody{
			PostingIds: getInt64SliceParam(tc.RequestBody, "posting_ids"),
		}
		return client.MarkPostingsUnseen(ctx, body)
	case "MovePostings":
		body := generated.MovePostingsJSONRequestBody{
			PostingIds: getInt64SliceParam(tc.RequestBody, "posting_ids"),
			BoxId:      getInt64Param(tc.RequestBody, "box_id"),
		}
		return client.MovePostings(ctx, body)
	case "TrashPostings":
		body := generated.TrashPostingsJSONRequestBody{
			PostingIds: getInt64SliceParam(tc.RequestBody, "posting_ids"),
		}
		return client.TrashPostings(ctx, body)
	case "MutePostings":
		body := generated.MutePostingsJSONRequestBody{
			PostingIds: getInt64SliceParam(tc.RequestBody, "posting_ids"),
		}
		return client.MutePostings(ctx, body)
	case "UnmutePostings":
		params := &generated.UnmutePostingsParams{PostingIds: getStringParam(tc.QueryParams, "posting_ids")}
		return client.UnmutePostings(ctx, params)
	case "MarkPostingsSpam":
		body := generated.MarkPostingsSpamJSONRequestBody{
			PostingIds: getInt64SliceParam(tc.RequestBody, "posting_ids"),
		}
		return client.MarkPostingsSpam(ctx, body)
	case "AddPostingsToBoxGroup":
		body := generated.AddPostingsToBoxGroupJSONRequestBody{
			PostingIds: getInt64SliceParam(tc.RequestBody, "posting_ids"),
			BoxId:      getInt64Param(tc.RequestBody, "box_id"),
			BoxGroupId: getInt64Param(tc.RequestBody, "box_group_id"),
		}
		return client.AddPostingsToBoxGroup(ctx, body)
	case "RemovePostingsFromBoxGroup":
		params := &generated.RemovePostingsFromBoxGroupParams{PostingIds: getStringParam(tc.QueryParams, "posting_ids")}
		return client.RemovePostingsFromBoxGroup(ctx, params)
	case "FilePostings":
		body := generated.FilePostingsJSONRequestBody{
			PostingIds: getInt64SliceParam(tc.RequestBody, "posting_ids"),
			FolderId:   getInt64Param(tc.RequestBody, "folder_id"),
		}
		return client.FilePostings(ctx, body)
	case "UnfilePostings":
		params := &generated.UnfilePostingsParams{
			PostingIds: getStringParam(tc.QueryParams, "posting_ids"),
		}
		if _, ok := tc.QueryParams["folder_id"]; ok {
			folderID := getInt64Param(tc.QueryParams, "folder_id")
			params.FolderId = &folderID
		}
		return client.UnfilePostings(ctx, params)
	case "CreateFolderForPostings":
		body := generated.CreateFolderForPostingsJSONRequestBody{
			PostingIds: getInt64SliceParam(tc.RequestBody, "posting_ids"),
			Folder:     generated.FolderPayload{Name: getStringParam(tc.RequestBody, "name")},
		}
		return client.CreateFolderForPostings(ctx, body)
	case "CancelPostingsBubbleUp":
		params := &generated.CancelPostingsBubbleUpParams{PostingIds: getStringParam(tc.QueryParams, "posting_ids")}
		return client.CancelPostingsBubbleUp(ctx, params)
	case "BubbleUpPostingsNow":
		body := generated.BubbleUpPostingsNowJSONRequestBody{
			PostingIds: getInt64SliceParam(tc.RequestBody, "posting_ids"),
		}
		return client.BubbleUpPostingsNow(ctx, body)

	// Topic status and moves
	case "TrashTopic":
		topicId := getInt64Param(tc.PathParams, "topicId")
		params := &generated.TrashTopicParams{}
		if _, ok := tc.QueryParams["confirm_destroy"]; ok {
			confirm := getStringParam(tc.QueryParams, "confirm_destroy")
			params.ConfirmDestroy = &confirm
		}
		return client.TrashTopic(ctx, topicId, params)
	case "RestoreTopic":
		topicId := getInt64Param(tc.PathParams, "topicId")
		return client.RestoreTopic(ctx, topicId)
	case "MarkTopicHam":
		topicId := getInt64Param(tc.PathParams, "topicId")
		return client.MarkTopicHam(ctx, topicId)
	case "EmptyTrash":
		return client.EmptyTrash(ctx)
	case "EmptySpam":
		return client.EmptySpam(ctx)
	case "MoveTopic":
		topicId := getInt64Param(tc.PathParams, "topicId")
		body := generated.MoveTopicJSONRequestBody{BoxId: getInt64Param(tc.RequestBody, "box_id")}
		return client.MoveTopic(ctx, topicId, body)

	// Entry status and forwards
	case "MarkEntrySpam":
		entryId := getInt64Param(tc.PathParams, "entryId")
		return client.MarkEntrySpam(ctx, entryId)
	case "NewEntryForward":
		entryId := getInt64Param(tc.PathParams, "entryId")
		return client.NewEntryForward(ctx, entryId)

	// Bulk reply
	case "NewBulkReply":
		return client.NewBulkReply(ctx, &generated.NewBulkReplyParams{
			PostingIds: getStringParam(tc.QueryParams, "posting_ids"),
		})
	case "CreateBulkReply":
		body := generated.CreateBulkReplyJSONRequestBody{
			EntryIds: getInt64SliceParam(tc.RequestBody, "entry_ids"),
			Message:  generated.BulkReplyMessagePayload{Content: getStringParam(tc.RequestBody, "content")},
		}
		return client.CreateBulkReply(ctx, body)

	// Contact bundles and screening
	case "BundleContact":
		contactId := getInt64Param(tc.PathParams, "contactId")
		return client.BundleContact(ctx, contactId)
	case "UnbundleContact":
		contactId := getInt64Param(tc.PathParams, "contactId")
		return client.UnbundleContact(ctx, contactId)
	case "UpdateContactClearance":
		contactId := getInt64Param(tc.PathParams, "contactId")
		body := generated.UpdateContactClearanceJSONRequestBody{Status: getStringParam(tc.RequestBody, "status")}
		return client.UpdateContactClearance(ctx, contactId, body)
	case "GetClearances":
		return client.GetClearances(ctx, &generated.GetClearancesParams{})
	case "UpdateClearance":
		clearanceId := getInt64Param(tc.PathParams, "clearanceId")
		body := generated.UpdateClearanceJSONRequestBody{Status: getStringParam(tc.RequestBody, "status")}
		return client.UpdateClearance(ctx, clearanceId, body)
	case "BulkUpdateClearances":
		body := generated.BulkUpdateClearancesJSONRequestBody{
			Ids:    getStringParam(tc.RequestBody, "ids"),
			Status: getStringParam(tc.RequestBody, "status"),
		}
		return client.BulkUpdateClearances(ctx, body)
	case "PuntClearances":
		return client.PuntClearances(ctx)
	case "GetMyClearances":
		return client.GetMyClearances(ctx, &generated.GetMyClearancesParams{})
	case "UpdateMyClearance":
		clearanceId := getInt64Param(tc.PathParams, "clearanceId")
		body := generated.UpdateMyClearanceJSONRequestBody{Status: getStringParam(tc.RequestBody, "status")}
		return client.UpdateMyClearance(ctx, clearanceId, body)

	// Contact writing and notes
	case "CreateContact":
		return client.CreateContact(ctx, generated.CreateContactRequestContent{
			ActingUserId: getInt64Param(tc.RequestBody, "acting_user_id"),
			Contact:      contactBody(tc).Contact,
		})
	case "UpdateContact":
		contactId := getInt64Param(tc.PathParams, "contactId")
		return client.UpdateContact(ctx, contactId, contactBody(tc))
	case "HideContact":
		contactId := getInt64Param(tc.PathParams, "contactId")
		return client.HideContact(ctx, contactId)
	case "RevealContact":
		contactId := getInt64Param(tc.PathParams, "contactId")
		return client.RevealContact(ctx, contactId)
	case "GetContactNote":
		contactId := getInt64Param(tc.PathParams, "contactId")
		return client.GetContactNote(ctx, contactId)
	case "UpdateContactNote":
		contactId := getInt64Param(tc.PathParams, "contactId")
		body := generated.UpdateContactNoteJSONRequestBody{
			Contact: generated.ContactNotePayload{Note: getStringParam(tc.RequestBody, "note")},
		}
		return client.UpdateContactNote(ctx, contactId, body)
	case "DeleteContactNote":
		contactId := getInt64Param(tc.PathParams, "contactId")
		return client.DeleteContactNote(ctx, contactId)

	// Box designations, groups and observation
	case "CreateBoxDesignation":
		boxId := getInt64Param(tc.PathParams, "boxId")
		body := generated.CreateBoxDesignationJSONRequestBody{ContactId: getInt64Param(tc.RequestBody, "contact_id")}
		return client.CreateBoxDesignation(ctx, boxId, body)
	case "DeleteBoxDesignation":
		boxId := getInt64Param(tc.PathParams, "boxId")
		designationId := getInt64Param(tc.PathParams, "designationId")
		return client.DeleteBoxDesignation(ctx, boxId, designationId)
	case "ListBoxGroups":
		boxId := getInt64Param(tc.PathParams, "boxId")
		return client.ListBoxGroups(ctx, boxId)
	case "CreateBoxGroup":
		boxId := getInt64Param(tc.PathParams, "boxId")
		body := generated.CreateBoxGroupJSONRequestBody{
			PostingIds: getInt64SliceParam(tc.RequestBody, "posting_ids"),
		}
		return client.CreateBoxGroup(ctx, boxId, body)
	case "DeleteBoxGroup":
		boxId := getInt64Param(tc.PathParams, "boxId")
		groupId := getInt64Param(tc.PathParams, "groupId")
		return client.DeleteBoxGroup(ctx, boxId, groupId)
	case "MarkBoxSeen":
		boxId := getInt64Param(tc.PathParams, "boxId")
		return client.MarkBoxSeen(ctx, boxId)

	// Folders
	case "GetFolder":
		folderId := getInt64Param(tc.PathParams, "folderId")
		return client.GetFolder(ctx, folderId, nil)

	// Collections
	case "ListCollections":
		return client.ListCollections(ctx)
	case "UpdateCollection":
		collectionId := getInt64Param(tc.PathParams, "collectionId")
		body := generated.UpdateCollectionJSONRequestBody{
			Collection: generated.CollectionPayload{
				Name:    getStringParam(tc.RequestBody, "name"),
				Summary: getStringParam(tc.RequestBody, "summary"),
			},
		}
		return client.UpdateCollection(ctx, collectionId, body)

	// Stickies
	case "ListStickies":
		params := &generated.ListStickiesParams{}
		if _, ok := tc.QueryParams["limit"]; ok {
			limit := getInt32Param(tc.QueryParams, "limit")
			params.Limit = &limit
		}
		return client.ListStickies(ctx, params)
	case "CreateSticky":
		return client.CreateSticky(ctx, stickyBody(tc))
	case "UpdateSticky":
		stickyId := getInt64Param(tc.PathParams, "stickyId")
		return client.UpdateSticky(ctx, stickyId, stickyBody(tc))
	case "DeleteSticky":
		stickyId := getInt64Param(tc.PathParams, "stickyId")
		return client.DeleteSticky(ctx, stickyId)
	case "MoveSticky":
		body := generated.MoveStickyJSONRequestBody{
			Id:       getInt64Param(tc.RequestBody, "id"),
			Position: getInt32Param(tc.RequestBody, "position"),
		}
		return client.MoveSticky(ctx, body)

	// Time track writes
	case "CreateTimeTrack":
		body := generated.CreateTimeTrackJSONRequestBody{
			StartsAt:      getTimeParam(tc.RequestBody, "starts_at"),
			EndsAt:        getTimeParam(tc.RequestBody, "ends_at"),
			CategoryTitle: getStringParam(tc.RequestBody, "category_title"),
			Notes:         getStringParam(tc.RequestBody, "notes"),
		}
		return client.CreateTimeTrack(ctx, body)
	case "DeleteTimeTrack":
		timeTrackId := getInt64Param(tc.PathParams, "timeTrackId")
		return client.DeleteTimeTrack(ctx, timeTrackId)

	// Habit CRUD
	case "CreateHabit":
		return client.CreateHabit(ctx, habitBody(tc))
	case "UpdateHabit":
		habitId := getInt64Param(tc.PathParams, "habitId")
		return client.UpdateHabit(ctx, habitId, habitBody(tc))
	case "DeleteHabit":
		habitId := getInt64Param(tc.PathParams, "habitId")
		return client.DeleteHabit(ctx, habitId)
	case "StopHabit":
		habitId := getInt64Param(tc.PathParams, "habitId")
		return client.StopHabit(ctx, habitId)
	case "ResumeHabit":
		habitId := getInt64Param(tc.PathParams, "habitId")
		return client.ResumeHabit(ctx, habitId)

	default:
		return nil, fmt.Errorf("unknown operation: %s", tc.Operation)
	}
}

func executeAccountScopedOperation(client *hey.Client, ctx context.Context, tc TestCase) error {
	switch tc.Operation {
	case "ListBoxes":
		_, err := client.Boxes().List(ctx)
		return err
	default:
		return fmt.Errorf("account-scoped conformance does not support operation: %s", tc.Operation)
	}
}

// getInt64Param extracts an int64 parameter from a map.
// Handles both json.Number (from UseNumber) and float64 (legacy).
func getInt64Param(params map[string]interface{}, key string) int64 {
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case json.Number:
			if i, err := v.Int64(); err == nil {
				return i
			}
		case float64:
			return int64(v)
		}
	}
	return 0
}

// getInt64SliceParam extracts a []int64 parameter from a map.
func getInt64SliceParam(params map[string]interface{}, key string) []int64 {
	val, ok := params[key]
	if !ok {
		return nil
	}
	arr, ok := val.([]interface{})
	if !ok {
		return nil
	}
	result := make([]int64, 0, len(arr))
	for _, v := range arr {
		switch n := v.(type) {
		case json.Number:
			if i, err := n.Int64(); err == nil {
				result = append(result, i)
			}
		case float64:
			result = append(result, int64(n))
		}
	}
	return result
}

// getInt32Param extracts an int32 parameter from a map, for the wire fields the API
// carries as 32-bit integers.
func getInt32Param(params map[string]interface{}, key string) int32 {
	value := getInt64Param(params, key)
	if value < math.MinInt32 || value > math.MaxInt32 {
		return 0
	}
	return int32(value)
}

// getInt32SliceParam extracts a []int32 parameter from a map.
func getInt32SliceParam(params map[string]interface{}, key string) []int32 {
	values := getInt64SliceParam(params, key)
	if values == nil {
		return nil
	}
	result := make([]int32, 0, len(values))
	for _, v := range values {
		if v < math.MinInt32 || v > math.MaxInt32 {
			continue
		}
		result = append(result, int32(v))
	}
	return result
}

// getTimeParam extracts an RFC 3339 timestamp from a map.
func getTimeParam(params map[string]interface{}, key string) time.Time {
	parsed, err := time.Parse(time.RFC3339, getStringParam(params, key))
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// stickyBody builds the {sticky: {body, size}} wire payload the stickies writes share.
func stickyBody(tc TestCase) generated.StickyRequestContent {
	return generated.StickyRequestContent{
		Sticky: generated.StickyPayload{
			Body: getStringParam(tc.RequestBody, "body"),
			Size: getStringParam(tc.RequestBody, "size"),
		},
	}
}

// habitBody builds the {calendar_habit: {...}} wire payload the habit writes share.
func habitBody(tc TestCase) generated.HabitRequestContent {
	return generated.HabitRequestContent{
		CalendarHabit: generated.HabitPayload{
			Name:  getStringParam(tc.RequestBody, "name"),
			Icon:  getStringParam(tc.RequestBody, "icon"),
			Color: getStringParam(tc.RequestBody, "color"),
			Days:  getInt32SliceParam(tc.RequestBody, "days"),
		},
	}
}

func contactBody(tc TestCase) generated.ContactRequestContent {
	return generated.ContactRequestContent{
		Contact: generated.ContactPayload{
			Name:                getStringParam(tc.RequestBody, "name"),
			EmailAddress:        getStringParam(tc.RequestBody, "email_address"),
			AliasEmailAddresses: getStringSliceParam(tc.RequestBody, "alias_email_addresses"),
		},
	}
}

// getStringSliceParam extracts a []string parameter from a map.
func getStringSliceParam(params map[string]interface{}, key string) []string {
	val, ok := params[key]
	if !ok {
		return nil
	}
	arr, ok := val.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
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
