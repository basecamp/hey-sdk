package generated

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageReceivedViaPreservesAbsentContact(t *testing.T) {
	var delivery MessageReceivedVia
	if err := json.Unmarshal([]byte(`{"email_address":"david+receipts@example.com"}`), &delivery); err != nil {
		t.Fatal(err)
	}
	if delivery.Contact != nil {
		t.Fatalf("contact = %#v, want nil", delivery.Contact)
	}

	encoded, err := json.Marshal(delivery)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"contact"`) {
		t.Fatalf("absent contact was encoded: %s", encoded)
	}
}
