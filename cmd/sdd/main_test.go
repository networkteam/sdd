package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func TestSessionStoreCollisionMessageIsSourceNeutral(t *testing.T) {
	message := sessionStoreCollisionMessage("/old-global/session.jsonl", "/desired/session.jsonl")
	if strings.Contains(message, "in-tree") || !strings.Contains(message, "source-store relocation") {
		t.Fatalf("source-specific collision message = %q", message)
	}
}

func TestChooseSessionStoreRelocationRequiresExplicitNonInteractiveOptIn(t *testing.T) {
	promptCalls := 0
	prompt := func() (bool, error) {
		promptCalls++
		return true, nil
	}

	got, err := chooseSessionStoreRelocation(true, false, false, prompt)
	if err != nil || got || promptCalls != 0 {
		t.Fatalf("non-interactive choice = %v, %v; prompt calls %d", got, err, promptCalls)
	}
	got, err = chooseSessionStoreRelocation(true, true, false, prompt)
	if err != nil || !got || promptCalls != 0 {
		t.Fatalf("explicit non-interactive choice = %v, %v; prompt calls %d", got, err, promptCalls)
	}
	got, err = chooseSessionStoreRelocation(true, false, true, prompt)
	if err != nil || !got || promptCalls != 1 {
		t.Fatalf("interactive choice = %v, %v; prompt calls %d", got, err, promptCalls)
	}
	declined, err := chooseSessionStoreRelocation(true, false, true, func() (bool, error) { return false, nil })
	if err != nil || declined {
		t.Fatalf("declined interactive choice = %v, %v", declined, err)
	}
}

func TestSessionStoreTransitionSummaryDescribesActualTransitionKinds(t *testing.T) {
	tests := []struct {
		name    string
		summary sessionStoreTransitionSummary
		want    []string
		avoid   string
	}{
		{
			name: "in-tree only", summary: sessionStoreTransitionSummary{InTreePayloads: 2},
			want: []string{"2 in-tree", "machine-global store"}, avoid: "rekeying",
		},
		{
			name: "old global rekey", summary: sessionStoreTransitionSummary{OldKeyPayloads: 3},
			want: []string{"3 identity-less global", "rekeying", "repository-ID"}, avoid: "in-tree",
		},
		{
			name: "marker only", summary: sessionStoreTransitionSummary{MarkerOnly: true},
			want: []string{"previously declined", "remains pending"}, avoid: "payload file",
		},
		{
			name: "old-target tombstone", summary: sessionStoreTransitionSummary{OldTargetTombstone: true},
			want: []string{"earlier in-tree relocation", "identity-less global store", "repository-ID cutover"}, avoid: "payload file",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.summary.description()
			for _, want := range test.want {
				if !strings.Contains(got, want) {
					t.Fatalf("description %q lacks %q", got, want)
				}
			}
			if test.avoid != "" && strings.Contains(got, test.avoid) {
				t.Fatalf("description %q unexpectedly contains %q", got, test.avoid)
			}
		})
	}
}

// The prompt header is the regression test for the acknowledgement that read as
// a status line: the census, the precondition and the question have to occupy
// separate lines, and no single line may be long enough to be truncated at a
// terminal edge before the reader reaches the question.
func TestSessionStoreTransitionPromptHeaderSeparatesCensusPreconditionAndQuestion(t *testing.T) {
	summary := sessionStoreTransitionSummary{InTreePayloads: 145, OldKeyPayloads: 2}
	header := summary.promptHeader()
	lines := strings.Split(header, "\n")

	if lines[0] != "Session store relocation is pending:" {
		t.Errorf("first line = %q, want the heading alone", lines[0])
	}
	if got := lines[len(lines)-1]; got != "Relocate now?" {
		t.Errorf("last line = %q, want the question alone", got)
	}

	var censusItems, precondition int
	for _, line := range lines {
		if strings.HasPrefix(line, "  - ") {
			censusItems++
		}
		if strings.Contains(line, "Stop every `sdd serve` process") {
			precondition++
			if strings.Contains(line, "Relocate now?") {
				t.Error("precondition and question share a line")
			}
		}
	}
	if censusItems != 2 {
		t.Errorf("census items = %d, want one line per pending item", censusItems)
	}
	if precondition != 1 {
		t.Errorf("precondition lines = %d, want exactly one", precondition)
	}
}

// Both surfaces render from one list, so a pending kind can never appear in the
// prompt but go missing from the non-interactive notice. Only the header is
// worth asserting: description() is a join over the same slice, so checking it
// against its own input could not fail.
func TestSessionStoreTransitionPromptHeaderCarriesEveryPendingItem(t *testing.T) {
	summary := sessionStoreTransitionSummary{InTreePayloads: 4, OldKeyPayloads: 1, MarkerOnly: true}
	items := summary.pendingItems()
	if len(items) != 3 {
		t.Fatalf("pending items = %d, want one per pending kind", len(items))
	}
	header := summary.promptHeader()
	for _, item := range items {
		if !strings.Contains(header, item) {
			t.Errorf("prompt header is missing pending item %q", item)
		}
	}
}

