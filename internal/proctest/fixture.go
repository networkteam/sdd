package proctest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
)

// GraphShape sizes the generated realistic graph — one knob per surface that
// scales an automatic serve (the audit inventory on d-tac-rzi). The door
// test's old fixture was blind exactly because it populated none of these.
type GraphShape struct {
	// Entries is the general population: gaps, pending directives, plans, and
	// dones spread over the last weeks, so recency- and heat-ranked lanes
	// have real material.
	Entries int
	// Facts enroll in the retrieval index (the shell's factIndex lane).
	Facts int
	// Principles are facts under principles/interactive — the shell's
	// as-bodies framing lane, which renders full bodies.
	Principles int
	// Actors and RolesPerActor populate the participants lane.
	Actors        int
	RolesPerActor int
	// Focuses with InvolvementsPerFocus targets populate the focus lane.
	Focuses              int
	InvolvementsPerFocus int
	// Aspirations and GuidingDirectives populate their framing lanes.
	Aspirations       int
	GuidingDirectives int
	// WIPMarkers are marker files under wip/ (groom's sweep, catch-up's lane).
	WIPMarkers int
	// HubFanOut entries all reference one hub plan — the chain-expansion
	// worst case for entryChains (engage, implementation, evaluate).
	HubFanOut int
	// TopicLabels is the number of distinct topic labels spread across the
	// population (capture's topicLabels lane).
	TopicLabels int
}

// DefaultShape approximates the SDD repository's own graph where scale
// matters to a serve: lane populations at their real order of magnitude.
func DefaultShape() GraphShape {
	return GraphShape{
		Entries:              120,
		Facts:                15,
		Principles:           3,
		Actors:               4,
		RolesPerActor:        1,
		Focuses:              4,
		InvolvementsPerFocus: 3,
		Aspirations:          6,
		GuidingDirectives:    12,
		WIPMarkers:           3,
		HubFanOut:            40,
		TopicLabels:          30,
	}
}

// HubID returns the generated hub plan's entry ID for a base — the anchor
// for chain-expansion worst-case walks.
func HubID(base time.Time) string {
	return fixtureID(base, "d", "tac", "hub")
}

// DefaultBase anchors generated timestamps near the real clock: entries
// spread backwards from here, so recency windows (heat exp-14d, by(date))
// see a live graph. Capture it once per fixture — every generating call must
// share one base or IDs drift across a second boundary.
func DefaultBase() time.Time {
	return time.Now().Add(-2 * time.Hour).Truncate(time.Second)
}

func fixtureID(ts time.Time, typ, layer, suffix string) string {
	return fmt.Sprintf("%s-%s-%s-%s", ts.Format("20060102-150405"), typ, layer, suffix)
}

// prose builds deterministic filler at a realistic register: n clauses of
// project-shaped text, so byte weights match real summaries and bodies.
func prose(subject string, n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "The %s carries its reasoning in full so a later reader can rebuild the trade-off from the entry alone, clause %d of the record. ", subject, i+1)
	}
	return strings.TrimSpace(b.String())
}

