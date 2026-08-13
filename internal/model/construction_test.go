package model_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
)

func constructionTime(t *testing.T, id string) time.Time {
	t.Helper()
	parts, err := model.ParseID(id)
	if err != nil {
		t.Fatalf("ParseID(%s): %v", id, err)
	}
	return parts.Time
}

func topicPath(t *testing.T, label string) model.TopicPath {
	t.Helper()
	p, err := model.ParseTopicPath(label)
	if err != nil {
		t.Fatalf("ParseTopicPath(%s): %v", label, err)
	}
	return p
}

// constructionCases covers every current kind with a valid construction.
func constructionCases(t *testing.T) map[string]*model.EntryConstruction {
	t.Helper()
	ref := model.Ref{ID: "20260601-120000-d-tac-ref", Kind: model.RefKind("addresses"), Desc: "the commitment this realizes"}
	cases := map[string]*model.EntryConstruction{
		"gap": {
			ID: "20260813-120000-s-tac-gap", Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindGap,
			Refs: []model.Ref{ref}, Participants: []string{"Christopher"}, Confidence: "high",
			Topics: []model.TopicPath{topicPath(t, "graph/entry-craft")},
			Body:   "A gap body.",
		},
		"fact": {
			ID: "20260813-120000-s-prc-fct", Type: model.TypeSignal, Layer: model.LayerProcess, Kind: model.KindFact,
			Topics: []model.TopicPath{topicPath(t, "engine/base-facts")},
			Fact:   &model.FactFields{Index: mustFactIndex(t, "Test fact", "engine/base-facts")},
			Body:   "A fact body.",
		},
		"question": {
			ID: "20260813-120000-s-cpt-qst", Type: model.TypeSignal, Layer: model.LayerConceptual, Kind: model.KindQuestion,
			Refs: []model.Ref{ref}, Body: "A question body?",
		},
		"insight": {
			ID: "20260813-120000-s-cpt-ins", Type: model.TypeSignal, Layer: model.LayerConceptual, Kind: model.KindInsight,
			Refs: []model.Ref{ref}, Body: "An insight body.",
		},
		"done": {
			ID: "20260813-120000-s-tac-don", Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindDone,
			Closes: []string{"20260601-120000-d-tac-ref"}, Body: "Delivered in commit abc1234.",
		},
		"actor": {
			ID: "20260813-120000-s-prc-act", Type: model.TypeSignal, Layer: model.LayerProcess, Kind: model.KindActor,
			Actor: &model.ActorFields{Canonical: "Jane", Aliases: []string{"Jane Doe"}},
			Body:  "Jane is a participant.",
		},
		"annotation": {
			ID: "20260813-120000-s-tac-ann", Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindAnnotation,
			Refs: []model.Ref{ref},
			Annotation: &model.AnnotationFields{Topics: []model.AnnotationTopic{
				{Label: "graph/entry-craft"},
				{Label: "engine/base-facts", Members: []string{ref.ID}},
			}},
			Body: "Retroactive topic assignment.",
		},
		"directive": {
			ID: "20260813-120000-d-tac-dir", Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindDirective,
			Refs: []model.Ref{ref}, Directive: &model.DirectiveFields{Intent: model.IntentPending},
			Body: "A directive body.",
		},
		"activity": {
			ID: "20260813-120000-d-tac-avt", Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindActivity,
			Refs: []model.Ref{ref}, Body: "An activity body.",
		},
		"plan": {
			ID: "20260813-120000-d-tac-pln", Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindPlan,
			Refs: []model.Ref{ref},
			Body: "A plan body.\n\n## Acceptance criteria\n\n- [ ] The first criterion holds.",
		},
		"contract": {
			ID: "20260813-120000-d-cpt-ctr", Type: model.TypeDecision, Layer: model.LayerConceptual, Kind: model.KindContract,
			Refs: []model.Ref{ref}, Body: "A contract body.",
		},
		"aspiration": {
			ID: "20260813-120000-d-stg-asp", Type: model.TypeDecision, Layer: model.LayerStrategic, Kind: model.KindAspiration,
			Refs: []model.Ref{ref}, Body: "An aspiration body.",
		},
		"role": {
			ID: "20260813-120000-d-prc-rol", Type: model.TypeDecision, Layer: model.LayerProcess, Kind: model.KindRole,
			Refs: []model.Ref{ref}, Role: &model.RoleFields{Actor: "Jane"},
			Body: "Jane's role.",
		},
		"focus": {
			ID: "20260813-120000-d-tac-foc", Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindFocus,
			Focus: &model.FocusFields{
				Actors: []string{"Jane"},
				When:   &model.FocusWhen{From: "2026-08-13"},
				Involvement: []model.Involvement{
					{Target: "20260601-120000-d-tac-ref", Actors: []string{"Jane"}, ActorsSet: true},
				},
			},
			Body: "A focus body.",
		},
		"procedure": {
			ID: "20260813-120000-d-prc-prd", Type: model.TypeDecision, Layer: model.LayerProcess, Kind: model.KindProcedure,
			Procedure: &model.ProcedureFields{Canonical: "test-move", Class: model.ProcedureClassShell},
			Body:      "A procedure body.",
		},
	}
	for _, c := range cases {
		c.Time = constructionTime(t, c.ID)
	}
	return cases
}

func mustFactIndex(t *testing.T, title, topic string) *model.FactIndex {
	t.Helper()
	idx, err := model.NewFactIndex(title, topic)
	if err != nil {
		t.Fatalf("NewFactIndex: %v", err)
	}
	return idx
}

