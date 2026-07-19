package engine

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/networkteam/sdd/internal/model"
)

// Spec is the typed, validated form of a procedure entry's state machine —
// what the engine executes. Parsed from the raw YAML the model retains on
// kind: procedure entries (structural validation happens here, at engine
// load time, per the type-system revision contract) plus the body's
// per-step instruction units.
type Spec struct {
	// EntryID is the graph entry the spec was loaded from.
	EntryID string
	// Canonical is the procedure's stable identity (capture, engage, …).
	Canonical string
	// Class is the procedure's execution role: a move is started through the
	// loop, a shell is a session base auto-started by the session door.
	Class model.ProcedureClass
	// Params are the typed inputs accepted at start_procedure.
	Params map[string]VarDecl
	// State is the report-writable variable store declaration. Engine-
	// written values are deliberately not declared — they enter the store
	// per the Go contracts of the functions the spec names, and collisions
	// with these declarations are rejected at load.
	State map[string]VarDecl
	// Steps is the state graph, in declaration order. The first step is the
	// entry step.
	Steps []*Step
	// StepByID indexes Steps.
	StepByID map[string]*Step
	// Units are the body's instruction units by name (`## unit: <name>`
	// sections), rendered as Go templates against the store at serve time.
	Units map[string]string
	// Framing declares a shell's session-framing lanes: inject query calls the
	// shell renders into its serve alongside the engine-supplied info block,
	// through the same query mechanism a step's inject uses. Empty on moves and
	// on shells that declare none (which then serve info-only framing).
	Framing []InjectCall
}

// VarDecl declares one typed variable. The desc seeds both the generated
// report schema and the "provide the following" instruction text — one
// declaration, both surfaces.
type VarDecl struct {
	Type     VarType
	Optional bool
	Desc     string
}

// ChooserKind classifies who advances a step: the engine (gate,
// auto-advance on guards), the agent (advisory pick, logged with evidence),
// or the user (dialogue-presence made structural).
type ChooserKind string

const (
	ChooserGate  ChooserKind = "gate"
	ChooserAgent ChooserKind = "agent"
	ChooserUser  ChooserKind = "user"
)

// CollectField names a state field a report may write at a step; Optional
// fields don't gate the cascade.
type CollectField struct {
	Name     string
	Optional bool
}

// InjectCall is a query op the engine runs before serving a step's unit.
// Args are literals or Go templates rendered against the store.
type InjectCall struct {
	Fn   string
	Args map[string]any
}

// Option is one choice on an agent or user chooser step.
type Option struct {
	Choice  string
	Call    string // command run on selection, per its Go contract
	Collect []CollectField
	To      string
	// Dispatch declares the grounding this junction option seeds into a child
	// dispatched after it is answered (child field ← parent field, named in the
	// parent's own spec). Nil means this option hands nothing down.
	Dispatch *Dispatch
}

// Dispatch is a junction option's declaration of the grounding it seeds into a
// child dispatched after this option is answered. The seeding contract
// (d-tac-tlo) keeps the declaration entirely in the parent's spec: answering an
// option carrying a Dispatch stashes its Seed on the parent instance, and the
// next child started under that parent inherits exactly that mapping. There is
// no engine-wide default — a handoff happens only where a junction declares
// one, so every inheritance is readable at the junction that grants it.
type Dispatch struct {
	// Procedure optionally names the child canonical this option dispatches.
	// When set, it guards the handoff — the seed applies only to a child of
	// that procedure. Omit it for a generic junction whose child is chosen at
	// runtime (e.g. engage's move), where the seed applies to whatever is
	// dispatched next.
	Procedure string
	// Seed maps child field ← parent field: the key is the field the child
	// declares as state, the value is this parent's own declared field carrying
	// the evidence. Required — a Dispatch with no seed has nothing to carry.
	Seed map[string]string
}

// Transition is one ordered {when, to} entry; the final entry may be an
// otherwise fallback (When nil).
type Transition struct {
	When      *GuardExpr
	Otherwise bool
	To        string
}

// Step is one node of the state graph.
type Step struct {
	ID          string
	Collect     []CollectField
	Inject      []InjectCall
	Render      string
	Chooser     ChooserKind
	Options     []Option
	Guard       *GuardExpr
	Op          string
	Transitions []Transition
	// Goal overrides the serve's generic per-chooser goal line for this
	// step. Built for resident steps whose standing posture the generic
	// wording would misstate (a session shell's junction is "dialogue
	// freely", not "put the choice to the user").
	Goal string
}

