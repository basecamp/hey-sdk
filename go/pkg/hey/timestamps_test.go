package hey

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// A time track that is still running has no end, and a todo that is not done has no
// completion. Marshalling one of those back out must not invent either: `omitempty` does
// not omit a zero time.Time, so without `omitzero` on the generated timestamps an ongoing
// track reports "ends_at":"0001-01-01T00:00:00Z" and reads as finished in year one.
func TestAZeroTimestampIsNotMarshalled(t *testing.T) {
	running := generated.Recording{
		Id:       12,
		Title:    "Reviewing the pagination fix",
		StartsAt: time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC),
	}

	encoded, err := json.Marshal(running)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, absent := range []string{"ends_at", "stopped_at", "completed_at"} {
		if strings.Contains(string(encoded), absent) {
			t.Errorf("expected %q to be left out of a running time track, got %s", absent, encoded)
		}
	}
	if !strings.Contains(string(encoded), `"starts_at":"2026-08-21T09:00:00Z"`) {
		t.Errorf("expected the timestamp that was set to survive, got %s", encoded)
	}
}

// The counterpart: a timestamp that carries a real time is still marshalled, so omitzero
// omits nothing anyone set.
func TestATimestampThatWasSetIsMarshalled(t *testing.T) {
	finished := generated.Recording{
		Id:        12,
		StartsAt:  time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC),
		EndsAt:    time.Date(2026, 8, 21, 11, 30, 0, 0, time.UTC),
		StoppedAt: time.Date(2026, 8, 21, 11, 30, 0, 0, time.UTC),
	}

	encoded, err := json.Marshal(finished)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(string(encoded), `"ends_at":"2026-08-21T11:30:00Z"`) {
		t.Errorf("expected the end of a finished track, got %s", encoded)
	}
	if !strings.Contains(string(encoded), `"stopped_at":"2026-08-21T11:30:00Z"`) {
		t.Errorf("expected the stop of a finished track, got %s", encoded)
	}
}