// RealisticGraph generates the fixture entries for a shape, timestamped
// backwards from base. Every reference resolves inside the generated set, so
// the graph loads warning-clean and the health framing lane stays quiet —
// measured serves reflect content, not fixture damage.
func RealisticGraph(base time.Time, shape GraphShape) []*model.Entry {
	var entries []*model.Entry
	add := func(e *model.Entry) { entries = append(entries, e) }

	topicLabel := func(i int) string {
		if shape.TopicLabels <= 0 {
			return "area00/member00"
		}
		return fmt.Sprintf("area%02d/member%02d", i%shape.TopicLabels/6, i%shape.TopicLabels)
	}

	// Hub: the fan-out anchor, referenced by HubFanOut entries below.
	hubTime := base
	hub := &model.Entry{
		ID: fixtureID(hubTime, "d", "tac", "hub"), Type: model.TypeDecision, Kind: model.KindPlan,
		Layer: model.LayerTactical, Time: hubTime, Confidence: "high",
		Topics:  MustTopics("area00/member00"),
		Summary: prose("hub plan", 3),
		Content: prose("hub plan", 8) + "\n\n## Acceptance criteria\n\n- [ ] " + prose("criterion", 1),
	}
	add(hub)

	// General population: cycle gap → pending directive → plan → done (the
	// done closes the directive three positions earlier, so the recent-done
	// lane has real closures).
	var generalIDs []string
	for i := range shape.Entries {
		ts := base.Add(-time.Duration(i+1) * 7 * time.Hour)
		topics := MustTopics(topicLabel(i))
		var e *model.Entry
		switch i % 4 {
		case 0:
			e = &model.Entry{
				ID: fixtureID(ts, "s", "tac", fmt.Sprintf("g%03d", i)), Type: model.TypeSignal, Kind: model.KindGap,
				Layer: model.LayerTactical, Time: ts, Topics: topics,
				Summary: prose("gap", 3), Content: prose("gap", 6),
			}
		case 1:
			e = &model.Entry{
				ID: fixtureID(ts, "d", "tac", fmt.Sprintf("g%03d", i)), Type: model.TypeDecision, Kind: model.KindDirective,
				Intent: model.IntentPending, Layer: model.LayerTactical, Time: ts, Confidence: "medium", Topics: topics,
				Summary: prose("pending directive", 3), Content: prose("pending directive", 6),
			}
		case 2:
			e = &model.Entry{
				ID: fixtureID(ts, "d", "tac", fmt.Sprintf("g%03d", i)), Type: model.TypeDecision, Kind: model.KindPlan,
				Layer: model.LayerTactical, Time: ts, Confidence: "medium", Topics: topics,
				Summary: prose("plan", 3), Content: prose("plan", 6),
			}
		default:
			e = &model.Entry{
				ID: fixtureID(ts, "s", "tac", fmt.Sprintf("g%03d", i)), Type: model.TypeSignal, Kind: model.KindDone,
				Layer: model.LayerTactical, Time: ts, Topics: topics,
				Summary: prose("done", 3), Content: prose("done", 6),
			}
			if len(generalIDs) >= 2 {
				e.Closes = []string{generalIDs[len(generalIDs)-2]}
			}
		}
		if len(generalIDs) >= 3 {
			e.Refs = append(e.Refs, model.Ref{ID: generalIDs[len(generalIDs)-3], Kind: model.RefKindGroundedIn, Desc: "the earlier record this one reasons from"})
		}
		generalIDs = append(generalIDs, e.ID)
		add(e)
	}

	// Fan-out: entries whose refs all point at the hub, expanding its
	// downstream chain.
	for i := range shape.HubFanOut {
		ts := base.Add(-time.Duration(i+1)*3*time.Hour - 30*time.Minute)
		add(&model.Entry{
			ID: fixtureID(ts, "s", "tac", fmt.Sprintf("f%03d", i)), Type: model.TypeSignal, Kind: model.KindGap,
			Layer: model.LayerTactical, Time: ts, Topics: MustTopics(topicLabel(i)),
			Summary: prose("fan-out gap", 3), Content: prose("fan-out gap", 5),
			Refs: []model.Ref{{ID: hub.ID, Kind: model.RefKindAddresses, Desc: "the hub plan this gap bears on"}},
		})
	}

	// Aspirations (strategic) and guiding directives (conceptual).
	for i := range shape.Aspirations {
		ts := base.Add(-time.Duration(i+40) * 24 * time.Hour)
		add(&model.Entry{
			ID: fixtureID(ts, "d", "stg", fmt.Sprintf("a%02d", i)), Type: model.TypeDecision, Kind: model.KindAspiration,
			Layer: model.LayerStrategic, Time: ts, Confidence: "high", Topics: MustTopics(topicLabel(i)),
			Summary: prose("aspiration", 4), Content: prose("aspiration", 7),
		})
	}
	var guidingIDs []string
	for i := range shape.GuidingDirectives {
		ts := base.Add(-time.Duration(i+20)*24*time.Hour - time.Hour)
		e := &model.Entry{
			ID: fixtureID(ts, "d", "cpt", fmt.Sprintf("d%02d", i)), Type: model.TypeDecision, Kind: model.KindDirective,
			Intent: model.IntentGuiding, Layer: model.LayerConceptual, Time: ts, Confidence: "high", Topics: MustTopics(topicLabel(i)),
			Summary: prose("guiding directive", 4), Content: prose("guiding directive", 7),
		}
		guidingIDs = append(guidingIDs, e.ID)
		add(e)
	}

	// Facts: indexed (factIndex lane), plus principles facts whose full
	// bodies the shell's principles lane renders.
	for i := range shape.Facts {
		ts := base.Add(-time.Duration(i+10)*24*time.Hour - 2*time.Hour)
		label := topicLabel(i)
		index, err := model.NewFactIndex(fmt.Sprintf("Reference fact %02d: %s", i, prose("fact title", 1)), label)
		if err != nil {
			panic(err)
		}
		add(&model.Entry{
			ID: fixtureID(ts, "s", "prc", fmt.Sprintf("k%02d", i)), Type: model.TypeSignal, Kind: model.KindFact,
			Layer: model.LayerProcess, Time: ts, Confidence: "high", Topics: MustTopics(label), Index: index,
			Summary: prose("reference fact", 3), Content: prose("reference fact", 8),
		})
	}
	for i := range shape.Principles {
		ts := base.Add(-time.Duration(i+15)*24*time.Hour - 3*time.Hour)
		add(&model.Entry{
			ID: fixtureID(ts, "s", "prc", fmt.Sprintf("p%02d", i)), Type: model.TypeSignal, Kind: model.KindFact,
			Layer: model.LayerProcess, Time: ts, Confidence: "high", Topics: MustTopics("principles/interactive"),
			Summary: prose("working principle", 3),
			Content: "## Principle " + fmt.Sprintf("%02d", i) + "\n\n" + prose("working principle", 20),
		})
	}

	// Actors with bound roles (participants lane).
	for i := range shape.Actors {
		ts := base.Add(-time.Duration(i+60) * 24 * time.Hour)
		canonical := fmt.Sprintf("Actor %02d", i)
		add(&model.Entry{
			ID: fixtureID(ts, "s", "prc", fmt.Sprintf("x%02d", i)), Type: model.TypeSignal, Kind: model.KindActor,
			Layer: model.LayerProcess, Time: ts, Canonical: canonical,
			Summary: prose("actor identity", 3), Content: prose("actor identity", 4),
		})
		for r := range shape.RolesPerActor {
			rts := ts.Add(time.Duration(r+1) * time.Hour)
			add(&model.Entry{
				ID: fixtureID(rts, "d", "prc", fmt.Sprintf("r%02d%d", i, r)), Type: model.TypeDecision, Kind: model.KindRole,
				Layer: model.LayerProcess, Time: rts, Actor: canonical, Confidence: "high",
				Summary: prose("role binding", 3), Content: prose("role binding", 4),
			})
		}
	}

	// Focuses with involvement targets (focus lane).
	for i := range shape.Focuses {
		ts := base.Add(-time.Duration(i+5)*24*time.Hour - 4*time.Hour)
		var involvement []model.Involvement
		for j := range shape.InvolvementsPerFocus {
			target := hub.ID
			if len(guidingIDs) > 0 {
				target = guidingIDs[(i+j)%len(guidingIDs)]
			}
			involvement = append(involvement, model.Involvement{Target: target})
		}
		add(&model.Entry{
			ID: fixtureID(ts, "d", "tac", fmt.Sprintf("c%02d", i)), Type: model.TypeDecision, Kind: model.KindFocus,
			Layer: model.LayerTactical, Time: ts, Confidence: "high",
			FocusActors: []string{fmt.Sprintf("Actor %02d", i%max(shape.Actors, 1))},
			FocusWhen:   &model.FocusWhen{From: ts.Format("2006-01-02")},
			Involvement: involvement,
			Summary:     prose("tactical focus", 3), Content: prose("tactical focus", 5),
		})
	}

	return entries
}

