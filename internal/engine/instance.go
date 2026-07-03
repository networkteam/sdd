package engine

import (
	"fmt"
	"sort"
	"strings"
	"text/template"
)

// InstanceStatus is a procedure instance's lifecycle state.
type InstanceStatus string

const (
	StatusRunning   InstanceStatus = "running"
	StatusCompleted InstanceStatus = "completed"
	StatusAbandoned InstanceStatus = "abandoned"
)

// Instance is one running procedure — a spec, its typed store, and a step
// position. Instances belong to a session; sub-procedures carry a parent
// link.
type Instance struct {
	ID     string
	Spec   *Spec
	Store  *Store
	Step   string
	Status InstanceStatus
	// Outcome is the end(...) label the instance finished with.
	Outcome string
	// Parent is the spawning instance's ID for sub-procedures, empty at the
	// top level.
	Parent string
	// opDone marks the current step's op as executed for this arrival, so a
	// stalled gate re-evaluated by a later report doesn't re-run its side
	// effect. Reset on every transition.
	opDone bool
}

// currentStep returns the instance's step definition, nil when terminal.
func (i *Instance) currentStep() *Step {
	if i.Status != StatusRunning {
		return nil
	}
	return i.Spec.StepByID[i.Step]
}

// FailedPredicate names a predicate that held a gate, with its registered
// failure message — the message becomes the served instruction.
type FailedPredicate struct {
	Name    string
	Message string
}

// ChooserOption is one selectable choice in a served chooser.
type ChooserOption struct {
	Choice  string
	Collect []CollectField
}

// ChooserServe is the pending-chooser part of a serve: who answers and what
// the options are.
type ChooserServe struct {
	Kind    ChooserKind
	Options []ChooserOption
}

// Serve is what the engine returns after every advance — the current step's
// rendered instructions, the report schema, chooser material when one is
// pending, and stall diagnostics naming exactly what's missing.
type Serve struct {
	Instance  string
	Procedure string
	Status    InstanceStatus
	Outcome   string
	Step      string
	// Goal is a one-line statement of what advances the instance from here.
	Goal string
	// Instructions is the step's instruction unit rendered against the
	// store, with stall diagnostics appended when a gate is held.
	Instructions string
	// Unit names the instruction unit rendered into Instructions (empty when
	// the step has none). Shells key their served-instruction memory on it.
	Unit string
	// UnitText is the rendered unit alone, without diagnostics — what a
	// shell replaces with a one-line reminder once the unit was served.
	UnitText string
	// Diagnostics are the stall messages appended to Instructions ("Gate
	// held" lines), kept separate so shells can recompose around UnitText.
	Diagnostics []string
	// Missing names the current step's required collect fields not yet in
	// the store.
	Missing []string
	// Failing lists predicates currently holding the gate.
	Failing []FailedPredicate
	// ReportSchema is the JSON Schema for the current step's report.
	ReportSchema map[string]any
	// Chooser is set when the step awaits an agent or user choice.
	Chooser *ChooserServe
	// Produced carries the engine-written results on completion (e.g. the
	// created entry ID), excluding internal trust machinery.
	Produced map[string]any
}

// evalGuard evaluates a guard against the registry, returning the verdict
// and the failing predicates for diagnostics.
func (s *Session) evalGuard(inst *Instance, g *GuardExpr) (bool, []FailedPredicate, error) {
	ctx := &Context{Store: inst.Store, Graph: s.engine.Graph, Step: inst.Step}
	ok, err := g.Eval(func(name string) (bool, error) {
		p, found := s.engine.Registry.Predicate(name)
		if !found {
			return false, fmt.Errorf("unknown predicate %q", name)
		}
		return p.Fn(ctx)
	})
	if err != nil {
		return false, nil, err
	}
	if ok {
		return true, nil, nil
	}
	// Full evaluation for diagnostics: name every referenced predicate that
	// is currently false (predicates are pure, so re-evaluation is safe).
	var failing []FailedPredicate
	for _, name := range g.Predicates() {
		p, found := s.engine.Registry.Predicate(name)
		if !found {
			continue
		}
		v, err := p.Fn(ctx)
		if err != nil {
			return false, nil, err
		}
		if !v {
			failing = append(failing, FailedPredicate{Name: name, Message: p.FailMessage})
		}
	}
	return false, failing, nil
}

