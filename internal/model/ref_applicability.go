package model

// Ref-kind applicability matrix (plan d-tac-tph AC 4).
//
// The applicable/inapplicable determination for a ref kind is a lookup over
// the target's kind and derived status — not a judgment. Declaring it here
// makes it the single source for three consumers: the mechanical pre-flight
// check (an inapplicable chosen kind is a deterministic high finding, no LLM
// call), the per-ref admissible-kind lines rendered into the pre-flight user
// prompt, and the rubric guidance text. The LLM's remaining ref-kind job is
// the genuinely semantic part: kind-vs-body fit among admissible kinds, and
// desc-vs-body contradiction.
//
// The matrix is deliberately permissive at the edges: a cell goes inapplicable
// only when the vocabulary documents the combination as impossible (refines
// needs a live target to sharpen; a terminal done records completed work and
// cannot be "addressed"). Everything defensible — including combinations the
// vocabulary frames as tie-breaks or unusual — stays applicable with a note,
// because a mechanical high that misfires is worse than an advisory one: it
// blocks reproducibly. Live-graph usage backs this: builds-on refs to active
// and open targets exist as accepted captures, so that cell is soft.

// RefTargetClass reduces a ref target's kind and derived status to the axes
// the applicability matrix keys on. Status is classified before kind so a
// retired entry (closed, superseded, cascade-closed, orphan) is TargetRetired
// regardless of its kind — including the rare corrective-done-closed done.
type RefTargetClass string

const (
	TargetLiveDecision RefTargetClass = "live-decision" // active decision (any decision kind)
	TargetLiveSignal   RefTargetClass = "live-signal"   // open signal (gap, fact, question, insight, actor, annotation)
	TargetTerminalDone RefTargetClass = "terminal-done" // done signal — terminal fact of execution, no lifecycle
	TargetRetired      RefTargetClass = "retired"       // closed, superseded, cascade-closed, or orphan
)

// ClassifyRefTarget maps a target entry plus its derived status to the matrix
// axis. Callers pass the status from Graph.DerivedStatus so classification and
// rendering agree on liveness.
func ClassifyRefTarget(target *Entry, status Status) RefTargetClass {
	switch status.Kind {
	case StatusClosedBy, StatusSupersededBy, StatusCascadeClosedBy, StatusCascadeOrphan:
		return TargetRetired
	}
	if target.Type == TypeSignal && target.Kind == KindDone {
		return TargetTerminalDone
	}
	if target.Type == TypeDecision {
		return TargetLiveDecision
	}
	return TargetLiveSignal
}

// RefKindCell is one matrix cell: whether the kind's precondition holds for
// the target class, plus the nuance note. For applicable cells the note
// carries the reading under which the kind fits (rendered into the pre-flight
// prompt); for inapplicable cells it names the violated precondition and the
// admissible alternatives (rendered into the mechanical finding).
type RefKindCell struct {
	Applicable bool
	Note       string
}