// Terminal transition targets. A procedure ends by transitioning to one of
// these instead of a step ID.
const (
	EndCompleted = "end(completed)"
	EndAbandoned = "end(abandoned)"
)

// IsEndTarget reports whether a transition target is a terminal rather than
// a step ID.
func IsEndTarget(to string) bool {
	return to == EndCompleted || to == EndAbandoned
}

// --- YAML intermediate shapes -------------------------------------------

type varDeclYAML struct {
	Type     string `yaml:"type"`
	Optional bool   `yaml:"optional"`
	Desc     string `yaml:"desc"`
}

type injectYAML struct {
	Fn   string         `yaml:"fn"`
	Args map[string]any `yaml:"args"`
}

type optionYAML struct {
	Choice   string        `yaml:"choice"`
	Call     string        `yaml:"call"`
	Collect  []string      `yaml:"collect"`
	To       string        `yaml:"to"`
	Dispatch *dispatchYAML `yaml:"dispatch"`
}

type dispatchYAML struct {
	Procedure string            `yaml:"procedure"`
	Seed      map[string]string `yaml:"seed"`
}

type transitionYAML struct {
	When      string `yaml:"when"`
	Otherwise string `yaml:"otherwise"`
	To        string `yaml:"to"`
}

type stepYAML struct {
	ID          string           `yaml:"id"`
	Collect     []string         `yaml:"collect"`
	Inject      []injectYAML     `yaml:"inject"`
	Render      string           `yaml:"render"`
	Chooser     string           `yaml:"chooser"`
	Options     []optionYAML     `yaml:"options"`
	Guard       string           `yaml:"guard"`
	Op          string           `yaml:"op"`
	Transitions []transitionYAML `yaml:"transitions"`
	Goal        string           `yaml:"goal"`
}

// decodeStrict re-encodes a retained YAML node and decodes it with unknown
// fields rejected, so a typo'd step key fails spec load instead of being
// silently dropped.
func decodeStrict(node *yaml.Node, out any) error {
	if node.IsZero() {
		return nil
	}
	raw, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	return dec.Decode(out)
}

