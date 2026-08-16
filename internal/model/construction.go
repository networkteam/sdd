package model

import (
	"fmt"
	"strings"
	"time"
)

// EntryConstruction is the single boundary for composing a graph entry: the
// common frontmatter fields alongside nil-able per-kind field blocks, of which
// at most one may be set and must agree with the declared type and kind.
// Construction never fails — validity is a checked property (Validate) — so
// the type can also hold every historical shape the graph contains. The
// per-kind structural requirements expressed here are those the active
// type-system contract enumerates (20260702-222259-d-cpt-7iy); this file is
// their single home. Write surfaces block on any finding; the read path
// projects parsed entries through ConstructFromEntry and keeps the non-
// write-only findings as health warnings.
type EntryConstruction struct {
	ID           string
	Type         EntryType
	Layer        Layer
	Kind         Kind
	Refs         []Ref
	Supersedes   []string
	Closes       []string
	Participants []string
	Confidence   string
	// Topics carries inline topic labels — valid on any non-annotation kind.
	// Annotation topic assignments live on the Annotation block instead.
	Topics      []TopicPath
	Body        string
	Time        time.Time
	Preflight   string
	Summary     string
	Attachments []string

	// Per-kind blocks. Only kinds with fields of their own have one; the
	// remaining kinds' rules (done's anchor, plan's acceptance criteria) hold
	// over the common fields and body.
	Directive  *DirectiveFields
	Actor      *ActorFields
	Role       *RoleFields
	Procedure  *ProcedureFields
	Annotation *AnnotationFields
	Focus      *FocusFields
	Fact       *FactFields
}

// DirectiveFields carries the directive kind's lifecycle posture. A nil block
// on a directive is the legacy pre-attribute shape — valid on read, refused
// on new captures.
type DirectiveFields struct {
	Intent Intent
}

// ActorFields carries the actor kind's identity fields.
type ActorFields struct {
	Canonical string
	Aliases   []string
}

// RoleFields names the canonical of the actor-identity chain the role binds to.
type RoleFields struct {
	Actor string
}

// ProcedureFields carries the procedure kind's identity and machine part.
// Structural validation of the spec itself stays the engine's load-time job.
type ProcedureFields struct {
	Canonical string
	Class     ProcedureClass
	Spec      *ProcedureSpecRaw
}

// AnnotationFields carries an annotation's topic assignments.
type AnnotationFields struct {
	Topics []AnnotationTopic
}

// FocusFields carries a focus decision's actor default, temporal scope, and
// involvement triples.
type FocusFields struct {
	Actors      []string
	When        *FocusWhen
	Involvement []Involvement
}

// FactFields carries a fact's optional index enrollment and override marker
// (Entry.Override).
type FactFields struct {
	Index    *FactIndex
	Override string
}

// Finding is one structural-rule violation from validating a construction.
type Finding struct {
	Field   string
	Value   string
	Message string
	// WriteOnly marks a rule enforced on new captures but waived for entries
	// already on disk — the type-system contract applies kind checks to new
	// captures; immutability prevails for history.
	WriteOnly bool
}

// Warning converts the finding to the read-side health shape.
func (f Finding) Warning() Warning {
	return Warning{Field: f.Field, Value: f.Value, Message: f.Message}
}

// ReadWarnings filters findings to those that hold on historical entries.
func ReadWarnings(findings []Finding) []Warning {
	var warnings []Warning
	for _, f := range findings {
		if !f.WriteOnly {
			warnings = append(warnings, f.Warning())
		}
	}
	return warnings
}

// DoneAnchorRequirement is the done kind's structural rule, declared once so
// the validator and the rendered kind fact serve the same words — the served
// rule cannot drift from what capture enforces.
const DoneAnchorRequirement = "done signal must carry at least one closes or refs (target of the completion claim)"

// SignalCloseRule is the closes rule over signal kinds, declared once so the
// validator and the served kind facts render the same words.
const SignalCloseRule = "only done-kind signals may close entries, or a fact/insight dissolving a question"

// DirectiveIntentRequirement is the directive kind's structural rule,
// declared once so the validator and the served kind fact render the same
// words.
const DirectiveIntentRequirement = "directive decisions require an explicit intent (pending, guiding, or settled)"

// SettledCloseRule is the settled directive's lifecycle rule, declared once
// so the validator and the served kind fact render the same words.
const SettledCloseRule = "a settled directive is born terminal and cannot be closed — supersede it instead"

