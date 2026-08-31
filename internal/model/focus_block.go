package model

// ShapeFocusBlock is the result shape produced by `expand(involvement)`
// over a list of focus entries, terminated by `as-focus-block`. It
// carries one entry per active focus, each with its involvement targets
// resolved to graph entries and tagged with derived state.
const ShapeFocusBlock RenderShape = "focus-block"

// FocusState classifies an involvement target's engagement level for
// the focus-block render. Slice 7 derives state from the resolved
// actors set and the target's heat under the configured rank, per the
// algorithm in d-tac-uww §6.
type FocusState string

const (
	// FocusStatePullAvailable — target's resolved actors set is empty,
	// either because the involvement explicitly declared `actors: []`
	// or the focus-level default is empty. The target is in scope but
	// awaiting pickup.
	FocusStatePullAvailable FocusState = "pull-available"
	// FocusStateStalled — target has actors assigned but its heat under
	// the section's rank is below the configured stalled threshold.
	// Signal that the work is dormant despite being attributed.
	FocusStateStalled FocusState = "stalled"
	// FocusStateDriving — target has actors and is engaged (heat above
	// the stalled threshold). The default expectation for declared work.
	FocusStateDriving FocusState = "driving"
)

// FocusBlock is a list of FocusGroup entries, one per focus decision
// in the input, in the order they appeared. Each FocusGroup carries
// its target rows already filtered for omission (closed/superseded
// targets dropped per design §6) and ordered as listed in the focus's
// involvement frontmatter.
type FocusBlock struct {
	Focuses []FocusGroup
	// Dropped counts whole units a serve budget kept out of this section;
	// Pull is the runnable layout expression for the complete section.
	// Zero/empty on explicit pulls, which are never cut (d-tac-rzi).
	Dropped int
	Pull    string
}

// FocusGroup is one focus decision plus its resolved involvement targets.
// Defaults (Actors, When) are carried explicitly so the renderer can show
// "inherited from focus" cleanly without re-resolving per target.
type FocusGroup struct {
	Focus   *Entry
	Actors  []string   // focus-level default actors (canonical-only)
	When    *FocusWhen // focus-level default temporal scope
	Targets []FocusTarget
}

// FocusTarget is one (focus, involvement-triple) row with the target
// entry resolved and per-row attributes computed. Score carries the
// rank-time heat (when computed) so the renderer can render
// `{score: X.XXX}` consistently with the as-list scored output. State
// is derived from Score and Actors per FocusState semantics.
type FocusTarget struct {
	Target         *Entry
	ResolvedActors []string   // per-involvement override or focus default; empty = pull-available
	ActorsExplicit bool       // actors came from the involvement triple, not the focus default
	ResolvedWhen   *FocusWhen // per-involvement override or focus default
	Score          float64    // heat under the section's rank (0 if unranked)
	State          FocusState
}

// Shape implements SectionData.
func (FocusBlock) Shape() RenderShape { return ShapeFocusBlock }

// Count implements SectionData: the number of focus groups produced.
func (f FocusBlock) Count() int { return len(f.Focuses) }