// ParseSpec parses a procedure entry into a typed Spec: variable
// declarations, the step graph with parsed guards, and the body's
// instruction units. Registry-independent validation happens here (types,
// step wiring, guard syntax, collect declarations); function existence and
// write-collision checks need the registry and run in Validate.
func ParseSpec(entry *model.Entry) (*Spec, error) {
	if !entry.IsProcedure() {
		return nil, fmt.Errorf("entry %s is not a kind: procedure decision", entry.ID)
	}
	if strings.TrimSpace(entry.Canonical) == "" {
		return nil, fmt.Errorf("procedure %s: missing canonical", entry.ID)
	}
	spec := &Spec{
		EntryID:   entry.ID,
		Canonical: entry.Canonical,
		Class:     model.ProcedureClassMove,
		Params:    map[string]VarDecl{},
		State:     map[string]VarDecl{},
		StepByID:  map[string]*Step{},
		Units:     parseUnits(entry.Content),
	}

	var problems []string
	addProblem := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	switch entry.Class {
	case "", model.ProcedureClassMove:
	case model.ProcedureClassShell:
		spec.Class = model.ProcedureClassShell
	case model.ProcedureClassTask:
		spec.Class = model.ProcedureClassTask
	default:
		addProblem("class: unknown class %q (move, shell, or task)", entry.Class)
	}

	raw := entry.ProcedureSpec
	if raw == nil {
		return nil, fmt.Errorf("procedure %s (%s): no params/state/steps frontmatter — nothing to execute", entry.Canonical, entry.ID)
	}

	var paramsYAML, stateYAML map[string]varDeclYAML
	if err := decodeStrict(&raw.Params, &paramsYAML); err != nil {
		return nil, fmt.Errorf("procedure %s: params: %w", entry.Canonical, err)
	}
	if err := decodeStrict(&raw.State, &stateYAML); err != nil {
		return nil, fmt.Errorf("procedure %s: state: %w", entry.Canonical, err)
	}
	var stepsYAML []stepYAML
	if err := decodeStrict(&raw.Steps, &stepsYAML); err != nil {
		return nil, fmt.Errorf("procedure %s: steps: %w", entry.Canonical, err)
	}
	var framingYAML []injectYAML
	if err := decodeStrict(&raw.Framing, &framingYAML); err != nil {
		return nil, fmt.Errorf("procedure %s: framing: %w", entry.Canonical, err)
	}
	for i, iy := range framingYAML {
		if iy.Fn == "" {
			addProblem("framing[%d]: missing fn", i)
			continue
		}
		spec.Framing = append(spec.Framing, InjectCall(iy))
	}
	if len(spec.Framing) > 0 && spec.Class != model.ProcedureClassShell {
		addProblem("framing: declared on a %s procedure — framing lanes are a session-shell concern, ignored anywhere else", spec.Class)
	}

	for name, d := range paramsYAML {
		decl, err := parseVarDecl(name, d)
		if err != nil {
			addProblem("params.%s: %v", name, err)
			continue
		}
		spec.Params[name] = decl
	}
	for name, d := range stateYAML {
		if _, dup := spec.Params[name]; dup {
			addProblem("state.%s: collides with a param of the same name", name)
			continue
		}
		decl, err := parseVarDecl(name, d)
		if err != nil {
			addProblem("state.%s: %v", name, err)
			continue
		}
		spec.State[name] = decl
	}

	if len(stepsYAML) == 0 {
		addProblem("steps: a procedure needs at least one step")
	}

	for i, sy := range stepsYAML {
		step, stepProblems := parseStep(sy, i)
		problems = append(problems, stepProblems...)
		if step == nil {
			continue
		}
		if _, dup := spec.StepByID[step.ID]; dup {
			addProblem("steps.%s: duplicate step id", step.ID)
			continue
		}
		spec.Steps = append(spec.Steps, step)
		spec.StepByID[step.ID] = step
	}

	// Wiring checks over the assembled graph.
	for _, step := range spec.Steps {
		prefix := "steps." + step.ID
		for _, t := range step.Transitions {
			if !IsEndTarget(t.To) && spec.StepByID[t.To] == nil {
				addProblem("%s: transition target %q is not a step or end(...)", prefix, t.To)
			}
		}
		for _, o := range step.Options {
			if !IsEndTarget(o.To) && spec.StepByID[o.To] == nil {
				addProblem("%s: option %q target %q is not a step or end(...)", prefix, o.Choice, o.To)
			}
			for _, c := range o.Collect {
				if _, ok := spec.State[c.Name]; !ok {
					addProblem("%s: option %q collects %q, which is not declared in state", prefix, o.Choice, c.Name)
				}
			}
			if o.Dispatch != nil {
				if len(o.Dispatch.Seed) == 0 {
					addProblem("%s: option %q dispatch: needs a seed mapping (child field <- parent field)", prefix, o.Choice)
				}
				// The source side of each seed mapping (the parent field) must
				// be one this spec declares — a mistyped source would silently
				// carry nothing, the exact failure the per-junction mapping
				// exists to make loud. The child field (the key) is validated
				// against the child at write time, not here.
				for child, parentField := range o.Dispatch.Seed {
					if _, isState := spec.State[parentField]; isState {
						continue
					}
					if _, isParam := spec.Params[parentField]; isParam {
						continue
					}
					addProblem("%s: option %q dispatch seeds %q from %q, which is not a declared param or state field of this procedure", prefix, o.Choice, child, parentField)
				}
			}
		}
		for _, c := range step.Collect {
			if _, ok := spec.State[c.Name]; !ok {
				addProblem("%s: collect names %q, which is not declared in state", prefix, c.Name)
			}
		}
		if step.Render != "" {
			if _, ok := spec.Units[step.Render]; !ok {
				addProblem("%s: render names unit %q, but the body has no `## unit: %s` section", prefix, step.Render, step.Render)
			}
		}
		// A task runs in a delegate context with no user present — a user
		// chooser there would block on a dialogue turn that never comes.
		if spec.Class == model.ProcedureClassTask && step.Chooser == ChooserUser {
			addProblem("%s: a task procedure has no user present — user choosers are not allowed (dispatch resolves its inputs as params)", prefix)
		}
	}

	if len(problems) > 0 {
		return nil, specError(spec.Canonical, entry.ID, problems)
	}
	return spec, nil
}

func parseVarDecl(name string, d varDeclYAML) (VarDecl, error) {
	if !isValidFuncName(name) {
		return VarDecl{}, fmt.Errorf("invalid variable name (identifiers only)")
	}
	t, err := ParseVarType(d.Type)
	if err != nil {
		return VarDecl{}, err
	}
	return VarDecl{Type: t, Optional: d.Optional, Desc: d.Desc}, nil
}

