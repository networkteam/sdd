package engine

import (
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
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
	// dispatchSeed is the handoff a dispatching junction declared when its
	// option was answered — child field ← parent field. The next child started
	// under this instance inherits it (seedFromParent). Empty until a
	// seed-bearing option is answered; re-derived from the chooser answer on
	// replay.
	dispatchSeed map[string]string
	// dispatchProcedure guards dispatchSeed to a named child canonical when the
	// answered option declared one; empty means the seed applies to whatever is
	// dispatched next (a generic junction like engage's move).
	dispatchProcedure string
	// draftServed holds, per step, the serveDelta snapshot last served to this
	// instance — the engine-owned base later serves diff against. In-memory
	// only, deliberately: a process restart or resume forgets it, so those
	// paths serve whole (20260826-120330-d-tac-8f8).
	draftServed map[string]map[string]string
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

// ChooserServe is the pending-chooser part of a serve: which step's chooser is
// pending, who answers, and what the options are. Chooser is the step ID the
// caller must name in a chooser answer — carried here so the value the answer
// requires appears in the payload it answers.
type ChooserServe struct {
	Chooser string
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
	// Lanes are the unit's rendered lanes in declaration order (empty
	// rendered lanes dropped), the blocks a host dedups independently
	// (d-tac-87o). UnitText is their join.
	Lanes []ServeLane
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

// ServeLane is one rendered lane of a serve's instruction unit — the unit a
// host's served-once memory dedups at (d-tac-87o).
type ServeLane struct {
	Name string
	Text string
}

// evalGuard evaluates a guard against the registry, returning the verdict
// and the failing predicates for diagnostics.
func (s *Session) evalGuard(inst *Instance, g *GuardExpr) (bool, []FailedPredicate, error) {
	ctx, err := s.funcContext(inst)
	if err != nil {
		return false, nil, err
	}
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
			msg := p.FailMessage
			if p.FailDetail != nil {
				if detail := p.FailDetail(ctx); detail != "" {
					msg = detail
				}
			}
			failing = append(failing, FailedPredicate{Name: name, Message: msg})
		}
	}
	return false, failing, nil
}

