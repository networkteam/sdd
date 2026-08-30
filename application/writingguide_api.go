package application

import (
	"context"
	"fmt"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// GuideFinding mirrors query.GuideFinding at the API boundary.
type GuideFinding struct {
	Reasoning string
	Axis      string
	Quote     string
	Repair    string
	Severity  string
}

// WritingGuideCheck runs the writing guide against a draft in isolation — the
// pre-playback half of the capture-guide architecture (d-cpt-20r). The draft's
// own fields feed the prompt and refs stay unresolved, because the guide's
// scope is the entry as a stranger reads it. The single exception is the
// draft's closure edges, described one summary sentence deep: which act an
// entry performs is not inferable from the body alone, and a guide that cannot
// see it misreads correct entries (s-tac-fu8). Findings are drafting input for
// the dialogue, never a gate.
func (a *Application) WritingGuideCheck(ctx context.Context, identity RequestIdentity, project ProjectID, draft EntryDraft) ([]GuideFinding, error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return nil, err
	}
	kind := model.Kind(draft.Kind)
	entryType, err := entryTypeForKind(kind)
	if err != nil {
		return nil, err
	}
	layer := model.Layer(draft.Layer)
	if expanded, ok := model.LayerFromAbbrev[draft.Layer]; ok {
		layer = expanded
	}
	entry := &model.Entry{
		Type: entryType, Kind: kind, Layer: layer,
		Intent: model.Intent(draft.Intent), Content: draft.Body,
		Attachments: append([]string(nil), draft.AttachmentHandles...),
	}
	for _, ref := range draft.Refs {
		entry.Refs = append(entry.Refs, model.Ref{ID: ref.ID, Kind: model.RefKind(ref.Kind), Desc: ref.Desc})
	}
	entry.Closes = append([]string(nil), draft.Closes...)
	entry.Supersedes = append([]string(nil), draft.Supersedes...)

	// The snapshot serves two reads: closure targets, and the reference facts
	// the prompt renders from the graph (type-system overview plus the drafted
	// kind's authoring fact).
	snapshot, err := a.snapshotWithDependencies(ctx, identity, runtime)
	if err != nil {
		return nil, err
	}
	var closureTargets []model.ClosureTarget
	if len(entry.Closes) > 0 || len(entry.Supersedes) > 0 {
		// An ID with no entry behind it passes through resolution unchanged and
		// is described by ID alone; only a genuinely ambiguous ID errors here.
		if entry.Closes, err = snapshot.graph.ResolveIDs(entry.Closes); err != nil {
			return nil, fmt.Errorf("resolving closes: %w", err)
		}
		if entry.Supersedes, err = snapshot.graph.ResolveIDs(entry.Supersedes); err != nil {
			return nil, fmt.Errorf("resolving supersedes: %w", err)
		}
		closureTargets = snapshot.graph.ClosureTargets(entry)
	}

	finder := finders.New(finders.Options{WritingGuideRunner: runtimeLLMRunner{executor: runtime.options.LLM, purpose: "writing-guide"}})
	guideCtx, cancel := context.WithTimeout(ctx, runtime.options.LLMTimeout)
	defer cancel()
	result, err := finder.WritingGuide(guideCtx, snapshot.graph, query.WritingGuideQuery{Entry: entry, ClosureTargets: closureTargets})
	if err != nil {
		return nil, fmt.Errorf("writing guide: %w", err)
	}
	findings := make([]GuideFinding, 0, len(result.Findings))
	for _, f := range result.Findings {
		findings = append(findings, GuideFinding{Reasoning: f.Reasoning, Axis: f.Axis, Quote: f.Quote, Repair: f.Repair, Severity: string(f.Severity)})
	}
	return findings, nil
}