// cascade advances gate steps until something stops them — a failing
// predicate, a missing field (no transition fires), or a chooser that
// belongs to the agent or the user. Ops run once per arrival, before the
// step's transitions are considered.
func (s *Session) cascade(inst *Instance) error {
	for inst.Status == StatusRunning {
		step := inst.currentStep()
		if step == nil {
			return fmt.Errorf("instance %s: step %q not found", inst.ID, inst.Step)
		}
		if step.Chooser != ChooserGate {
			return nil // pending chooser — agent or user judgment
		}

		if step.Guard != nil {
			ok, failing, err := s.evalGuard(inst, step.Guard)
			if err != nil {
				return err
			}
			if !ok {
				// Confirmation staleness: when the gate is held by
				// playbackConfirmed and a stale confirmation record exists,
				// reopen the chooser that recorded it — the
				// edit-after-confirm rule, owned entirely by the
				// confirmPlayback/playbackConfirmed pair.
				if reopened, err := s.reopenStalePlayback(inst, failing); err != nil {
					return err
				} else if reopened {
					continue
				}
				return nil // stalled: guard holds the gate
			}
		}

		if step.Op != "" && !inst.opDone {
			if err := s.runCommand(inst, step.Op); err != nil {
				return err
			}
			inst.opDone = true
		}

		fired := false
		for _, t := range step.Transitions {
			if t.Otherwise {
				if err := s.transitionTo(inst, t.To, false); err != nil {
					return err
				}
				fired = true
				break
			}
			ok, _, err := s.evalGuard(inst, t.When)
			if err != nil {
				return err
			}
			if ok {
				if err := s.transitionTo(inst, t.To, false); err != nil {
					return err
				}
				fired = true
				break
			}
		}
		if !fired {
			return nil // stalled: the serve names what's missing
		}
	}
	return nil
}

// reopenStalePlayback implements edit-after-confirm: if the failing
// predicates include playbackConfirmed and the store holds a confirmation
// whose snapshot no longer matches, the confirmation is cleared and the
// instance returns to the chooser step that recorded it.
func (s *Session) reopenStalePlayback(inst *Instance, failing []FailedPredicate) (bool, error) {
	holdsOnConfirmation := false
	for _, f := range failing {
		if f.Name == "playbackConfirmed" {
			holdsOnConfirmation = true
			break
		}
	}
	if !holdsOnConfirmation {
		return false, nil
	}
	v, ok := inst.Store.Get(fieldPlaybackConfirmation)
	if !ok {
		return false, nil // never confirmed — a plain stall, not staleness
	}
	conf, ok := asConfirmation(v)
	if !ok || conf.Step == "" || inst.Spec.StepByID[conf.Step] == nil {
		return false, nil
	}
	if conf.Snapshot == inst.Store.StateSnapshot() {
		return false, nil // confirmation is current; the gate is held by something else
	}
	inst.Store.WriteEngine(fieldPlaybackConfirmation, nil)
	s.appendEvent(inst.ID, EventOpResult, map[string]any{
		"step": inst.Step,
		"fn":   "reopenPlayback",
		"writes": map[string]any{
			fieldPlaybackConfirmation: nil,
		},
	})
	return true, s.transitionTo(inst, conf.Step, true)
}

// runCommand executes a registry command at the instance's current step and
// logs its engine writes as an op_result event.
func (s *Session) runCommand(inst *Instance, name string) error {
	cmd, ok := s.engine.Registry.Command(name)
	if !ok {
		return fmt.Errorf("instance %s: %q is not a registered command", inst.ID, name)
	}
	ctx := &Context{Store: inst.Store, Graph: s.engine.Graph, Step: inst.Step}
	inst.Store.beginJournal()
	err := cmd.Fn(ctx)
	writes := inst.Store.drainJournal()
	if err != nil {
		return fmt.Errorf("command %q at step %s: %w", name, inst.Step, err)
	}
	s.appendEvent(inst.ID, EventOpResult, map[string]any{
		"step":   inst.Step,
		"fn":     name,
		"writes": writes,
	})
	return nil
}

