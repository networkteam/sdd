package engine

import (
	"regexp"
	"strings"

	"github.com/networkteam/sdd/internal/serveview"
)

// The authoring arithmetic (d-tac-rzi): a pure, advisory worst-case sizing of
// every automatic serve a spec can produce, checked against the engine's
// default budget at authoring surfaces — pre-flight on procedure capture and
// `sdd lint` on specs already in a graph. Never called by ParseSpec, Validate,
// or LoadSpec: overshoot is a risk, not a defect, and the spec still runs.

// QueryResolver resolves a query's registration by name — what the arithmetic
// sizes inject caps against. *Registry implements it. A resolver miss means
// the spec names an unknown query, which Validate rejects before the
// arithmetic runs; the arithmetic skips such injects.
type QueryResolver interface {
	Query(name string) (*Query, bool)
}

// StepSize is one step's worst-case automatic serve weight in bytes.
type StepSize struct {
	Step  string
	Bytes int
}

// EffectiveTotal is the total the arithmetic checks against: the spec's
// declared serveBudget when set — the author's recorded trade — else the
// budget's default Total.
func (s *Spec) EffectiveTotal(budget serveview.Budget) int {
	if s.ServeBudget > 0 {
		return s.ServeBudget
	}
	return budget.Total
}

// OverBudget returns the steps whose worst case exceeds the effective total.
func (s *Spec) OverBudget(budget serveview.Budget, resolver QueryResolver) []StepSize {
	total := s.EffectiveTotal(budget)
	var over []StepSize
	for _, size := range s.WorstCaseServeBytes(budget, resolver) {
		if size.Bytes > total {
			over = append(over, size)
		}
	}
	return over
}

// WorstCaseServeBytes sizes every step's automatic serve at its declared
// worst: the unit's lane skeleton (authored template text as written), each
// inject at its effective cap, each referenced declared field at its typed
// slot allowance, and the draft lane's cap when the step serves a delta.
// Individually capped parts can still sum past a host budget — this is the
// check that moved from runtime to authoring surfaces.
func (s *Spec) WorstCaseServeBytes(budget serveview.Budget, resolver QueryResolver) []StepSize {
	sizes := make([]StepSize, 0, len(s.Steps))
	for _, step := range s.Steps {
		sizes = append(sizes, StepSize{Step: step.ID, Bytes: s.worstCaseStep(step, budget, resolver)})
	}
	return sizes
}

func (s *Spec) worstCaseStep(step *Step, budget serveview.Budget, resolver QueryResolver) int {
	total := 0
	unitName := step.ID
	if step.Render != "" {
		unitName = step.Render
	}
	unit, hasUnit := s.Units[unitName]
	if hasUnit {
		for _, lane := range unit.Lanes {
			total += len(lane.Text)
		}
		total += s.slotAllowances(unit, budget)
	}
	for _, inj := range step.Inject {
		total += injectAllowance(inj, budget, resolver)
	}
	if len(step.ServeDelta) > 0 {
		total += budget.Cap(serveview.PartDraft).MaxBytes
	}
	return total
}

// injectAllowance sizes one inject at its effective cap: declared bytes win,
// then declared items at the part's allowance, then the registration bound,
// then the budget default for the query's part kind.
func injectAllowance(inj InjectCall, budget serveview.Budget, resolver QueryResolver) int {
	part := serveview.PartText
	registered := serveview.Cap{}
	if resolver != nil {
		if q, ok := resolver.Query(inj.Fn); ok {
			registered = q.Bound.Cap
			if q.Bound.Part != "" {
				part = q.Bound.Part
			}
		}
	}
	cap := serveview.Effective(inj.Cap, registered)
	if cap.Zero() {
		cap = budget.Cap(part)
	}
	if cap.MaxBytes > 0 {
		return cap.MaxBytes
	}
	if cap.MaxItems > 0 {
		return cap.MaxItems * itemAllowance(part, budget)
	}
	// An unbounded part under a budget with no default for its kind: size it
	// as producer-rendered text so the gap surfaces instead of counting zero.
	return budget.Cap(serveview.PartText).MaxBytes
}

// itemAllowance is the per-item byte weight for a count-capped part: the
// part's own byte/item ratio when its budget declares both, else the measured
// default row allowance.
func itemAllowance(part serveview.PartKind, budget serveview.Budget) int {
	if cap := budget.Cap(part); cap.MaxBytes > 0 && cap.MaxItems > 0 {
		return cap.MaxBytes / cap.MaxItems
	}
	return serveview.ItemAllowanceBytes
}

// slotAllowances sizes the declared fields a unit's lanes interpolate: text
// and prose at the store-value cap the serve seam enforces, lists at the same
// allowance (they render as compact rows), and small scalars at a nominal
// weight. Engine-written template values carry no declaration and are sized
// by the harness, not here.
func (s *Spec) slotAllowances(unit Unit, budget serveview.Budget) int {
	text := make([]string, 0, len(unit.Lanes))
	for _, lane := range unit.Lanes {
		text = append(text, lane.Text)
	}
	joined := strings.Join(text, "\n")
	total := 0
	for name, decl := range s.Params {
		if referencesField(joined, name) {
			total += slotAllowance(decl, budget)
		}
	}
	for name, decl := range s.State {
		if referencesField(joined, name) {
			total += slotAllowance(decl, budget)
		}
	}
	return total
}

const scalarSlotAllowanceBytes = 128

func slotAllowance(decl VarDecl, budget serveview.Budget) int {
	if decl.Type.List || decl.Type.Base == TypeText || decl.Type.Base == TypeProse {
		return budget.Cap(serveview.PartStoreValue).MaxBytes
	}
	return scalarSlotAllowanceBytes
}

// referencesField reports whether a unit's template text mentions a declared
// field. A conservative word-boundary match — false positives inflate the
// advisory worst case, never break a spec.
func referencesField(text, name string) bool {
	re, err := regexp.Compile(`\.` + regexp.QuoteMeta(name) + `\b`)
	if err != nil {
		return strings.Contains(text, "."+name)
	}
	return re.MatchString(text)
}