// PlanAcceptanceHeading and PlanChecklistItem are the plan kind's structural
// markers, declared once for the validator and rendered kind facts alike.
const (
	PlanAcceptanceHeading = "## Acceptance criteria"
	PlanChecklistItem     = "- [ ]"
)

// kindListPhrase renders a kind enumeration as "a, b, or c" — generated from
// the declaration so messages track the enumeration.
func kindListPhrase(kinds []Kind) string {
	parts := make([]string, len(kinds))
	for i, k := range kinds {
		parts[i] = string(k)
	}
	if len(parts) < 2 {
		return strings.Join(parts, "")
	}
	return strings.Join(parts[:len(parts)-1], ", ") + ", or " + parts[len(parts)-1]
}

// ConstructFromEntry projects the raw parsed form into the construction
// model. The translation may be lossy: a field that does not belong to the
// entry's kind is reported as a finding and left out of the projection, and a
// kindless entry has no block to convert into — never an error, so every
// parseable historical entry projects.
func ConstructFromEntry(e *Entry) (*EntryConstruction, []Finding) {
	c := &EntryConstruction{
		ID:           e.ID,
		Type:         e.Type,
		Layer:        e.Layer,
		Kind:         e.Kind,
		Refs:         e.Refs,
		Supersedes:   e.Supersedes,
		Closes:       e.Closes,
		Participants: e.Participants,
		Confidence:   e.Confidence,
		Topics:       e.Topics,
		Body:         e.Content,
		Time:         e.Time,
		Preflight:    e.Preflight,
		Summary:      e.Summary,
		Attachments:  e.Attachments,
	}
	var findings []Finding
	stray := func(field, value, message string) {
		findings = append(findings, Finding{Field: field, Value: value, Message: message})
	}

	isDirective := e.Type == TypeDecision && e.Kind == KindDirective
	if isDirective && e.Intent != "" {
		c.Directive = &DirectiveFields{Intent: e.Intent}
	} else if e.Intent != "" {
		stray("intent", string(e.Intent), fmt.Sprintf("intent is only valid on directive decisions (got %s)", e.Kind))
	}

	switch {
	case e.IsActor():
		c.Actor = &ActorFields{Canonical: e.Canonical, Aliases: e.Aliases}
	case e.IsProcedure():
		c.Procedure = &ProcedureFields{Canonical: e.Canonical, Class: e.Class, Spec: e.ProcedureSpec}
		if len(e.Aliases) > 0 {
			stray("aliases", "", "aliases are only meaningful on kind: actor signals")
		}
	default:
		if e.Canonical != "" {
			stray("canonical", e.Canonical, "canonical is only meaningful on kind: actor signals and kind: procedure decisions")
		}
		if len(e.Aliases) > 0 {
			stray("aliases", "", "aliases are only meaningful on kind: actor signals")
		}
	}
	if !e.IsProcedure() {
		if e.Class != "" {
			stray("class", string(e.Class), fmt.Sprintf("class is only valid on procedure decisions (got %s)", e.Kind))
		}
		if e.ProcedureSpec != nil {
			stray("steps", "", "a procedure spec (params, state, steps, framing) is only meaningful on kind: procedure decisions")
		}
	}

	if e.IsRole() {
		c.Role = &RoleFields{Actor: e.Actor}
	} else if e.Actor != "" {
		stray("actor", e.Actor, fmt.Sprintf("actor is only valid on role decisions (got %s)", e.Kind))
	}

	if e.IsAnnotation() {
		c.Annotation = &AnnotationFields{Topics: e.AnnotationTopics}
		if len(e.Topics) > 0 {
			c.Topics = nil
			stray("topics", "", "inline topics are not valid on kind: annotation signals (topics carry the annotation's assignments)")
		}
	} else if len(e.AnnotationTopics) > 0 {
		stray("topics", "", "annotation topic assignments are only meaningful on kind: annotation signals")
	}

	if e.IsFocus() {
		c.Focus = &FocusFields{Actors: e.FocusActors, When: e.FocusWhen, Involvement: e.Involvement}
	} else {
		if len(e.FocusActors) > 0 {
			stray("actors", "", "actors is only valid on focus decisions")
		}
		if e.FocusWhen != nil {
			stray("when", "", "when is only valid on focus decisions")
		}
		if len(e.Involvement) > 0 {
			stray("involvement", "", "involvement is only valid on focus decisions")
		}
	}

	isFact := e.Type == TypeSignal && e.Kind == KindFact
	if isFact {
		if e.Index != nil || e.Override != "" {
			c.Fact = &FactFields{Index: e.Index, Override: e.Override}
		}
	} else {
		if e.Index != nil {
			stray("index", "", "index is only valid on kind: fact")
		}
		if e.Override != "" {
			stray("override", e.Override, "override is only valid on kind: fact")
		}
	}

	return c, findings
}