// transitionTo moves the instance to a step or terminal target and logs the
// transition. The reopen flag marks the edit-after-confirm return to a
// chooser.
func (s *Session) transitionTo(inst *Instance, to string, reopen bool) error {
	from := inst.Step
	data := map[string]any{"from": from, "to": to}
	if reopen {
		data["reopen"] = true
	}
	s.appendEvent(inst.ID, EventTransition, data)

	switch to {
	case EndCompleted:
		inst.Status = StatusCompleted
		inst.Outcome = "completed"
		s.appendEvent(inst.ID, EventCompleted, map[string]any{
			"produced": inst.produced(),
		})
	case EndAbandoned:
		inst.Status = StatusAbandoned
		inst.Outcome = "abandoned"
		s.appendEvent(inst.ID, EventAbandoned, map[string]any{
			"reason": "procedure exit",
		})
	default:
		if inst.Spec.StepByID[to] == nil {
			return fmt.Errorf("instance %s: transition target %q not found", inst.ID, to)
		}
		inst.Step = to
		inst.opDone = false
	}
	return nil
}

// produced returns the engine-written values worth surfacing on completion,
// excluding the internal trust machinery fields.
func (i *Instance) produced() map[string]any {
	out := make(map[string]any)
	for name, ev := range i.Store.Export() {
		if ev.Provenance != ProvenanceEngine {
			continue
		}
		if name == fieldPlaybackConfirmation || name == fieldPreflightOverride {
			continue
		}
		if ev.Value == nil {
			continue
		}
		out[name] = ev.Value
	}
	return out
}

// serve builds the Serve for the instance's current position: rendered
// instructions, report schema, chooser material, and stall diagnostics.
func (s *Session) serve(inst *Instance) (*Serve, error) {
	sv := &Serve{
		Instance:  inst.ID,
		Procedure: inst.Spec.Canonical,
		Status:    inst.Status,
		Outcome:   inst.Outcome,
	}
	if inst.Status != StatusRunning {
		sv.Produced = inst.produced()
		sv.Goal = "the procedure has ended (" + inst.Outcome + ")"
		return sv, nil
	}

	step := inst.currentStep()
	if step == nil {
		return nil, fmt.Errorf("instance %s: step %q not found", inst.ID, inst.Step)
	}
	sv.Step = step.ID
	sv.ReportSchema = inst.Spec.ReportSchemaForStep(step)
	sv.Missing = s.missingFields(inst, step)

	unitName := step.ID
	if step.Render != "" {
		unitName = step.Render
	}
	if _, ok := inst.Spec.Units[unitName]; ok {
		sv.Unit = unitName
	}
	instructions, err := s.renderUnit(inst, step)
	if err != nil {
		return nil, err
	}
	sv.UnitText = instructions

	var diagnostics []string
	switch step.Chooser {
	case ChooserGate:
		if step.Guard != nil {
			ok, failing, err := s.evalGuard(inst, step.Guard)
			if err != nil {
				return nil, err
			}
			if !ok {
				sv.Failing = failing
			}
		}
		if len(sv.Failing) == 0 && len(sv.Missing) == 0 && len(step.Transitions) > 0 {
			// The gate didn't cascade, its guard holds, and no collect field
			// is missing — the transitions' own predicates hold it. Name
			// those.
			sv.Failing = s.transitionDiagnostics(inst, step)
		}
		for _, f := range sv.Failing {
			msg := f.Message
			if msg == "" {
				msg = "predicate " + f.Name + " does not hold"
			}
			diagnostics = append(diagnostics, msg)
		}
		if len(sv.Missing) > 0 {
			diagnostics = append(diagnostics, "missing: "+strings.Join(sv.Missing, ", "))
		}
		sv.Goal = "provide the missing report fields"
		if len(sv.Failing) > 0 {
			sv.Goal = "resolve what holds the gate, then report again"
		}
	case ChooserAgent, ChooserUser:
		options := make([]ChooserOption, 0, len(step.Options))
		for _, o := range step.Options {
			options = append(options, ChooserOption{Choice: o.Choice, Collect: o.Collect})
		}
		sv.Chooser = &ChooserServe{Kind: step.Chooser, Options: options}
		if step.Chooser == ChooserUser {
			sv.Goal = "put the choice to the user and relay their answer verbatim"
		} else {
			sv.Goal = "judge and answer the chooser, with the evidence its option collects"
		}
	}
	if step.Goal != "" {
		sv.Goal = step.Goal
	}

	sv.Diagnostics = diagnostics
	sv.Instructions = ComposeInstructions(instructions, diagnostics)

	s.appendEvent(inst.ID, EventServed, map[string]any{"step": step.ID})
	return sv, nil
}

