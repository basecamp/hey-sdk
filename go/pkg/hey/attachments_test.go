package hey

import (
	"context"
	"crypto/md5" //nolint:gosec // Active Storage test checksum.
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

func TestAttachmentsUpload(t *testing.T) {
	const contents = "quarterly report contents"
	checksumBytes := md5.Sum([]byte(contents)) //nolint:gosec // Active Storage requires MD5.
	checksum := base64.StdEncoding.EncodeToString(checksumBytes[:])

	uploadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("storage Authorization = %q, want empty", got)
		}
		if got := r.Header.Get("Content-MD5"); got != checksum {
			t.Errorf("Content-MD5 = %q, want %q", got, checksum)
		}
		if got := r.Header.Get("Content-Type"); got != "application/pdf" {
			t.Errorf("Content-Type = %q", got)
		}
		if r.ContentLength != int64(len(contents)) {
			t.Errorf("Content-Length = %d, want %d", r.ContentLength, len(contents))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != contents {
			t.Errorf("body = %q", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(uploadServer.Close)

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body generated.CreateDirectUploadRequestContent
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Blob.Filename != "quarterly-report.pdf" || body.Blob.ByteSize != int64(len(contents)) || body.Blob.Checksum != checksum || body.Blob.ContentType != "application/pdf" {
			t.Errorf("blob = %+v", body.Blob)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"signed_id":"signed-123",
			"attachable_sgid":"sgid-456",
			"direct_upload":{
				"url":"` + uploadServer.URL + `/blob",
				"headers":{"Content-Type":"application/pdf","Content-MD5":"` + checksum + `"}
			}
		}`))
	})

	upload, err := client.Attachments().Upload(context.Background(), "quarterly-report.pdf", "application/pdf", strings.NewReader(contents))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if upload.AttachableSgid != "sgid-456" {
		t.Errorf("attachable SGID = %q", upload.AttachableSgid)
	}
}

func TestAttachmentsUploadRejectsInsecureStorageTarget(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"signed_id":"signed-123",
			"attachable_sgid":"sgid-456",
			"direct_upload":{"url":"http://uploads.example.com/blob","headers":{}}
		}`))
	})

	_, err := client.Attachments().Upload(context.Background(), "report.pdf", "application/pdf", strings.NewReader("report"))
	if err == nil || !strings.Contains(err.Error(), "unsafe attachment upload target") {
		t.Fatalf("Upload error = %v", err)
	}
}

func TestAttachmentsCreateDirectUpload(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/rails/active_storage/direct_uploads.json" {
			t.Errorf("path = %s, want /rails/active_storage/direct_uploads.json", r.URL.Path)
		}

		var body generated.CreateDirectUploadRequestContent
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Blob.Filename != "quarterly-report.pdf" {
			t.Errorf("filename = %q", body.Blob.Filename)
		}
		if body.Blob.ByteSize != 128 {
			t.Errorf("byte size = %d", body.Blob.ByteSize)
		}
		if body.Blob.Checksum != "YWJjZA==" {
			t.Errorf("checksum = %q", body.Blob.Checksum)
		}
		if body.Blob.ContentType != "application/pdf" {
			t.Errorf("content type = %q", body.Blob.ContentType)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"signed_id":"signed-123",
			"attachable_sgid":"sgid-456",
			"direct_upload":{
				"url":"https://uploads.example.com/blob",
				"headers":{"Content-Type":"application/pdf","Content-MD5":"YWJjZA=="}
			}
		}`))
	})

	result, err := client.Attachments().CreateDirectUpload(context.Background(), generated.CreateDirectUploadRequestContent{
		Blob: generated.DirectUploadBlob{
			Filename:    "quarterly-report.pdf",
			ByteSize:    128,
			Checksum:    "YWJjZA==",
			ContentType: "application/pdf",
		},
	})
	if err != nil {
		t.Fatalf("CreateDirectUpload: %v", err)
	}
	if result.SignedId != "signed-123" {
		t.Errorf("signed ID = %q", result.SignedId)
	}
	if result.AttachableSgid != "sgid-456" {
		t.Errorf("attachable SGID = %q", result.AttachableSgid)
	}
	if result.DirectUpload.Url != "https://uploads.example.com/blob" {
		t.Errorf("upload URL = %q", result.DirectUpload.Url)
	}
	if result.DirectUpload.Headers["Content-MD5"] != "YWJjZA==" {
		t.Errorf("upload headers = %v", result.DirectUpload.Headers)
	}
}
