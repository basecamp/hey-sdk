package main

import (
	"errors"
	"io"
	"testing"
)

func TestReadRequestBodyPropagatesPartialReadError(t *testing.T) {
	sentinel := errors.New("request body read failed")
	reader := &partialErrorReadCloser{
		contents: []byte("status=drafted"),
		err:      sentinel,
	}

	body, err := readRequestBody(reader)
	if !errors.Is(err, sentinel) {
		t.Fatalf("readRequestBody() error = %v, want %v", err, sentinel)
	}
	if got, want := string(body), "status=drafted"; got != want {
		t.Fatalf("readRequestBody() body = %q, want %q", got, want)
	}
	if !reader.closed {
		t.Fatal("readRequestBody() did not close the request body")
	}
}

func TestReadRequestBodyPropagatesCloseError(t *testing.T) {
	sentinel := errors.New("request body close failed")
	reader := &partialErrorReadCloser{
		contents: []byte("status=drafted"),
		closeErr: sentinel,
	}

	body, err := readRequestBody(reader)
	if !errors.Is(err, sentinel) {
		t.Fatalf("readRequestBody() error = %v, want %v", err, sentinel)
	}
	if got, want := string(body), "status=drafted"; got != want {
		t.Fatalf("readRequestBody() body = %q, want %q", got, want)
	}
}

type partialErrorReadCloser struct {
	contents []byte
	err      error
	closeErr error
	read     bool
	closed   bool
}

func (r *partialErrorReadCloser) Read(p []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	r.read = true
	return copy(p, r.contents), r.err
}

func (r *partialErrorReadCloser) Close() error {
	r.closed = true
	return r.closeErr
}