func parseStep(sy stepYAML, index int) (*Step, []string) {
	var problems []string
	addProblem := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	if sy.ID == "" {
		addProblem("steps[%d]: missing id", index)
		return nil, problems
	}
	prefix := "steps." + sy.ID

	step := &Step{
		ID:      sy.ID,
		Render:  sy.Render,
		Op:      sy.Op,
		Chooser: ChooserGate,
		Goal:    sy.Goal,
	}

	switch sy.Chooser {
	case "", string(ChooserGate):
	case string(ChooserAgent):
		step.Chooser = ChooserAgent
	case string(ChooserUser):
		step.Chooser = ChooserUser
	default:
		addProblem("%s: unknown chooser %q (gate, agent, or user)", prefix, sy.Chooser)
	}

	for _, f := range sy.Collect {
		cf, err := parseCollectField(f)
		if err != nil {
			addProblem("%s: collect: %v", prefix, err)
			continue
		}
		step.Collect = append(step.Collect, cf)
	}

	for j, iy := range sy.Inject {
		if iy.Fn == "" {
			addProblem("%s: inject[%d]: missing fn", prefix, j)
			continue
		}
		step.Inject = append(step.Inject, InjectCall(iy))
	}

	if sy.Guard != "" {
		g, err := ParseGuard(sy.Guard)
		if err != nil {
			addProblem("%s: guard: %v", prefix, err)
		} else {
			step.Guard = g
		}
	}

	for j, oy := range sy.Options {
		if oy.Choice == "" {
			addProblem("%s: options[%d]: missing choice", prefix, j)
			continue
		}
		if oy.To == "" {
			addProblem("%s: option %q: missing to", prefix, oy.Choice)
			continue
		}
		opt := Option{Choice: oy.Choice, Call: oy.Call, To: oy.To}
		if oy.Dispatch != nil {
			opt.Dispatch = &Dispatch{Procedure: oy.Dispatch.Procedure, Seed: oy.Dispatch.Seed}
		}
		for _, f := range oy.Collect {
			cf, err := parseCollectField(f)
			if err != nil {
				addProblem("%s: option %q collect: %v", prefix, oy.Choice, err)
				continue
			}
			opt.Collect = append(opt.Collect, cf)
		}
		step.Options = append(step.Options, opt)
	}

	seenOtherwise := false
	for j, ty := range sy.Transitions {
		switch {
		case ty.Otherwise != "" || (ty.When == "" && ty.To != ""):
			// Otherwise fallback in any of its accepted spellings:
			// `otherwise: <target>`, `{otherwise: true, to: <target>}`, or a
			// bare `{to: <target>}` final entry.
			to := ty.Otherwise
			if to == "" || to == "true" {
				to = ty.To
			}
			if to == "" {
				addProblem("%s: transitions[%d]: otherwise needs a target", prefix, j)
				continue
			}
			if seenOtherwise {
				addProblem("%s: transitions[%d]: more than one otherwise fallback", prefix, j)
				continue
			}
			if j != len(sy.Transitions)-1 {
				addProblem("%s: transitions[%d]: otherwise must be the final transition", prefix, j)
			}
			seenOtherwise = true
			step.Transitions = append(step.Transitions, Transition{Otherwise: true, To: to})
		case ty.When != "":
			if ty.To == "" {
				addProblem("%s: transitions[%d]: missing to", prefix, j)
				continue
			}
			g, err := ParseGuard(ty.When)
			if err != nil {
				addProblem("%s: transitions[%d]: %v", prefix, j, err)
				continue
			}
			step.Transitions = append(step.Transitions, Transition{When: g, To: ty.To})
		default:
			addProblem("%s: transitions[%d]: needs when+to or otherwise", prefix, j)
		}
	}

	// Shape rules per chooser class: gates auto-advance on transitions and
	// may run an op; choosers advance through options only.
	switch step.Chooser {
	case ChooserGate:
		if len(step.Options) > 0 {
			addProblem("%s: options belong to agent/user chooser steps, not gates", prefix)
		}
		if len(step.Transitions) == 0 {
			addProblem("%s: a gate step needs at least one transition", prefix)
		}
	case ChooserAgent, ChooserUser:
		if len(step.Options) == 0 {
			addProblem("%s: a %s chooser needs options", prefix, step.Chooser)
		}
		if step.Op != "" {
			addProblem("%s: op runs on gate steps only (chooser options use call)", prefix)
		}
		if step.Guard != nil {
			addProblem("%s: guard applies to gate steps only", prefix)
		}
		if len(step.Transitions) > 0 {
			addProblem("%s: chooser steps transition through options, not transitions", prefix)
		}
	}

	return step, problems
}

