package generated

import (
	"encoding/json"
	"testing"
)

func TestRecordingCategoryPreservesNullableWireValues(t *testing.T) {
	var recording Recording
	if err := json.Unmarshal([]byte(`{"category":"Client work"}`), &recording); err != nil {
		t.Fatal(err)
	}
	if recording.Category == nil || *recording.Category != "Client work" {
		t.Fatalf("category = %v, want Client work", recording.Category)
	}
	if err := json.Unmarshal([]byte(`{}`), &recording); err != nil {
		t.Fatal(err)
	}
	if recording.Category == nil || *recording.Category != "Client work" {
		t.Fatalf("an omitted category changed the existing value: %v", recording.Category)
	}
	if err := json.Unmarshal([]byte(`{"category":null}`), &recording); err != nil {
		t.Fatal(err)
	}
	if recording.Category != nil {
		t.Fatalf("category = %q, want nil", *recording.Category)
	}
	if err := json.Unmarshal([]byte(`{"category":""}`), &recording); err != nil {
		t.Fatal(err)
	}
	if recording.Category == nil || *recording.Category != "" {
		t.Fatalf("category = %v, want a nonnil empty string", recording.Category)
	}
}