// Entry materializes the flat storage form of the construction. The result
// carries no warnings — validity is asked through Validate, not the
// materialization.
func (c *EntryConstruction) Entry() *Entry {
	e := &Entry{
		ID:           c.ID,
		Type:         c.Type,
		Layer:        c.Layer,
		Kind:         c.Kind,
		Refs:         c.Refs,
		Supersedes:   c.Supersedes,
		Closes:       c.Closes,
		Participants: c.Participants,
		Confidence:   c.Confidence,
		Topics:       c.Topics,
		Content:      c.Body,
		Time:         c.Time,
		Preflight:    c.Preflight,
		Summary:      c.Summary,
		Attachments:  c.Attachments,
	}
	if c.Directive != nil {
		e.Intent = c.Directive.Intent
	}
	if c.Actor != nil {
		e.Canonical = c.Actor.Canonical
		e.Aliases = c.Actor.Aliases
	}
	if c.Role != nil {
		e.Actor = c.Role.Actor
	}
	if c.Procedure != nil {
		e.Canonical = c.Procedure.Canonical
		e.Class = c.Procedure.Class
		e.ProcedureSpec = c.Procedure.Spec
	}
	if c.Annotation != nil {
		e.AnnotationTopics = c.Annotation.Topics
	}
	if c.Focus != nil {
		e.FocusActors = c.Focus.Actors
		e.FocusWhen = c.Focus.When
		e.Involvement = c.Focus.Involvement
	}
	if c.Fact != nil {
		e.Index = c.Fact.Index
		e.Override = c.Fact.Override
	}
	return e
}

