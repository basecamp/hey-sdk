package hey

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/basecamp/hey-sdk/go/pkg/generated"
)

// A `[]` HEY sends has to reach the caller as a non-nil empty slice and marshal back as
// `[]`: a caller that re-serializes a response (a CLI's --json, an MCP tool result) must
// not see the key vanish or turn into null. An absent key stays absent.

// requestSideSlices are the generated request-body types whose optional slices keep
// omitempty (scripts/merge-behavior-model leaves request-side schemas alone): an empty
// slice there is the caller's own value, and callers rely on it leaving the key out.
var requestSideSlices = map[string]bool{"ContactPayload": true, "HabitPayload": true, "MessageAddressed": true}

// TestGeneratedSlicesKeepEmptyLists holds the shape across every generated response
// struct: an optional slice is tagged omitzero, which drops only a nil slice, and never
// omitempty, which drops an empty one too. scripts/merge-behavior-model marks the
// properties so. A new request-side type with a slice fails here until it is named in
// requestSideSlices.
func TestGeneratedSlicesKeepEmptyLists(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "../generated/client.gen.go", nil, 0)
	if err != nil {
		t.Fatalf("parse generated client: %v", err)
	}

	var slices, omitzero int
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := spec.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, field := range st.Fields.List {
			if _, isSlice := field.Type.(*ast.ArrayType); !isSlice || field.Tag == nil {
				continue
			}
			tag, _ := strconv.Unquote(field.Tag.Value)
			jsonTag := reflect.StructTag(tag).Get("json")
			slices++
			if strings.Contains(jsonTag, "omitzero") {
				omitzero++
			}
			if strings.Contains(jsonTag, "omitempty") && !requestSideSlices[spec.Name.Name] {
				t.Errorf("%s: %s.%s is tagged %q; omitempty drops a [] HEY sent — want omitzero",
					fset.Position(field.Pos()), spec.Name.Name, field.Names[0].Name, jsonTag)
			}
		}
		return true
	})
	if slices == 0 || omitzero == 0 {
		t.Fatalf("found %d slice fields, %d omitzero: the scan saw nothing to check", slices, omitzero)
	}
}

// TestGeneratedTypesRoundTripEmptyLists checks what the tag means at runtime, on types
// the service methods answer with: `[]` decodes non-nil and marshals back as `[]`, and an
// absent key stays absent.
func TestGeneratedTypesRoundTripEmptyLists(t *testing.T) {
	cases := []struct {
		name string
		into any
		wire string
		keys []string
	}{
		{"Topic", &generated.Topic{}, `{"id":1,"entries":[],"contacts":[],"collections":[],"extenzions":[]}`,
			[]string{"entries", "contacts", "collections", "extenzions"}},
		{"Posting", &generated.Posting{}, `{"id":1,"addressed_contacts":[],"contacts":[],"folders":[],"workflows":[]}`,
			[]string{"addressed_contacts", "contacts", "folders", "workflows"}},
		{"BoxShowResponse", &generated.BoxShowResponse{}, `{"id":1,"postings":[]}`, []string{"postings"}},
		{"GetBoxPostingChangesResponseContent", &generated.GetBoxPostingChangesResponseContent{},
			`{"added":[],"updated":[],"deleted":[]}`, []string{"added", "updated", "deleted"}},
		{"Identity", &generated.Identity{}, `{"id":1,"accounts":[],"senders":[]}`, []string{"accounts", "senders"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tc.wire), tc.into); err != nil {
				t.Fatalf("decode: %v", err)
			}
			out, err := json.Marshal(tc.into)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			for _, key := range tc.keys {
				if !strings.Contains(string(out), `"`+key+`":[]`) {
					t.Errorf("%s: %q lost its []: %s", tc.name, key, out)
				}
			}
		})
	}

	t.Run("absent stays absent", func(t *testing.T) {
		var topic generated.Topic
		if err := json.Unmarshal([]byte(`{"id":1}`), &topic); err != nil {
			t.Fatalf("decode: %v", err)
		}
		out, _ := json.Marshal(topic)
		if strings.Contains(string(out), `"entries"`) {
			t.Errorf("an absent entries key came back: %s", out)
		}
	})
}