// ComposeInstructions joins a rendered unit (or a shell-substituted
// reminder) with stall diagnostics — the one composition rule for the
// Instructions field, shared by the engine's serve and shells recomposing
// around served-instruction memory.
func ComposeInstructions(unitText string, diagnostics []string) string {
	out := unitText
	if len(diagnostics) > 0 {
		if out != "" {
			out += "\n\n"
		}
		out += "Gate held:\n- " + strings.Join(diagnostics, "\n- ")
	}
	return out
}

// missingFields names the step's required collect fields not yet present —
// the "names exactly what's missing" part of the cascade rule.
func (s *Session) missingFields(inst *Instance, step *Step) []string {
	var missing []string
	for _, cf := range step.Collect {
		if cf.Optional {
			continue
		}
		if !inst.Store.Has(cf.Name) {
			missing = append(missing, cf.Name)
		}
	}
	sort.Strings(missing)
	return missing
}

// transitionDiagnostics evaluates the step's transition guards and returns
// the predicates that are false across them — why no transition fired.
func (s *Session) transitionDiagnostics(inst *Instance, step *Step) []FailedPredicate {
	seen := make(map[string]bool)
	var failing []FailedPredicate
	for _, t := range step.Transitions {
		if t.When == nil {
			continue
		}
		_, f, err := s.evalGuard(inst, t.When)
		if err != nil {
			continue
		}
		for _, fp := range f {
			if !seen[fp.Name] {
				seen[fp.Name] = true
				failing = append(failing, fp)
			}
		}
	}
	return failing
}

// renderUnit renders the step's instruction unit (its render override or the
// section named after the step) as a Go template over the store, with inject
// query results joined into the template context under each fn's name.
func (s *Session) renderUnit(inst *Instance, step *Step) (string, error) {
	unitName := step.ID
	if step.Render != "" {
		unitName = step.Render
	}
	unit, ok := inst.Spec.Units[unitName]
	if !ok {
		return "", nil // gates without prose serve diagnostics and schema only
	}

	tmplCtx := inst.Store.TemplateContext()
	for _, inj := range step.Inject {
		q, found := s.engine.Registry.Query(inj.Fn)
		if !found {
			return "", fmt.Errorf("step %s: inject fn %q is not a registered query", step.ID, inj.Fn)
		}
		args, err := s.renderInjectArgs(inst, inj.Args)
		if err != nil {
			return "", fmt.Errorf("step %s: inject %s: %w", step.ID, inj.Fn, err)
		}
		result, err := q.Fn(&Context{Store: inst.Store, Graph: s.engine.Graph, Step: inst.Step}, args)
		if err != nil {
			return "", fmt.Errorf("step %s: inject %s: %w", step.ID, inj.Fn, err)
		}
		tmplCtx[inj.Fn] = result
	}

	tmpl, err := template.New(unitName).Option("missingkey=zero").Parse(unit)
	if err != nil {
		return "", fmt.Errorf("unit %s: %w", unitName, err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, tmplCtx); err != nil {
		return "", fmt.Errorf("unit %s: %w", unitName, err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// renderInjectArgs renders string args as Go templates against the store
// (literals pass through untouched); non-string args pass through as
// literals.
func (s *Session) renderInjectArgs(inst *Instance, args map[string]any) (map[string]any, error) {
	if len(args) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(args))
	tmplCtx := inst.Store.TemplateContext()
	for name, v := range args {
		str, ok := v.(string)
		if !ok || !strings.Contains(str, "{{") {
			out[name] = v
			continue
		}
		tmpl, err := template.New(name).Option("missingkey=zero").Parse(str)
		if err != nil {
			return nil, fmt.Errorf("arg %s: %w", name, err)
		}
		var buf strings.Builder
		if err := tmpl.Execute(&buf, tmplCtx); err != nil {
			return nil, fmt.Errorf("arg %s: %w", name, err)
		}
		out[name] = buf.String()
	}
	return out, nil
}
