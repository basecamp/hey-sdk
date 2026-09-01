package hey

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
)

// cachingDoer sends the generated client's requests through the response cache the way
// singleRequest does for hand-written ones: a GET whose URL and credentials have a cached
// body goes out conditional on its ETag, a 304 is answered from the cache, and a fresh 200
// bearing an ETag replaces what was stored. Only JSON API reads participate: form and
// document representations pass through untouched, as does any request whose context
// bypasses the cache and any request without an Authorization header to key by.
type cachingDoer struct {
	client *Client
}

func (d *cachingDoer) Do(req *http.Request) (*http.Response, error) {
	c := d.client
	authorization := req.Header.Get("Authorization")
	if req.Method != http.MethodGet || authorization == "" ||
		req.Header.Get("Accept") != "application/json" || noCacheFromContext(req.Context()) {
		return c.httpClient.Do(req)
	}

	// The request goes out conditional only when the cache can answer the 304 it may
	// draw: an ETag without its body, or a body the current buffer bound would refuse,
	// is no better than a miss.
	key := c.cache.Key(req.URL.String(), authorization)
	var cached []byte
	if etag := c.cache.GetETag(key); etag != "" {
		if body := c.cache.GetBody(key); body != nil && int64(len(body)) <= c.bufferBound(req) {
			cached = body
			req.Header.Set("If-None-Match", etag)
			c.logger.Debug("cache conditional request", "etag", etag)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return resp, err
	}

	switch {
	case resp.StatusCode == http.StatusNotModified && cached != nil:
		c.logger.Debug("cache hit", "status", 304)
		_ = resp.Body.Close()
		return cachedResponse(req, resp, cached), nil
	case resp.StatusCode == http.StatusOK:
		if etag := resp.Header.Get("ETag"); etag != "" {
			d.store(key, etag, resp)
		}
	}
	return resp, nil
}

// store reads the response body to cache it and hands the response back a fresh reader. A
// read failure is kept for the caller's own read to surface: the round trip succeeded, and
// an error returned here would send the generated retry loop after a response it already
// has — a body that broke the limit would be refetched at full size once per attempt.
func (d *cachingDoer) store(key, etag string, resp *http.Response) {
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		resp.Body = io.NopCloser(&errorReader{err: err})
		return
	}
	_ = d.client.cache.Set(key, body, etag)
	d.client.logger.Debug("cache stored", "etag", etag)
	resp.Body = io.NopCloser(bytes.NewReader(body))
}

// cachedResponse answers a 304 with the body stored from the 200 it revalidated. The
// generated parsers choose a shape by status code and Content-Type, and a 304 carries
// neither for the representation it confirms, so the response reports the JSON the
// cache holds.
func cachedResponse(req *http.Request, notModified *http.Response, body []byte) *http.Response {
	header := notModified.Header.Clone()
	header.Set("Content-Type", "application/json")
	header.Set("Content-Length", strconv.Itoa(len(body)))
	return &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Proto:         notModified.Proto,
		ProtoMajor:    notModified.ProtoMajor,
		ProtoMinor:    notModified.ProtoMinor,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}

// errorReader delivers a body read failure to the reader the response is parsed with.
type errorReader struct{ err error }

func (r *errorReader) Read([]byte) (int, error) { return 0, r.err }