func parseCollectField(f string) (CollectField, error) {
	name, optional := strings.CutSuffix(f, "?")
	if !isValidFuncName(name) {
		return CollectField{}, fmt.Errorf("invalid field name %q", f)
	}
	return CollectField{Name: name, Optional: optional}, nil
}

// unitHeading matches `## unit: <name>` section headings in a procedure
// entry's body.
var unitHeading = regexp.MustCompile(`(?m)^##\s+unit:\s*(\S+)\s*$`)

// parseUnits splits a procedure body into named instruction units. A unit
// runs from its heading to the next `## ` heading or the end of the body.
// Headings inside fenced code blocks are unit content, not boundaries — a
// unit may quote markdown structure (e.g. a briefing template).
func parseUnits(body string) map[string]string {
	units := make(map[string]string)
	var name string
	var buf []string
	inFence := false
	flush := func() {
		if name != "" {
			units[name] = strings.TrimSpace(strings.Join(buf, "\n"))
		}
		name, buf = "", nil
	}
	for line := range strings.SplitSeq(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		} else if !inFence {
			if m := unitHeading.FindStringSubmatch(line); m != nil {
				flush()
				name = m[1]
				continue
			}
			if strings.HasPrefix(line, "## ") {
				flush()
				continue
			}
		}
		if name != "" {
			buf = append(buf, line)
		}
	}
	flush()
	return units
}

// Validate runs the registry-dependent load checks: every function a spec
// names exists in the right class, and no named command writes into a
// declared param or state field (engine-written values must stay
// structurally out of the report-writable surface). Returns all problems.
func (s *Spec) Validate(reg *Registry) []string {
	var problems []string
	addProblem := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	checkCommandWrites := func(prefix, name string) {
		cmd, ok := reg.Command(name)
		if !ok {
			addProblem("%s: %q is not a registered command", prefix, name)
			return
		}
		for _, w := range cmd.Doc.Writes {
			if _, isParam := s.Params[w]; isParam {
				addProblem("%s: command %q writes %q, which collides with a declared param", prefix, name, w)
			}
			if _, isState := s.State[w]; isState {
				addProblem("%s: command %q writes %q, which collides with declared state — engine-written fields are never declared", prefix, name, w)
			}
		}
	}

	for i, inj := range s.Framing {
		q, ok := reg.Query(inj.Fn)
		if !ok {
			addProblem("framing[%d]: inject fn %q is not a registered query", i, inj.Fn)
			continue
		}
		if !q.ServeSafe {
			addProblem("framing[%d]: query %q is not serve-safe — a framing lane renders on every serve and must not write or log (I7)", i, inj.Fn)
		}
	}

	for _, step := range s.Steps {
		prefix := "steps." + step.ID
		var guards []*GuardExpr
		if step.Guard != nil {
			guards = append(guards, step.Guard)
		}
		for _, t := range step.Transitions {
			if t.When != nil {
				guards = append(guards, t.When)
			}
		}
		for _, g := range guards {
			for _, name := range g.Predicates() {
				if _, ok := reg.Predicate(name); !ok {
					addProblem("%s: guard %q names unknown predicate %q", prefix, g.String(), name)
				}
			}
		}
		for _, inj := range step.Inject {
			if _, ok := reg.Query(inj.Fn); !ok {
				addProblem("%s: inject fn %q is not a registered query", prefix, inj.Fn)
			}
		}
		if step.Op != "" {
			checkCommandWrites(prefix, step.Op)
		}
		for _, o := range step.Options {
			if o.Call != "" {
				checkCommandWrites(prefix+" option "+o.Choice, o.Call)
			}
		}
	}
	return problems
}

// LoadSpec parses and fully validates a procedure entry against the
// registry — the one-call path the engine and lint use.
func LoadSpec(entry *model.Entry, reg *Registry) (*Spec, error) {
	spec, err := ParseSpec(entry)
	if err != nil {
		return nil, err
	}
	if problems := spec.Validate(reg); len(problems) > 0 {
		return nil, specError(spec.Canonical, spec.EntryID, problems)
	}
	return spec, nil
}

func specError(canonical, entryID string, problems []string) error {
	sort.Strings(problems)
	return fmt.Errorf("procedure %s (%s): invalid spec:\n  - %s", canonical, entryID, strings.Join(problems, "\n  - "))
}

func (s *Spec) paramNames() []string {
	names := make([]string, 0, len(s.Params))
	for n := range s.Params {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (s *Spec) stateNames() []string {
	names := make([]string, 0, len(s.State))
	for n := range s.State {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