// Validate checks the construction against the per-kind structural
// requirements the type-system contract enumerates. It never mutates.
// g supplies resolution context (focus involvement targets); nil skips
// those rules.
func (c *EntryConstruction) Validate(g *Graph) []Finding {
	var findings []Finding
	add := func(field, value, message string) {
		findings = append(findings, Finding{Field: field, Value: value, Message: message})
	}
	addWriteOnly := func(field, value, message string) {
		findings = append(findings, Finding{Field: field, Value: value, Message: message, WriteOnly: true})
	}

	findings = append(findings, c.validateKind()...)
	findings = append(findings, c.validateBlockAgreement()...)

	// Directive: explicit intent from the closed value set, required on new
	// captures — no default, because a default would fabricate the
	// non-derivable posture the attribute exists to capture honestly.
	isDirective := c.Type == TypeDecision && c.Kind == KindDirective
	if c.Directive != nil {
		if !IsValidIntent(string(c.Directive.Intent)) {
			add("intent", string(c.Directive.Intent), fmt.Sprintf("invalid intent %q (expected pending, guiding, or settled)", c.Directive.Intent))
		}
	} else if isDirective {
		addWriteOnly("intent", "", DirectiveIntentRequirement)
	}

	if c.isKind(TypeSignal, KindActor) {
		if c.Actor == nil || strings.TrimSpace(c.Actor.Canonical) == "" {
			add("canonical", "", "actor signal missing required canonical field")
		}
		if c.Layer != LayerProcess {
			add("layer", string(c.Layer), fmt.Sprintf("actor signal should live at process layer (got %s)", c.Layer))
		}
		if c.Actor != nil {
			seen := make(map[string]bool, len(c.Actor.Aliases))
			for i, alias := range c.Actor.Aliases {
				switch {
				case strings.TrimSpace(alias) == "":
					add("aliases", fmt.Sprintf("aliases[%d]", i), fmt.Sprintf("aliases[%d]: empty alias", i))
				case alias == c.Actor.Canonical:
					add("aliases", alias, fmt.Sprintf("alias %q duplicates the canonical", alias))
				case seen[alias]:
					add("aliases", alias, fmt.Sprintf("duplicate alias %q", alias))
				}
				seen[alias] = true
			}
		}
	}

	if c.isKind(TypeDecision, KindRole) {
		if c.Role == nil || strings.TrimSpace(c.Role.Actor) == "" {
			add("actor", "", "role decision missing required actor field")
		}
		if c.Layer != LayerProcess {
			add("layer", string(c.Layer), fmt.Sprintf("role decision should live at process layer (got %s)", c.Layer))
		}
	}

	if c.isKind(TypeDecision, KindProcedure) {
		if c.Procedure == nil || strings.TrimSpace(c.Procedure.Canonical) == "" {
			add("canonical", "", "procedure decision missing required canonical field")
		}
		if c.Layer != LayerProcess {
			add("layer", string(c.Layer), fmt.Sprintf("procedure decision should live at process layer (got %s)", c.Layer))
		}
		if c.Procedure != nil {
			switch c.Procedure.Class {
			case "", ProcedureClassMove, ProcedureClassShell, ProcedureClassTask:
			default:
				add("class", string(c.Procedure.Class), "procedure class must be move, shell, or task (empty means move)")
			}
		}
	}

	// Done: a fact-of-completion points at the commitment it fulfills — a
	// target is the minimum anchor for the claim.
	if c.isKind(TypeSignal, KindDone) && len(c.Closes) == 0 && len(c.Refs) == 0 {
		add("closes", "", DoneAnchorRequirement)
	}

	// Plan: commits are checkable — the acceptance-criteria section is the
	// plan kind's structural shape. Applies to new captures; history keeps
	// its original form.
	if c.isKind(TypeDecision, KindPlan) {
		if !strings.Contains(c.Body, PlanAcceptanceHeading) || !strings.Contains(c.Body, PlanChecklistItem) {
			addWriteOnly("content", "", fmt.Sprintf("plan decision requires a %q section with at least one %q checklist item", PlanAcceptanceHeading, PlanChecklistItem))
		}
	}

	if c.isKind(TypeSignal, KindAnnotation) {
		findings = append(findings, c.validateAnnotation()...)
	}

	if c.isKind(TypeDecision, KindFocus) {
		findings = append(findings, c.validateFocus(g)...)
	}

	if c.Fact != nil && c.Fact.Index != nil {
		if err := c.Fact.Index.ValidateForEntry(c.Kind, c.Topics); err != nil {
			add("index", "", err.Error())
		}
	}

	if c.Fact != nil && c.Fact.Override != "" && c.Fact.Override != OverrideClosed {
		add("override", c.Fact.Override, fmt.Sprintf("override: only %q is defined", OverrideClosed))
	}

	findings = append(findings, c.validateInlineTopics()...)

	return findings
}

func (c *EntryConstruction) isKind(t EntryType, k Kind) bool {
	return c.Type == t && c.Kind == k
}

// validateKind checks that the entry carries a kind consistent with its type.
func (c *EntryConstruction) validateKind() []Finding {
	var findings []Finding
	var order []Kind
	switch c.Type {
	case TypeSignal:
		order = signalKindOrder
	case TypeDecision:
		order = decisionKindOrder
	default:
		return nil
	}
	if c.Kind == "" {
		findings = append(findings, Finding{
			Field:   "kind",
			Message: fmt.Sprintf("%s missing kind field (expected %s)", c.Type, kindListPhrase(order)),
		})
		return findings
	}
	if !IsValidKindForType(c.Type, c.Kind) {
		findings = append(findings, Finding{
			Field:   "kind",
			Value:   string(c.Kind),
			Message: fmt.Sprintf("invalid %s kind %q (expected %s)", c.Type, c.Kind, kindListPhrase(order)),
		})
	}
	return findings
}

// validateBlockAgreement enforces the at-most-one rule: every set block must
// agree with the declared type and kind.
func (c *EntryConstruction) validateBlockAgreement() []Finding {
	var findings []Finding
	check := func(set bool, field string, t EntryType, k Kind) {
		if set && !c.isKind(t, k) {
			findings = append(findings, Finding{
				Field:   field,
				Message: fmt.Sprintf("%s fields are only meaningful on %s entries of kind %s (got %s %s)", k, t, k, c.Type, c.Kind),
			})
		}
	}
	check(c.Directive != nil, "intent", TypeDecision, KindDirective)
	check(c.Actor != nil, "canonical", TypeSignal, KindActor)
	check(c.Role != nil, "actor", TypeDecision, KindRole)
	check(c.Procedure != nil, "canonical", TypeDecision, KindProcedure)
	check(c.Annotation != nil, "topics", TypeSignal, KindAnnotation)
	check(c.Focus != nil, "involvement", TypeDecision, KindFocus)
	check(c.Fact != nil, "index", TypeSignal, KindFact)
	return findings
}