// funcContext builds the Context registry functions run against at the
// instance's current position, resolving the current graph through the
// engine's provider. Reading can fail (a disk load error surfaces here rather
// than through a mutable field poked from outside); callers propagate it.
func (s *Session) funcContext(inst *Instance) (*Context, error) {
	var (
		graph *model.Graph
		err   error
	)
	if contextual, ok := s.engine.Graphs.(ContextualGraphs); ok {
		graph, err = contextual.CurrentFor(inst.Store)
	} else {
		graph, err = s.engine.Graphs.Current()
	}
	if err != nil {
		return nil, fmt.Errorf("resolving current graph: %w", err)
	}
	return &Context{Store: inst.Store, Graph: graph, Step: inst.Step, Reads: s.reads}, nil
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
	if err := inst.Store.WriteEngine(fieldPlaybackConfirmation, nil); err != nil {
		return false, err
	}
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
	ctx, err := s.funcContext(inst)
	if err != nil {
		return err
	}
	candidate := inst.Store.Clone()
	ctx.Store = candidate
	candidate.beginJournal()
	err = cmd.Fn(ctx)
	writes := candidate.drainJournal()
	if err != nil {
		return fmt.Errorf("command %q at step %s: %w", name, inst.Step, err)
	}
	inst.Store.commit(candidate)
	s.appendEvent(inst.ID, EventOpResult, map[string]any{
		"step":   inst.Step,
		"fn":     name,
		"writes": writes,
	})
	// A command that mutates the graph invalidates the provider so later reads
	// in the same advance — the fidelity review of a just-written entry — see
	// the write. Freshness is driven by the command's own declaration, not a
	// separate refresh call a new write command could forget.
	if cmd.MutatesGraph {
		s.engine.Graphs.Invalidate()
	}
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

// isTrustMachineryField reports whether a store field is internal trust
// machinery — the playback confirmation record and the pre-flight override —
// never surfaced to the agent as produced or collected state. The one
// authoritative exclusion list, shared by produced() and Store.Collected().
func isTrustMachineryField(name string) bool {
	return name == fieldPlaybackConfirmation || name == fieldPreflightOverride
}

// produced returns the engine-written values worth surfacing on completion,
// excluding the internal trust machinery fields.
func (i *Instance) produced() map[string]any {
	out := make(map[string]any)
	for name, ev := range i.Store.Export() {
		if ev.Provenance != ProvenanceEngine {
			continue
		}
		if isTrustMachineryField(name) {
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
	return s.serveWith(inst, false)
}

// serveWith renders the position; fullDraft forces serveDelta fields whole —
// the rehydrate path (Serve), where a resuming agent holds no earlier base.
func (s *Session) serveWith(inst *Instance, fullDraft bool) (*Serve, error) {
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
	lanes, err := s.renderUnit(inst, step)
	if err != nil {
		return nil, err
	}
	if len(step.ServeDelta) > 0 {
		cur := draftSnapshot(inst, step.ServeDelta)
		var prev map[string]string
		if !fullDraft {
			prev = inst.draftServed[step.ID]
		}
		if block := renderDraft(inst.Spec, step.ServeDelta, prev, cur, fullDraft); block != "" {
			lanes = append(lanes, ServeLane{Name: "draft", Text: block})
		}
		if inst.draftServed == nil {
			inst.draftServed = map[string]map[string]string{}
		}
		inst.draftServed[step.ID] = cur
	}
	sv.Lanes = lanes
	texts := make([]string, 0, len(lanes))
	for _, lane := range lanes {
		texts = append(texts, lane.Text)
	}
	instructions := strings.Join(texts, "\n\n")
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
		sv.Chooser = &ChooserServe{Chooser: step.ID, Kind: step.Chooser, Options: options}
		// At a chooser the report is a chooser-answer envelope, not a flat
		// state write — serve the schema that matches, so collected fields nest
		// under `fields` and the chooser is named where the answer needs it.
		sv.ReportSchema = inst.Spec.AnswerSchemaForStep(step)
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
// section named after the step) lane by lane, each lane a Go template over
// the store, with inject query results in the template context under each
// call's effective id. Lanes whose rendered text is empty are dropped.
func (s *Session) renderUnit(inst *Instance, step *Step) ([]ServeLane, error) {
	unitName := step.ID
	if step.Render != "" {
		unitName = step.Render
	}
	unit, ok := inst.Spec.Units[unitName]
	if !ok {
		return nil, nil // gates without prose serve diagnostics and schema only
	}

	tmplCtx, err := s.templateContext(inst)
	if err != nil {
		return nil, fmt.Errorf("unit %s: %w", unitName, err)
	}
	if len(step.Inject) > 0 {
		fctx, err := s.funcContext(inst)
		if err != nil {
			return nil, fmt.Errorf("step %s: %w", step.ID, err)
		}
		for _, inj := range step.Inject {
			q, found := s.engine.Registry.Query(inj.Fn)
			if !found {
				return nil, fmt.Errorf("step %s: inject fn %q is not a registered query", step.ID, inj.Fn)
			}
			args, err := s.renderInjectArgs(inst, inj.Args)
			if err != nil {
				return nil, fmt.Errorf("step %s: inject %s: %w", step.ID, inj.Fn, err)
			}
			result, err := q.Fn(fctx, args)
			if err != nil {
				return nil, fmt.Errorf("step %s: inject %s: %w", step.ID, inj.Fn, err)
			}
			tmplCtx[inj.EffectiveID()] = result
		}
	}

	lanes := make([]ServeLane, 0, len(unit.Lanes))
	for _, lane := range unit.Lanes {
		tmpl, err := template.New(lane.Name).Option("missingkey=zero").Parse(lane.Text)
		if err != nil {
			return nil, fmt.Errorf("unit %s: lane %s: %w", unitName, lane.Name, err)
		}
		var buf strings.Builder
		if err := tmpl.Execute(&buf, tmplCtx); err != nil {
			return nil, fmt.Errorf("unit %s: lane %s: %w", unitName, lane.Name, err)
		}
		if text := strings.TrimSpace(buf.String()); text != "" {
			lanes = append(lanes, ServeLane{Name: lane.Name, Text: text})
		}
	}
	return lanes, nil
}

// renderInjectArgs renders string args as Go templates against the store
// (literals pass through untouched); non-string args pass through as
// literals.
func (s *Session) renderInjectArgs(inst *Instance, args map[string]any) (map[string]any, error) {
	if len(args) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(args))
	tmplCtx, err := s.templateContext(inst)
	if err != nil {
		return nil, err
	}
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

// templateContext is the store's template context plus the engine's host-
// supplied template values. A value whose name collides with a declared param
// or state field fails the render — the spec's own vocabulary always wins,
// and silently shadowing either side would corrupt whichever loses.
func (s *Session) templateContext(inst *Instance) (map[string]any, error) {
	ctx := inst.Store.TemplateContext()
	for name, value := range s.engine.templateValues {
		if _, ok := inst.Spec.Params[name]; ok {
			return nil, fmt.Errorf("template value %q collides with a procedure param", name)
		}
		if _, ok := inst.Spec.State[name]; ok {
			return nil, fmt.Errorf("template value %q collides with a procedure state field", name)
		}
		ctx[name] = value
	}
	return ctx, nil
}