// refKindMatrix declares every (ref kind × target class) cell. Keep the notes
// aligned with the canonical vocabulary in references/ref-kinds.md — the
// definitions live there; the notes here carry only the per-cell reading.
var refKindMatrix = map[RefKind]map[RefTargetClass]RefKindCell{
	RefKindGroundedIn: {
		TargetLiveDecision: {Applicable: true, Note: "a basis conformed to or reasoned from — contract, aspiration, standing directive, or prior decision"},
		TargetLiveSignal:   {Applicable: true, Note: "a fact taken as premise or an insight reasoned from"},
		TargetTerminalDone: {Applicable: true, Note: "reasoning from the done's outcome as empirical basis; the tie-break with builds-on is the author's call"},
		TargetRetired:      {Applicable: true, Note: "a retired entry can still be the basis the source reasons from"},
	},
	RefKindBuildsOn: {
		TargetLiveDecision: {Applicable: true, Note: "only as the forward next-step reading; a live decision the body sharpens in place is refines instead"},
		TargetLiveSignal:   {Applicable: true, Note: "only as the forward next-step after that line of observation; merely reasoning from an open signal is grounded-in"},
		TargetTerminalDone: {Applicable: true, Note: "extending a finished chain; defensible alongside grounded-in — the tie-break is the author's call"},
		TargetRetired:      {Applicable: true, Note: "extending a closed line of work — the kind's home case"},
	},
	RefKindRefines: {
		TargetLiveDecision: {Applicable: true, Note: "sharpens an active commitment in place — the augmenting pattern; the refining entry closes alongside the target"},
		TargetLiveSignal:   {Applicable: true, Note: "unusual — narrowing a signal is normally supersession; in-place sharpening of an open signal is rare but not impossible"},
		TargetTerminalDone: {Applicable: false, Note: "refines requires an active target sharpened in place — a done signal is terminal, there is nothing active to sharpen"},
		TargetRetired:      {Applicable: false, Note: "refines requires an active target sharpened in place — for a closed or superseded target, builds-on extends it instead"},
	},
	RefKindAddresses: {
		TargetLiveDecision: {Applicable: true, Note: "realizing the decision's commitment — operationalizing a directive, supplying a plan's AC or an activity's work, including partially"},
		TargetLiveSignal:   {Applicable: true, Note: "responding to an open gap, question, or insight; a fact is normally a premise (grounded-in) unless the body acts on a concern the fact carries"},
		TargetTerminalDone: {Applicable: false, Note: "a done records completed work — not a gap, question, insight, or commitment, so it cannot be addressed; the next step after it is builds-on, reasoning from it is grounded-in"},
		TargetRetired:      {Applicable: true, Note: "the target is already retired — under concurrent capture this can be a stale ref; if the body merely reasons from it, grounded-in"},
	},
	RefKindSurfaces: {
		TargetLiveDecision: {Applicable: true, Note: "doing this entry's work created or discovered the target"},
		TargetLiveSignal:   {Applicable: true, Note: "doing this entry's work created or discovered the target"},
		TargetTerminalDone: {Applicable: true, Note: "doing this entry's work created or discovered the target"},
		TargetRetired:      {Applicable: true, Note: "doing this entry's work created or discovered the target, which has since been retired"},
	},
	RefKindDependsOn: {
		TargetLiveDecision: {Applicable: true, Note: "gated on the target landing or holding first"},
		TargetLiveSignal:   {Applicable: true, Note: "gated on the target landing or holding first"},
		TargetTerminalDone: {Applicable: true, Note: "the prerequisite has already landed; if the body cites it as basis rather than gate, grounded-in reads sharper"},
		TargetRetired:      {Applicable: true, Note: "the prerequisite has already resolved; if the body cites it as basis rather than gate, grounded-in reads sharper"},
	},
	RefKindRequiredBy: {
		TargetLiveDecision: {Applicable: true, Note: "forward class — this entry is what the target was waiting on, recorded from the prerequisite's side"},
		TargetLiveSignal:   {Applicable: true, Note: "forward class — this entry is what the target was waiting on, recorded from the prerequisite's side"},
		TargetTerminalDone: {Applicable: true, Note: "forward class — this entry is what the target's work was waiting on, recorded from the prerequisite's side"},
		TargetRetired:      {Applicable: true, Note: "forward class — this entry is what the target was waiting on, recorded from the prerequisite's side"},
	},
	RefKindRelated: {
		TargetLiveDecision: {Applicable: true, Note: "the floor — a genuine sibling or context the source accounts for; never a default"},
		TargetLiveSignal:   {Applicable: true, Note: "the floor — a genuine sibling or context the source accounts for; never a default"},
		TargetTerminalDone: {Applicable: true, Note: "the floor — a genuine sibling or context the source accounts for; never a default"},
		TargetRetired:      {Applicable: true, Note: "the floor — a genuine sibling or context the source accounts for; never a default"},
	},
}

// RefKindApplicability returns the matrix cell for a capturable kind against
// a target class. The second return is false for kinds outside the capturable
// vocabulary (unknown, legacy aliases) — those are rejected by the separate
// capturable-kind mechanical check, not classified here.
func RefKindApplicability(kind RefKind, class RefTargetClass) (RefKindCell, bool) {
	row, ok := refKindMatrix[kind]
	if !ok {
		return RefKindCell{}, false
	}
	cell, ok := row[class]
	return cell, ok
}

// AdmissibleRefKinds returns the capturable kinds whose precondition holds for
// the target class, in display order. This is the set rendered into the
// pre-flight prompt per ref and the set a mechanical inapplicability finding
// names as alternatives.
func AdmissibleRefKinds(class RefTargetClass) []RefKind {
	var out []RefKind
	for _, k := range RefKindValues() {
		if cell, ok := RefKindApplicability(k, class); ok && cell.Applicable {
			out = append(out, k)
		}
	}
	return out
}
