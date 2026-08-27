package model_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

// linkWarnings runs the read-side validation over a body and returns the
// attachment-link warnings it produced.
func linkWarnings(t *testing.T, id, body string, attachments ...string) []model.Warning {
	t.Helper()
	e := &model.Entry{
		ID: id, Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindDirective,
		Content: body, Attachments: attachments,
	}
	model.ValidateEntry(e, model.NewGraph([]*model.Entry{}))

	var out []model.Warning
	for _, w := range e.Warnings {
		if w.Field == "attachments" {
			out = append(out, w)
		}
	}
	return out
}

// TestAttachmentLinkCodeRegions pins the discriminator the check was missing
// (s-tac-ujs): a body documenting the attachment syntax inside code makes no
// claim to carry an attachment, while the same text in prose still does.
func TestAttachmentLinkCodeRegions(t *testing.T) {
	// The entry ID whose resolved prefix is ./06-115516-d-tac-beh/.
	const id = "20260406-115516-d-tac-beh"

	tests := []struct {
		name string
		body string
		want int
	}{
		{
			name: "prose link to a missing file still warns",
			body: "See [design]({{attachments}}/design.md) for details.",
			want: 1,
		},
		{
			name: "inline code span is documentation, not a claim",
			body: "Reference attachments as `{{attachments}}/design.md` in the body.",
			want: 0,
		},
		{
			name: "multi-backtick span holds a literal backtick",
			body: "Write `` {{attachments}}/design.md ` `` to document it.",
			want: 0,
		},
		{
			name: "backtick fence",
			body: "Example:\n\n```\nsdd new d tac \"See [d]({{attachments}}/design.md).\"\n```\n\nThat is all.",
			want: 0,
		},
		{
			name: "tilde fence",
			body: "Example:\n\n~~~\n[d]({{attachments}}/design.md)\n~~~\n",
			want: 0,
		},
		{
			name: "info-string fence",
			body: "Example:\n\n```sh\nsdd new --attach x.md \"[d]({{attachments}}/design.md)\"\n```\n",
			want: 0,
		},
		{
			name: "indented code block",
			body: "Example:\n\n    sdd new d tac \"See [d]({{attachments}}/design.md).\"\n\nThat is all.",
			want: 0,
		},
		{
			name: "resolved prefix inside a code span",
			body: "The link resolves to `./06-115516-d-tac-beh/design.md` on disk.",
			want: 0,
		},
		{
			name: "resolved prefix in prose still warns",
			body: "See [design](./06-115516-d-tac-beh/design.md) for details.",
			want: 1,
		},
		{
			name: "code elsewhere does not excuse a prose link",
			body: "Run `sdd lint` first.\n\nThen see [design]({{attachments}}/design.md).",
			want: 1,
		},
		{
			name: "a fence does not swallow the prose that follows it",
			body: "```\ncode\n```\n\nSee [design]({{attachments}}/design.md).",
			want: 1,
		},
		{
			name: "an unclosed backtick is literal, not a span opener",
			body: "A stray ` tick, then [design]({{attachments}}/design.md).",
			want: 1,
		},
		{
			name: "an indented paragraph continuation is not a code block",
			body: "A paragraph that wraps\n    onto [design]({{attachments}}/design.md) here.",
			want: 1,
		},
		{
			name: "both prefixes in one fence",
			body: "```\n[a]({{attachments}}/a.md) and [b](./06-115516-d-tac-beh/b.md)\n```\n",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := linkWarnings(t, id, tt.body)
			if len(got) != tt.want {
				for _, w := range got {
					t.Logf("warning: %s", w.Message)
				}
				t.Fatalf("got %d attachment-link warning(s), want %d", len(got), tt.want)
			}
		})
	}
}

// TestAttachmentLinkCarriedFileNeverWarns guards the direction the check
// exists for: a link whose file the entry actually carries is valid wherever
// it appears.
func TestAttachmentLinkCarriedFileNeverWarns(t *testing.T) {
	const id = "20260406-115516-d-tac-beh"
	body := "See [design]({{attachments}}/design.md), documented as `{{attachments}}/design.md`."

	if got := linkWarnings(t, id, body, "2026/04/06-115516-d-tac-beh/design.md"); len(got) != 0 {
		t.Fatalf("got %d warning(s) for a carried attachment, want 0", len(got))
	}
}

// TestAttachmentLinkWriteGateSkipsCode pins that the write gate — where a
// finding blocks unconditionally with no override path — reads code regions
// the same way the read side does.
func TestAttachmentLinkWriteGateSkipsCode(t *testing.T) {
	c := &model.EntryConstruction{
		ID: "20260406-115516-d-tac-beh", Type: model.TypeDecision,
		Layer: model.LayerTactical, Kind: model.KindDirective,
		Body: "Reference attachments as `{{attachments}}/design.md` in the body.",
	}
	_, findings := c.ValidateForWrite(model.NewGraph([]*model.Entry{}))
	for _, f := range findings {
		if strings.Contains(f.Message, "broken attachment link") {
			t.Fatalf("write gate blocked a documented example: %s", f.Message)
		}
	}
}

// TestAttachmentLinkWriteGateStillBlocksProse is the counterpart: the claim
// the check exists to stop is unaffected by the code-region skip.
func TestAttachmentLinkWriteGateStillBlocksProse(t *testing.T) {
	c := &model.EntryConstruction{
		ID: "20260406-115516-d-tac-beh", Type: model.TypeDecision,
		Layer: model.LayerTactical, Kind: model.KindDirective,
		Body: "The full record is attached: [review]({{attachments}}/review.md).",
	}
	_, findings := c.ValidateForWrite(model.NewGraph([]*model.Entry{}))
	for _, f := range findings {
		if strings.Contains(f.Message, "broken attachment link") {
			return
		}
	}
	t.Fatal("write gate let a false attachment claim through")
}