// TestConvertersKeepEmptyLists feeds every hand-written converter a fixture whose lists
// are `[]` and requires non-nil empty results that marshal back as `[]`.
func TestConvertersKeepEmptyLists(t *testing.T) {
	ctx := context.Background()

	t.Run("mapSlice", func(t *testing.T) {
		identity := func(i int) int { return i }
		if got := mapSlice([]int{}, identity); got == nil || len(got) != 0 {
			t.Errorf("mapSlice([]) = %#v, want non-nil empty", got)
		}
		if got := mapSlice([]int(nil), identity); got != nil {
			t.Errorf("mapSlice(nil) = %#v, want nil", got)
		}
	})

	t.Run("appendKept", func(t *testing.T) {
		if got := appendKept([]int(nil), []int{}); got == nil {
			t.Error("appendKept(nil, []) = nil, want non-nil empty")
		}
		if got := appendKept([]int(nil), []int(nil)); got != nil {
			t.Errorf("appendKept(nil, nil) = %#v, want nil", got)
		}
		if got := appendKept([]int{1}, []int{2, 3}); len(got) != 3 {
			t.Errorf("appendKept([1], [2 3]) = %v", got)
		}
	})

	t.Run("Workflows.GetStage", func(t *testing.T) {
		client := newServiceTestClient(t, map[string]string{
			"/workflows/%s/stages/%s.json": `{"id":5512,"name":"Applied","topics":[]}`,
		})
		stage, err := client.Workflows().GetStage(ctx, 8801, 5512)
		if err != nil {
			t.Fatalf("GetStage: %v", err)
		}
		assertEmptyList(t, stage.Topics, stage, `"topics":[]`)
	})

	t.Run("Workflows.List", func(t *testing.T) {
		client := newServiceTestClient(t, map[string]string{
			"/autocompletable/accounts/%s/workflows": `[]`,
		})
		workflows, err := client.Workflows().List(ctx, 77)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		assertEmptyList(t, workflows, workflows, `[]`)
	})

	t.Run("Client.GetAll", func(t *testing.T) {
		client := newServiceTestClient(t, map[string]string{"/boxes.json": `[]`})
		items, err := client.GetAll(ctx, "/boxes.json")
		if err != nil {
			t.Fatalf("GetAll: %v", err)
		}
		assertEmptyList(t, items, items, `[]`)
	})

	t.Run("Postings.AllChanges", func(t *testing.T) {
		c, cursor := newChangesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"added":[],"updated":[],"deleted":[]}`))
		})
		changes, err := c.Postings().AllChanges(ctx, 24088, cursor)
		if err != nil {
			t.Fatalf("AllChanges: %v", err)
		}
		assertEmptyList(t, changes.Added, changes, `"added":[]`)
		assertEmptyList(t, changes.Updated, changes, `"updated":[]`)
		assertEmptyList(t, changes.Deleted, changes, `"deleted":[]`)
	})

	t.Run("Calendars.AllCalendarChanges", func(t *testing.T) {
		c, cursor, _ := newCalendarChangesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"added":[],"updated":[],"deleted":[]}`))
		})
		changes, err := c.Calendars().AllCalendarChanges(ctx, cursor)
		if err != nil {
			t.Fatalf("AllCalendarChanges: %v", err)
		}
		assertEmptyList(t, changes.Added, changes, `"added":[]`)
		assertEmptyList(t, changes.Updated, changes, `"updated":[]`)
		assertEmptyList(t, changes.Deleted, changes, `"deleted":[]`)
	})

	t.Run("Calendars.AllRecordingChanges", func(t *testing.T) {
		c, _, cursor := newCalendarChangesTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"added":{"Calendar::Event":[]},"updated":{},"deleted":{"Calendar::Todo":[]}}`))
		})
		changes, err := c.Calendars().AllRecordingChanges(ctx, 512, cursor)
		if err != nil {
			t.Fatalf("AllRecordingChanges: %v", err)
		}
		if events, ok := changes.Added["Calendar::Event"]; !ok || events == nil {
			t.Errorf("added Calendar::Event bucket = %#v (present %v), want a non-nil empty slice", events, ok)
		}
		if changes.Updated == nil {
			t.Error("updated {} came back nil")
		}
		assertEmptyList(t, changes.Deleted, changes, `"deleted":[]`)
		out, _ := json.Marshal(changes)
		if !strings.Contains(string(out), `"Calendar::Event":[]`) {
			t.Errorf("added bucket lost its []: %s", out)
		}
	})

	t.Run("flattenDeletedRecordings", func(t *testing.T) {
		if got := flattenDeletedRecordings(map[string][]DeletedRecording{}); got == nil {
			t.Error("flattenDeletedRecordings({}) = nil, want non-nil empty")
		}
		if got := flattenDeletedRecordings(nil); got != nil {
			t.Errorf("flattenDeletedRecordings(nil) = %#v, want nil", got)
		}
	})
}

// assertEmptyList requires list to be a non-nil empty slice and whole to marshal with want in it.
func assertEmptyList(t *testing.T, list, whole any, want string) {
	t.Helper()
	v := reflect.ValueOf(list)
	if v.Kind() != reflect.Slice || v.IsNil() || v.Len() != 0 {
		t.Errorf("got %#v, want a non-nil empty slice", list)
	}
	out, err := json.Marshal(whole)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.Contains(string(out), want) {
		t.Errorf("marshaled %s, want it to contain %s", out, want)
	}
}

// notWireData names the exported structs that are not API data, so a caller has no reason
// to serialize them and their fields carry no json tags. Every other exported struct in
// the package is something a service method answers with — or a part of one — and each of
// its exported fields must carry a snake_case json tag, the wire's own spelling.
var notWireData = map[string]string{
	// Client plumbing and transport results.
	"Client": "client handle", "Response": "raw transport response", "FormResponse": "raw transport response",
	"BodyReadError": "error", "Error": "error", "FieldError": "error", "ContactConflictError": "error",
	"Router": "URL router handle",
	// Hooks and their arguments.
	"RequestInfo": "hook argument", "OperationInfo": "hook argument", "RequestResult": "hook argument",
	"NoopHooks": "hooks", "ChainHooks": "hooks",
	// Auth strategies and token providers.
	"BearerAuth": "auth strategy", "StaticTokenProvider": "token provider",
	// Client configuration.
	"HTTPOptions": "client option", "ResilienceConfig": "client option", "BulkheadConfig": "client option",
	"RateLimitConfig": "client option", "CircuitBreakerConfig": "client option",
	// Inputs a caller builds; the SDK writes the wire body from them itself.
	"CreateCollectionParams": "input", "UpdateCollectionParams": "input", "SearchParams": "input",
	"EventContentParams": "input", "CountdownParams": "input", "RepeatParams": "input",
	"CreateCalendarEventParams": "input", "UpdateCalendarEventParams": "input",
	"UpdateCalendarEventOccurrenceParams": "input", "TodoChanges": "input", "ScreenOptions": "input",
	"HabitParams": "input", "MessageContent": "input", "DraftContent": "input", "DraftSchedule": "input",
	"ContactParams": "input", "CreateExtenzionParams": "input", "UpdateExtenzionParams": "input",
	// oauth: PKCE material and token-endpoint inputs.
	"PKCE": "oauth input", "ExchangeRequest": "oauth input", "RefreshRequest": "oauth input",
}

var snakeCaseKey = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// TestExportedTypesUseSnakeCaseJSONKeys parses the package's own source: an exported
// struct the SDK hands back marshals with the wire's keys, not Go's field names. A struct
// added without tags fails here until it is tagged or named in notWireData with a reason.
func TestExportedTypesUseSnakeCaseJSONKeys(t *testing.T) {
	fset := token.NewFileSet()
	var checked int
	seen := map[string]bool{}
	for _, dir := range []string{".", "oauth"} {
		pkgs, err := parser.ParseDir(fset, dir, func(fi fs.FileInfo) bool {
			return !strings.HasSuffix(fi.Name(), "_test.go")
		}, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", dir, err)
		}
		for _, pkg := range pkgs {
			for _, file := range pkg.Files {
				ast.Inspect(file, func(n ast.Node) bool {
					spec, ok := n.(*ast.TypeSpec)
					if !ok || !spec.Name.IsExported() {
						return true
					}
					st, ok := spec.Type.(*ast.StructType)
					if !ok {
						return true
					}
					seen[spec.Name.Name] = true
					if _, skip := notWireData[spec.Name.Name]; skip {
						return true
					}
					checked++
					for _, field := range st.Fields.List {
						for _, name := range field.Names {
							if !name.IsExported() {
								continue
							}
							var key string
							if field.Tag != nil {
								tag, _ := strconv.Unquote(field.Tag.Value)
								key, _, _ = strings.Cut(reflect.StructTag(tag).Get("json"), ",")
							}
							if key != "-" && !snakeCaseKey.MatchString(key) {
								t.Errorf("%s: %s.%s has json key %q; want a snake_case tag (or list %s in notWireData)",
									fset.Position(name.Pos()), spec.Name.Name, name.Name, key, spec.Name.Name)
							}
						}
					}
					return true
				})
			}
		}
	}
	for name := range notWireData {
		if !seen[name] {
			t.Errorf("notWireData names %s, which is no longer an exported struct", name)
		}
	}
	if checked < 20 {
		t.Fatalf("checked %d structs: the scan missed the package", checked)
	}
}
