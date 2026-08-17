package hey

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

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