// TestConstructionValidates proves every kind's valid construction passes the
// one rule set with zero findings.
func TestConstructionValidates(t *testing.T) {
	for name, c := range constructionCases(t) {
		t.Run(name, func(t *testing.T) {
			if findings := c.Validate(nil); len(findings) > 0 {
				t.Errorf("expected no findings, got %+v", findings)
			}
		})
	}
}

// TestConstructionRoundTrip proves a constructed entry serializes and
// re-parses to an equal raw form for every current kind.
func TestConstructionRoundTrip(t *testing.T) {
	for name, c := range constructionCases(t) {
		t.Run(name, func(t *testing.T) {
			entry := c.Entry()
			serialized := model.FormatFrontmatter(entry) + "\n" + entry.Content + "\n"
			reparsed, err := model.ParseEntry(c.ID+".md", serialized)
			if err != nil {
				t.Fatalf("re-parsing serialized entry: %v", err)
			}
			if len(reparsed.Warnings) > 0 {
				t.Fatalf("re-parse produced warnings: %+v", reparsed.Warnings)
			}
			if !reflect.DeepEqual(entry, reparsed) {
				t.Errorf("round-trip mismatch:\nconstructed: %+v\nreparsed:    %+v", entry, reparsed)
			}
			// The projection of the re-parsed form must be lossless for a
			// valid entry: no drift findings, and equal construction.
			back, findings := model.ConstructFromEntry(reparsed)
			if len(findings) > 0 {
				t.Errorf("projection of valid entry produced findings: %+v", findings)
			}
			if !reflect.DeepEqual(c, back) {
				t.Errorf("projection mismatch:\nconstructed: %+v\nprojected:   %+v", c, back)
			}
		})
	}
}

// TestConstructionWriteOnlyRules pins the capture-only rules: they block a
// new capture but never warn on a historical entry.
func TestConstructionWriteOnlyRules(t *testing.T) {
	legacyDirective := &model.EntryConstruction{
		ID: "20260101-120000-d-tac-old", Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindDirective,
		Body: "A legacy directive without intent.",
	}
	legacyPlan := &model.EntryConstruction{
		ID: "20260101-120000-d-tac-olp", Type: model.TypeDecision, Layer: model.LayerTactical, Kind: model.KindPlan,
		Body: "A legacy plan without acceptance criteria.",
	}
	for name, c := range map[string]*model.EntryConstruction{"directive-intent": legacyDirective, "plan-acceptance": legacyPlan} {
		t.Run(name, func(t *testing.T) {
			findings := c.Validate(nil)
			if len(findings) != 1 || !findings[0].WriteOnly {
				t.Fatalf("expected exactly one write-only finding, got %+v", findings)
			}
			if warnings := model.ReadWarnings(findings); len(warnings) != 0 {
				t.Errorf("write-only finding leaked into read warnings: %+v", warnings)
			}
		})
	}
}

// TestConstructionBlockAgreement pins the at-most-one-block rule: a block on
// the wrong kind is a finding, on read and write alike.
func TestConstructionBlockAgreement(t *testing.T) {
	c := &model.EntryConstruction{
		ID: "20260813-120000-s-tac-bad", Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindGap,
		Directive: &model.DirectiveFields{Intent: model.IntentPending},
		Role:      &model.RoleFields{Actor: "Jane"},
		Body:      "A gap carrying foreign blocks.",
	}
	findings := c.Validate(nil)
	if len(findings) != 2 {
		t.Fatalf("expected two agreement findings, got %+v", findings)
	}
	for _, f := range findings {
		if f.WriteOnly {
			t.Errorf("agreement finding must hold on read too: %+v", f)
		}
		if !strings.Contains(f.Message, "only meaningful on") {
			t.Errorf("unexpected message: %s", f.Message)
		}
	}
}

// TestConstructFromEntry_StrayFields pins the lossy projection: fields that
// do not belong to the kind surface as findings and drop from the projection.
func TestConstructFromEntry_StrayFields(t *testing.T) {
	entry := &model.Entry{
		ID: "20260813-120000-s-tac-sty", Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindGap,
		Intent:    model.IntentPending,
		Canonical: "stray",
		Actor:     "stray",
		Class:     model.ProcedureClassMove,
		Content:   "A gap with stray per-kind fields.",
	}
	c, findings := model.ConstructFromEntry(entry)
	if len(findings) != 4 {
		t.Fatalf("expected four stray-field findings, got %+v", findings)
	}
	if c.Directive != nil || c.Actor != nil || c.Role != nil || c.Procedure != nil {
		t.Errorf("stray fields must not populate blocks: %+v", c)
	}
}

// TestConstructFromEntry_Kindless pins the projection of a kindless signal:
// no block, and the kind finding comes from validation, not the projection.
func TestConstructFromEntry_Kindless(t *testing.T) {
	entry := &model.Entry{
		ID: "20260101-120000-s-tac-nok", Type: model.TypeSignal, Layer: model.LayerTactical,
		Content: "A historical kindless signal.",
	}
	c, findings := model.ConstructFromEntry(entry)
	if len(findings) != 0 {
		t.Fatalf("projection of kindless entry must not fail: %+v", findings)
	}
	validation := c.Validate(nil)
	if len(validation) != 1 || validation[0].Field != "kind" || validation[0].WriteOnly {
		t.Fatalf("expected one read-visible kind finding, got %+v", validation)
	}
}
