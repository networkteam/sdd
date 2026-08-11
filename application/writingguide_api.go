package application

import (
	"context"
	"fmt"
	"time"

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
// pre-playback half of the capture-guide architecture (d-cpt-20r). Only the
// draft's own fields feed the prompt; refs stay unresolved and no graph
// context is assembled, because the guide's scope is the entry as a stranger
// reads it. Findings are drafting input for the dialogue, never a gate.
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
	}
	for _, ref := range draft.Refs {
		entry.Refs = append(entry.Refs, model.Ref{ID: ref.ID, Kind: model.RefKind(ref.Kind), Desc: ref.Desc})
	}

	finder := finders.New(finders.Options{WritingGuideRunner: runtimeLLMRunner{executor: runtime.options.LLM, purpose: "writing-guide"}})
	guideCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	result, err := finder.WritingGuide(guideCtx, query.WritingGuideQuery{Entry: entry})
	if err != nil {
		return nil, fmt.Errorf("writing guide: %w", err)
	}
	findings := make([]GuideFinding, 0, len(result.Findings))
	for _, f := range result.Findings {
		findings = append(findings, GuideFinding{Reasoning: f.Reasoning, Axis: f.Axis, Quote: f.Quote, Repair: f.Repair, Severity: string(f.Severity)})
	}
	return findings, nil
}