// validateAnnotation checks the structural shape of an annotation: at least
// one ref (the canonical edge to its members), at least one topic, every
// topic label parses, and every explicit members list is a subset of refs.
func (c *EntryConstruction) validateAnnotation() []Finding {
	var findings []Finding
	add := func(field, value, message string) {
		findings = append(findings, Finding{Field: field, Value: value, Message: message})
	}
	if len(c.Refs) == 0 {
		add("refs", "", "annotation signal must carry at least one ref (the entries the annotation is about)")
	}
	var topics []AnnotationTopic
	if c.Annotation != nil {
		topics = c.Annotation.Topics
	}
	if len(topics) == 0 {
		add("topics", "", "annotation signal must declare at least one topic")
		return findings
	}
	refSet := make(map[string]bool, len(c.Refs))
	for _, r := range c.Refs {
		refSet[r.ID] = true
	}
	for i, t := range topics {
		if t.Label == "" {
			add("topics", fmt.Sprintf("topics[%d]", i), fmt.Sprintf("topics[%d]: missing label", i))
			continue
		}
		if _, err := ParseTopicPath(t.Label); err != nil {
			add("topics", t.Label, fmt.Sprintf("topics[%d].label: %v", i, err))
		}
		for j, m := range t.Members {
			if !refSet[m] {
				add("topics", m, fmt.Sprintf("topics[%d].members[%d]: %s is not in refs (members must be a subset of the annotation's refs)", i, j, m))
			}
		}
	}
	return findings
}

// validateFocus checks the structural shape of a focus decision: well-formed
// when scopes, non-empty actor names, and at least one involvement triple
// whose target resolves in the graph.
func (c *EntryConstruction) validateFocus(g *Graph) []Finding {
	var findings []Finding
	add := func(field, value, message string) {
		findings = append(findings, Finding{Field: field, Value: value, Message: message})
	}
	var focus FocusFields
	if c.Focus != nil {
		focus = *c.Focus
	}
	if err := focus.When.Validate(); err != nil {
		add("when", "", err.Error())
	}
	for i, name := range focus.Actors {
		if strings.TrimSpace(name) == "" {
			add("actors", fmt.Sprintf("actors[%d]", i), fmt.Sprintf("actors[%d]: empty actor name", i))
		}
	}
	if len(focus.Involvement) == 0 {
		add("involvement", "", "focus decision must declare at least one involvement triple")
		return findings
	}
	for i, inv := range focus.Involvement {
		if strings.TrimSpace(inv.Target) == "" {
			add("involvement", fmt.Sprintf("involvement[%d]", i), fmt.Sprintf("involvement[%d]: missing target", i))
			continue
		}
		if g != nil {
			if _, ok := g.ByID[inv.Target]; !ok {
				add("involvement", inv.Target, fmt.Sprintf("involvement[%d].target: %s does not resolve to an existing entry", i, inv.Target))
			}
		}
		if err := inv.When.Validate(); err != nil {
			add("involvement", fmt.Sprintf("involvement[%d].when", i), fmt.Sprintf("involvement[%d].when: %v", i, err))
		}
		for j, name := range inv.Actors {
			if strings.TrimSpace(name) == "" {
				add("involvement", fmt.Sprintf("involvement[%d].actors[%d]", i, j), fmt.Sprintf("involvement[%d].actors[%d]: empty actor name", i, j))
			}
		}
	}
	return findings
}

// validateInlineTopics checks that every inline topic path is well-formed —
// entries built programmatically may carry unparsed labels.
func (c *EntryConstruction) validateInlineTopics() []Finding {
	var findings []Finding
	for i, t := range c.Topics {
		if t.IsZero() {
			findings = append(findings, Finding{
				Field:   "topics",
				Value:   fmt.Sprintf("topics[%d]", i),
				Message: fmt.Sprintf("topics[%d]: empty topic path", i),
			})
			continue
		}
		if _, err := ParseTopicPath(t.String()); err != nil {
			findings = append(findings, Finding{
				Field:   "topics",
				Value:   t.String(),
				Message: fmt.Sprintf("topics[%d]: %v", i, err),
			})
		}
	}
	return findings
}