// Enter must leave the store alone. This is the safety property the whole
// prompt exists for: the defect it replaced silently answered for the user, and
// a default landing on "Relocate now" would do the same thing in the opposite
// direction. It must survive any future change of prompt widget.
func TestRelocationPromptDefaultsToDeclining(t *testing.T) {
	options := relocationPromptOptions()
	if relocationPromptDefaultCursor < 0 || relocationPromptDefaultCursor >= len(options) {
		t.Fatalf("default cursor %d is outside the offered options", relocationPromptDefaultCursor)
	}
	if preselected := options[relocationPromptDefaultCursor]; preselected.Value {
		t.Errorf("enter preselects %q, which relocates; the default must decline", preselected.Label)
	}
	var relocating int
	for _, option := range options {
		if option.Value {
			relocating++
		}
	}
	if relocating != 1 {
		t.Errorf("relocating options = %d, want exactly one", relocating)
	}
}

// TestSplitCSV_TrimsWhitespaceAndDropsEmpty is the regression test for the
// CSV whitespace-trim bug (s-prc-omw, d-tac-955) and the d-prc-8vh contract
// requiring regression tests for bug fixes. Before the fix,
// `--participants "Christopher, Claude"` stored " Claude" with leading space
// as a distinct participant identity across ~30 entries in the graph.
func TestSplitCSV_TrimsWhitespaceAndDropsEmpty(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"Christopher", []string{"Christopher"}},
		// The regression case: comma-space-separated input must not leak " Claude".
		{"Christopher, Claude", []string{"Christopher", "Claude"}},
		{"  Christopher  ,  Claude  ", []string{"Christopher", "Claude"}},
		{"Christopher,Claude", []string{"Christopher", "Claude"}},
		// Empty elements after trim are dropped so stray commas don't produce phantom participants.
		{"Christopher,,Claude", []string{"Christopher", "Claude"}},
		{",,", nil},
		{"   ", nil},
	}
	for _, tt := range tests {
		got := splitCSV(tt.input)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitCSV(%q) = %#v, want %#v", tt.input, got, tt.want)
		}
	}
}

func TestParseAttachSpec(t *testing.T) {
	tests := []struct {
		spec       string
		wantSource string
		wantTarget string
	}{
		// Plain path — no colon, target empty (basename fallback handled by caller)
		{"file.md", "file.md", ""},
		{"/tmp/design.md", "/tmp/design.md", ""},

		// source:target mapping
		{"/tmp/design.md:plan.md", "/tmp/design.md", "plan.md"},
		{"notes.txt:renamed.txt", "notes.txt", "renamed.txt"},

		// Stdin alias
		{"-:plan-requirements.md", "-", "plan-requirements.md"},

		// Bare stdin (caller validates that target is required)
		{"-", "-", ""},

		// Colon in source path — splits on last colon
		{"/path/with:colon/file.md:target.md", "/path/with:colon/file.md", "target.md"},
	}

	for _, tt := range tests {
		src, tgt := parseAttachSpec(tt.spec)
		if src != tt.wantSource || tgt != tt.wantTarget {
			t.Errorf("parseAttachSpec(%q) = (%q, %q), want (%q, %q)",
				tt.spec, src, tgt, tt.wantSource, tt.wantTarget)
		}
	}
}

