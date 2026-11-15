package application

import (
	"fmt"
	"path/filepath"

	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
)

// The entry-less graph writes of the base procedures — a summary replaced, a
// WIP marker created or removed — run like capture: identities and the
// precondition are allocated before the intent, the write publishes once under
// the intent's key, and the command reports what it left (d-tac-n47,
// d-tac-wgw, d-tac-7mh).

// documentPublicationKey is a document write's storage identity: this
// session, the intent's position and the command with its resource.
func (w *WorkflowSession) documentPublicationKey(intent *engine.MutationIntent, resource string) PublicationKey {
	return PublicationKey{Session: w.ID(), Sequence: intent.Ref, Discriminator: intent.Command + ":" + resource}
}

func intentTarget(intent *engine.MutationIntent) MutationTarget {
	return MutationTarget{Project: ProjectID(intent.Values["project"]), Branch: intent.Values["branch"]}
}

// prepareWorkflowReplaceSummary records the entry, its target and the bytes
// the correction was read from, so the write conditions on that document.
func (w *WorkflowSession) prepareWorkflowReplaceSummary(ctx *engine.Context) (map[string]string, error) {
	id, ok := workflowStoreString(ctx.Store, "entryId")
	if !ok {
		return nil, fmt.Errorf("replaceSummary: entryId is not set")
	}
	_, hasSummary := workflowStoreString(ctx.Store, "correctedSummary")
	_, hasTitle := workflowStoreString(ctx.Store, "correctedTitle")
	if !hasSummary && !hasTitle {
		return nil, fmt.Errorf("replaceSummary: neither correctedSummary nor correctedTitle is set")
	}
	target, fromBinding := w.effectiveTargetFor(w.instanceProject(ctx.Instance), ctx.Store)
	if err := w.authorizeTarget(target.Project, AccessWrite); err != nil {
		return nil, err
	}
	logicalPath, err := model.IDToRelPath(id)
	if err != nil {
		return nil, err
	}
	current, err := w.app.readDocument(w.ctx, w.identity, target, filepath.ToSlash(logicalPath))
	if err != nil {
		return nil, w.withSessionBindingTargetError(err, fromBinding)
	}
	if current.Absent {
		return nil, fmt.Errorf("replaceSummary: entry %s is not on the target", id)
	}
	return map[string]string{"entryId": id, "project": string(target.Project), "branch": target.Branch, "expectedBlob": GitBlobID(current.Content)}, nil
}

func (w *WorkflowSession) runWorkflowReplaceSummary(ctx *engine.Context) error {
	summary, _ := workflowStoreString(ctx.Store, "correctedSummary")
	title, _ := workflowStoreString(ctx.Store, "correctedTitle")
	intent := ctx.Intent
	_, err := w.app.ReplaceSummary(w.ctx, w.identity, w.instanceProject(ctx.Instance), w.binding, SummaryReplacement{
		Target: intentTarget(intent), Publication: w.documentPublicationKey(intent, intent.Values["entryId"]),
		EntryID: intent.Values["entryId"], ExpectedBlob: intent.Values["expectedBlob"], Summary: summary, Title: title,
	})
	return err
}

// reportWorkflowReplaceSummaryEffects reports the entry as replaced when the
// intent's key published, unchanged when it did not, unknown when the store
// cannot answer.
func (w *WorkflowSession) reportWorkflowReplaceSummaryEffects(ctx *engine.Context) ([]engine.Effect, error) {
	intent := ctx.Intent
	if intent == nil {
		return nil, fmt.Errorf("replaceSummary effects require a recorded invocation")
	}
	entryID := intent.Values["entryId"]
	logicalPath, err := model.IDToRelPath(entryID)
	if err != nil {
		return nil, err
	}
	state := "unknown"
	_, exists, err := w.app.lookupDocumentPublication(w.ctx, w.identity, intentTarget(intent), w.documentPublicationKey(intent, entryID), filepath.ToSlash(logicalPath))
	switch {
	case err != nil:
		slogutils.FromContext(w.ctx).Info("publication lookup for summary replacement failed", "entry", entryID, "error", err)
	case exists:
		state = "replaced"
	default:
		state = "unchanged"
	}
	return []engine.Effect{{Kind: "entry", ID: entryID, State: state}}, nil
}