// WriteWIPMarkers writes the shape's WIP marker files into the graph's wip/
// directory, each pointing at the base's generated hub entry.
func WriteWIPMarkers(t *testing.T, graphDir string, base time.Time, shape GraphShape) {
	t.Helper()
	for i := range shape.WIPMarkers {
		ts := base.Add(-time.Duration(i+1) * 26 * time.Hour)
		marker := &model.WIPMarker{
			ID:          fmt.Sprintf("%s-actor-%02d", ts.Format("20060102-150405"), i),
			Entry:       fixtureID(base, "d", "tac", "hub"),
			Participant: fmt.Sprintf("Actor %02d", i%max(shape.Actors, 1)),
			Exclusive:   i == 0,
			Content:     prose("in-flight work", 2),
			Time:        ts,
		}
		path := filepath.Join(graphDir, model.WIPMarkerPath(marker.ID))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(model.FormatWIPMarker(marker)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// NewRealisticWorld builds a world over a generated realistic graph and
// returns it with the hub entry's ID.
func NewRealisticWorld(t *testing.T, shape GraphShape) (*World, string) {
	t.Helper()
	base := DefaultBase()
	world := NewWorld(t, WithEntries(RealisticGraph(base, shape)...))
	WriteWIPMarkers(t, world.GraphDir, base, shape)
	return world, HubID(base)
}