func TestParseAttachFlags_PlainPath(t *testing.T) {
	tmp := canonicalTempDir(t)
	f := filepath.Join(tmp, "design.md")
	if err := os.WriteFile(f, []byte("# Design"), 0644); err != nil {
		t.Fatal(err)
	}

	atts, err := parseAttachFlags([]string{f}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 1 {
		t.Fatalf("got %d attachments, want 1", len(atts))
	}
	if atts[0].source != f {
		t.Errorf("source = %q, want %q", atts[0].source, f)
	}
	if atts[0].target != "design.md" {
		t.Errorf("target = %q, want %q", atts[0].target, "design.md")
	}
}

func TestParseAttachFlags_SourceTargetMapping(t *testing.T) {
	tmp := canonicalTempDir(t)
	f := filepath.Join(tmp, "tmpXXX.md")
	if err := os.WriteFile(f, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	atts, err := parseAttachFlags([]string{f + ":plan.md"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 1 {
		t.Fatalf("got %d attachments, want 1", len(atts))
	}
	if atts[0].source != f {
		t.Errorf("source = %q, want %q", atts[0].source, f)
	}
	if atts[0].target != "plan.md" {
		t.Errorf("target = %q, want %q", atts[0].target, "plan.md")
	}
}

func TestParseAttachFlags_Stdin(t *testing.T) {
	reader := strings.NewReader("stdin content")

	atts, err := parseAttachFlags([]string{"-:output.md"}, reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 1 {
		t.Fatalf("got %d attachments, want 1", len(atts))
	}
	if atts[0].source != "-" {
		t.Errorf("source = %q, want %q", atts[0].source, "-")
	}
	if atts[0].target != "output.md" {
		t.Errorf("target = %q, want %q", atts[0].target, "output.md")
	}
	if string(atts[0].data) != "stdin content" {
		t.Errorf("data = %q, want %q", string(atts[0].data), "stdin content")
	}
}

func TestParseAttachFlags_BareStdinError(t *testing.T) {
	_, err := parseAttachFlags([]string{"-"}, strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for bare stdin, got nil")
	}
	if !strings.Contains(err.Error(), "requires a target name") {
		t.Errorf("error = %q, want it to mention target name requirement", err.Error())
	}
}

func TestParseAttachFlags_DuplicateStdinError(t *testing.T) {
	reader := strings.NewReader("content")

	_, err := parseAttachFlags([]string{"-:a.md", "-:b.md"}, reader)
	if err == nil {
		t.Fatal("expected error for duplicate stdin, got nil")
	}
	if !strings.Contains(err.Error(), "only be used once") {
		t.Errorf("error = %q, want it to mention single use", err.Error())
	}
}

func TestParseAttachFlags_MissingFileError(t *testing.T) {
	_, err := parseAttachFlags([]string{"/nonexistent/file.md"}, nil)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to mention not found", err.Error())
	}
}

func TestParseAttachFlags_MultipleAttachments(t *testing.T) {
	tmp := canonicalTempDir(t)
	f1 := filepath.Join(tmp, "a.md")
	f2 := filepath.Join(tmp, "b.md")
	if err := os.WriteFile(f1, []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}

	reader := strings.NewReader("from stdin")

	atts, err := parseAttachFlags([]string{f1, f2 + ":renamed.md", "-:stdin.md"}, reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 3 {
		t.Fatalf("got %d attachments, want 3", len(atts))
	}
	// First: plain path, basename fallback
	if atts[0].target != "a.md" {
		t.Errorf("atts[0].target = %q, want %q", atts[0].target, "a.md")
	}
	// Second: source:target mapping
	if atts[1].target != "renamed.md" {
		t.Errorf("atts[1].target = %q, want %q", atts[1].target, "renamed.md")
	}
	// Third: stdin
	if atts[2].target != "stdin.md" {
		t.Errorf("atts[2].target = %q, want %q", atts[2].target, "stdin.md")
	}
	if string(atts[2].data) != "from stdin" {
		t.Errorf("atts[2].data = %q, want %q", string(atts[2].data), "from stdin")
	}
}

func TestParseRefFlags_ObjectForm(t *testing.T) {
	specs := []string{
		`{"id":"20260101-000000-s-cpt-aaa","kind":"grounded-in"}`,
		`{"id":"20260102-000000-s-cpt-bbb","kind":"addresses","desc":"resolves the gap"}`,
	}
	got, err := parseRefFlags(specs)
	if err != nil {
		t.Fatalf("parseRefFlags: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	want0 := model.Ref{ID: "20260101-000000-s-cpt-aaa", Kind: model.RefKindGroundedIn}
	if got[0] != want0 {
		t.Errorf("got[0] = %+v, want %+v", got[0], want0)
	}
	want1 := model.Ref{ID: "20260102-000000-s-cpt-bbb", Kind: model.RefKindAddresses, Desc: "resolves the gap"}
	if got[1] != want1 {
		t.Errorf("got[1] = %+v, want %+v", got[1], want1)
	}
}

func TestParseRefFlags_CrossRepo(t *testing.T) {
	// A well-formed cross-repo ref (<repo-id>:<entry-id>) is accepted with
	// syntactic validation of both parts; malformed forms are rejected.
	got, err := parseRefFlags([]string{`{"id":"github.com/networkteam/other:20260601-120000-s-tac-abc","kind":"grounded-in"}`})
	if err != nil {
		t.Fatalf("parseRefFlags: %v", err)
	}
	if got[0].ID != "github.com/networkteam/other:20260601-120000-s-tac-abc" {
		t.Errorf("cross-repo ref ID mutated: %q", got[0].ID)
	}
	for _, id := range []string{
		"nohost:20260601-120000-s-tac-abc",
		"github.com/networkteam/other:s-tac-abc",
	} {
		spec := `{"id":"` + id + `","kind":"grounded-in"}`
		if _, err := parseRefFlags([]string{spec}); err == nil {
			t.Errorf("parseRefFlags(%q): want rejection of malformed cross-repo ref", id)
		}
	}
}

func TestParseRefFlags_LegacyKindRejected(t *testing.T) {
	// AC 1: grounds and evidence are not capturable — the read-layer alias maps
	// them on disk, but a new capture must use grounded-in. Reject with the
	// closed-set guidance.
	for _, kind := range []string{"grounds", "evidence"} {
		spec := `{"id":"20260101-000000-s-cpt-aaa","kind":"` + kind + `"}`
		if _, err := parseRefFlags([]string{spec}); err == nil {
			t.Errorf("parseRefFlags(kind=%q): want rejection (legacy kind not capturable)", kind)
		}
	}
}

func TestParseRefFlags_BareString_RejectedWithGuidance(t *testing.T) {
	// AC 7: bare-string --refs id1,id2 is rejected with a clear error
	// directing the user to the new JSON form.
	cases := []string{
		"20260101-000000-s-cpt-aaa",
		"20260101-000000-s-cpt-aaa,20260102-000000-s-cpt-bbb",
		"s-cpt-aaa",
	}
	for _, spec := range cases {
		_, err := parseRefFlags([]string{spec})
		if err == nil {
			t.Errorf("parseRefFlags(%q): want error", spec)
			continue
		}
		if !strings.Contains(err.Error(), "JSON object") {
			t.Errorf("parseRefFlags(%q): error %q should mention JSON object form", spec, err.Error())
		}
	}
}

func TestParseRefFlags_MissingKind(t *testing.T) {
	_, err := parseRefFlags([]string{`{"id":"20260101-000000-s-cpt-aaa"}`})
	if err == nil {
		t.Fatal("want error for missing kind")
	}
	if !strings.Contains(err.Error(), "kind") {
		t.Errorf("error %q should mention kind", err.Error())
	}
}

func TestParseRefFlags_MissingID(t *testing.T) {
	_, err := parseRefFlags([]string{`{"kind":"grounds"}`})
	if err == nil {
		t.Fatal("want error for missing id")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Errorf("error %q should mention id", err.Error())
	}
}

func TestParseRefFlags_InvalidKind(t *testing.T) {
	_, err := parseRefFlags([]string{`{"id":"20260101-000000-s-cpt-aaa","kind":"bogus"}`})
	if err == nil {
		t.Fatal("want error for invalid kind")
	}
	if !strings.Contains(err.Error(), "invalid kind") {
		t.Errorf("error %q should mention invalid kind", err.Error())
	}
}

func TestParseRefFlags_UnknownKindRejectedAtCapture(t *testing.T) {
	// The `unknown` sentinel exists for legacy bare-string round-trip on disk
	// but is rejected at capture — new entries must use one of the seven
	// semantic kinds.
	_, err := parseRefFlags([]string{`{"id":"20260101-000000-s-cpt-aaa","kind":"unknown"}`})
	if err == nil {
		t.Fatal("want error for kind: unknown at capture")
	}
	if !strings.Contains(err.Error(), "invalid kind") {
		t.Errorf("error %q should reject unknown at capture", err.Error())
	}
}

func TestParseFactIndexFlag(t *testing.T) {
	index, err := parseFactIndexFlag(`{"title":"How to compose graph views","topic":"cli/view"}`)
	if err != nil {
		t.Fatal(err)
	}
	if index.Title != "How to compose graph views" || index.Topic.String() != "cli/view" {
		t.Fatalf("index = %+v", index)
	}
	for _, input := range []string{
		`{"title":"Missing topic"}`,
		`{"title":" ","topic":"cli/view"}`,
		`{"title":"Title","topic":"cli view"}`,
		`{"title":"Title\n## Injected","topic":"cli/view"}`,
		`{"title":"Title\u0007Injected","topic":"cli/view"}`,
		`{"title":"Title\u2028Injected","topic":"cli/view"}`,
		`{"title":"Title","topic":"cli/view","extra":true}`,
		`{"title":"Title","topic":"cli/view"} {}`,
	} {
		if _, err := parseFactIndexFlag(input); err == nil {
			t.Errorf("parseFactIndexFlag(%q) accepted invalid object", input)
		}
	}
}

func TestParseRefFlags_EmptyInput(t *testing.T) {
	got, err := parseRefFlags(nil)
	if err != nil {
		t.Fatalf("parseRefFlags(nil): %v", err)
	}
	if got != nil {
		t.Errorf("got = %v, want nil", got)
	}
}

func TestParseRefFlags_MalformedJSON(t *testing.T) {
	_, err := parseRefFlags([]string{`{"id":"a","kind":"grounds",`})
	if err == nil {
		t.Fatal("want error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("error %q should mention invalid JSON", err.Error())
	}
}