// prepareWorkflowWIPStart allocates the marker's identity before the intent.
func (w *WorkflowSession) prepareWorkflowWIPStart(ctx *engine.Context) (map[string]string, error) {
	anchor, ok := workflowStoreString(ctx.Store, "anchor")
	if !ok {
		return nil, fmt.Errorf("wipStart: anchor is not set")
	}
	values, err := w.wipTarget(ctx)
	if err != nil {
		return nil, err
	}
	marker, err := w.app.WIPMarkerID(w.ctx, w.identity, ProjectID(values["project"]))
	if err != nil {
		return nil, err
	}
	values["markerId"], values["anchor"] = marker, anchor
	return values, nil
}

func (w *WorkflowSession) runWorkflowWIPStart(ctx *engine.Context) error {
	description, _ := workflowStoreString(ctx.Store, "wipDescription")
	intent := ctx.Intent
	marker := intent.Values["markerId"]
	_, err := w.app.StartWIP(w.ctx, w.identity, w.instanceProject(ctx.Instance), w.binding, WIPMarkerWrite{
		Target: intentTarget(intent), Publication: w.documentPublicationKey(intent, marker),
		MarkerID: marker, EntryID: intent.Values["anchor"], Description: description,
	})
	if err != nil {
		return withTargetRemedy(intent, err)
	}
	return ctx.Store.WriteEngine("wipMarker", marker)
}

// prepareWorkflowWIPDone records the marker the implementation run removes.
func (w *WorkflowSession) prepareWorkflowWIPDone(ctx *engine.Context) (map[string]string, error) {
	marker, ok := workflowStoreString(ctx.Store, "wipMarker")
	if !ok {
		return nil, fmt.Errorf("wipDone: wipMarker is not set")
	}
	values, err := w.wipTarget(ctx)
	if err != nil {
		return nil, err
	}
	values["markerId"] = marker
	return values, nil
}

func (w *WorkflowSession) runWorkflowWIPDone(ctx *engine.Context) error {
	if err := w.removeWIPMarker(ctx); err != nil {
		return err
	}
	return ctx.Store.WriteEngine("wipMarker", nil)
}

// prepareWorkflowWIPRemove records the stale marker groom removes; its target
// is the instance project's default branch.
func (w *WorkflowSession) prepareWorkflowWIPRemove(ctx *engine.Context) (map[string]string, error) {
	marker, ok := workflowStoreString(ctx.Store, "staleMarker")
	if !ok {
		return nil, fmt.Errorf("wipRemove: staleMarker is not set")
	}
	project := w.instanceProject(ctx.Instance)
	if err := w.authorizeTarget(project, AccessWrite); err != nil {
		return nil, err
	}
	return map[string]string{"markerId": marker, "project": string(project), "branch": ""}, nil
}

func (w *WorkflowSession) runWorkflowWIPRemove(ctx *engine.Context) error {
	return w.removeWIPMarker(ctx)
}

func (w *WorkflowSession) removeWIPMarker(ctx *engine.Context) error {
	intent := ctx.Intent
	marker := intent.Values["markerId"]
	_, err := w.app.FinishWIP(w.ctx, w.identity, w.instanceProject(ctx.Instance), w.binding, intentTarget(intent), w.documentPublicationKey(intent, marker), marker)
	return withTargetRemedy(intent, err)
}

// reportWorkflowWIPEffects reports the marker as present or absent on the
// target after the intent, reading it live; unknown when the store cannot answer.
func (w *WorkflowSession) reportWorkflowWIPEffects(ctx *engine.Context) ([]engine.Effect, error) {
	intent := ctx.Intent
	if intent == nil {
		return nil, fmt.Errorf("WIP effects require a recorded invocation")
	}
	marker := intent.Values["markerId"]
	state := "unknown"
	current, err := w.app.readDocument(w.ctx, w.identity, intentTarget(intent), filepath.ToSlash(model.WIPMarkerPath(marker)))
	switch {
	case err != nil:
		slogutils.FromContext(w.ctx).Info("WIP marker read for effects report failed", "marker", marker, "error", err)
	case current.Absent:
		state = "absent"
	default:
		state = "present"
	}
	return []engine.Effect{{Kind: "wip-marker", ID: marker, State: state}}, nil
}
