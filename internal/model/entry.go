package model

import (
	"crypto/rand"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type EntryType string

const (
	TypeSignal   EntryType = "signal"
	TypeDecision EntryType = "decision"
)

type Layer string

const (
	LayerStrategic   Layer = "strategic"
	LayerConceptual  Layer = "conceptual"
	LayerTactical    Layer = "tactical"
	LayerOperational Layer = "operational"
	LayerProcess     Layer = "process"
)

// TypeAbbrev maps full type names to abbreviations used in IDs.
var TypeAbbrev = map[EntryType]string{
	TypeSignal:   "s",
	TypeDecision: "d",
}

// TypeFromAbbrev maps abbreviations to full type names.
var TypeFromAbbrev = map[string]EntryType{
	"s": TypeSignal,
	"d": TypeDecision,
}

// LayerAbbrev maps full layer names to abbreviations used in IDs.
var LayerAbbrev = map[Layer]string{
	LayerStrategic:   "stg",
	LayerConceptual:  "cpt",
	LayerTactical:    "tac",
	LayerOperational: "ops",
	LayerProcess:     "prc",
}

// LayerFromAbbrev maps abbreviations to full layer names.
var LayerFromAbbrev = map[string]Layer{
	"stg": LayerStrategic,
	"cpt": LayerConceptual,
	"tac": LayerTactical,
	"ops": LayerOperational,
	"prc": LayerProcess,
}

// Kind is a sub-type classifier carried on signals and decisions. Its allowed
// values depend on the entry's Type:
//
//   - Signal kinds: gap (default), fact, question, insight, done, actor, annotation
//   - Decision kinds: directive (default), activity, plan, contract, aspiration, role, focus, procedure
//
// Empty Kind on a new entry is replaced by the type's default during capture.
type Kind string

const (
	// Signal kinds.
	KindGap        Kind = "gap"
	KindFact       Kind = "fact"
	KindQuestion   Kind = "question"
	KindInsight    Kind = "insight"
	KindDone       Kind = "done"
	KindActor      Kind = "actor"
	KindAnnotation Kind = "annotation"

	// Decision kinds.
	KindDirective  Kind = "directive"
	KindActivity   Kind = "activity"
	KindPlan       Kind = "plan"
	KindContract   Kind = "contract"
	KindAspiration Kind = "aspiration"
	KindRole       Kind = "role"
	KindFocus      Kind = "focus"
	KindProcedure  Kind = "procedure"
)

// signalKinds is the set of kinds valid on type: signal entries.
var signalKinds = map[Kind]bool{
	KindGap:        true,
	KindFact:       true,
	KindQuestion:   true,
	KindInsight:    true,
	KindDone:       true,
	KindActor:      true,
	KindAnnotation: true,
}

// decisionKinds is the set of kinds valid on type: decision entries.
var decisionKinds = map[Kind]bool{
	KindDirective:  true,
	KindActivity:   true,
	KindPlan:       true,
	KindContract:   true,
	KindAspiration: true,
	KindRole:       true,
	KindFocus:      true,
	KindProcedure:  true,
}

// IsValidKindForType reports whether k is an allowed kind for the given type.
// Empty kind is allowed at this layer — defaults are applied separately during
// entry construction (see DefaultKindForType).
func IsValidKindForType(t EntryType, k Kind) bool {
	if k == "" {
		return true
	}
	switch t {
	case TypeSignal:
		return signalKinds[k]
	case TypeDecision:
		return decisionKinds[k]
	default:
		return false
	}
}

// DefaultKindForType returns the kind applied when a new entry of type t is
// captured without an explicit --kind. Signals default to gap; decisions to
// directive. Other types have no default (empty).
func DefaultKindForType(t EntryType) Kind {
	switch t {
	case TypeSignal:
		return KindGap
	case TypeDecision:
		return KindDirective
	default:
		return ""
	}
}

// Intent classifies a directive's lifecycle posture. It is stored frontmatter,
// supplied explicitly at capture, and meaningful only on kind: directive
// decisions:
//
//   - pending — demands follow-up; the action-on default
//   - guiding — standing context that shapes later decisions, never "completed"
//   - settled — born terminal; needs no follow-up and carries no closing edge
//
// A directive with no intent reads as unspecified (legacy / pre-attribute) and
// renders exactly as before. The model stays permissive on read; capture-time
// validation (command.NewEntryCmd.Validate) requires a value on new directives,
// because a default would fabricate the non-derivable value the attribute
// exists to capture honestly.
type Intent string

const (
	IntentPending Intent = "pending"
	IntentGuiding Intent = "guiding"
	IntentSettled Intent = "settled"
)

var validIntents = map[Intent]bool{
	IntentPending: true,
	IntentGuiding: true,
	IntentSettled: true,
}

// IsValidIntent reports whether s is one of the three intent values.
func IsValidIntent(s string) bool {
	return validIntents[Intent(s)]
}

// Warning represents a validation issue found on a graph entry.
type Warning struct {
	Field   string // "refs", "closes", "supersedes"
	Value   string // the offending ID or value
	Message string // human-readable description
}

type Entry struct {
	ID           string
	Type         EntryType
	Layer        Layer
	Kind         Kind // only meaningful for decisions; empty = directive (default)
	Refs         []Ref
	Supersedes   []string
	Closes       []string
	Participants []string
	Confidence   string
	// Intent is the lifecycle posture of a directive (pending|guiding|settled).
	// Stored frontmatter, meaningful only on kind: directive decisions; empty on
	// every other entry and on legacy directives captured before the attribute
	// existed. See the Intent type.
	Intent  Intent
	Content string
	Time    time.Time
	// Canonical is only meaningful on kind: actor signals and kind: procedure
	// decisions. On an actor it is the write-once identity string used in
	// participants fields; on a procedure it is the move's stable identity
	// (capture, engage, …) everything binds to instead of the entry ID. Each
	// kind's canonicals form their own namespace, but the write-once-across-
	// chains rule is the same. Aliases are only meaningful on actors —
	// read-side conveniences for mining and dialogue comprehension.
	Canonical string
	Aliases   []string
	// Class is only meaningful on kind: procedure decisions. It classifies
	// the procedure's execution role: a move (the default when empty) is a
	// playbook step started through the engine loop; a shell is a session
	// base auto-started by the session door and refused by start_procedure.
	Class ProcedureClass
	// Actor is only meaningful on kind: role decisions. It names the canonical
	// of the actor-identity chain the role binds to. Role status derives from
	// the actor chain's canonical history (see Graph.RoleStatus).
	Actor string
	// ProcedureSpec is only meaningful on kind: procedure decisions. It
	// retains the machine part of the frontmatter — params, state, steps — as
	// raw YAML nodes so procedure entries round-trip losslessly. The model
	// stays permissive here by design: the type-system revision contract
	// defers structural validation of the spec to engine load time, where the
	// engine decodes these nodes strictly.
	ProcedureSpec *ProcedureSpecRaw
	// Topics carries inline topic labels — valid on any non-annotation entry.
	// Empty for kind: annotation entries (those use AnnotationTopics, which
	// supports the richer "label or {label, members}" form). Each path is
	// parsed and validated at load time; invalid components surface as
	// Warnings rather than failing the parse, matching how other shape rules
	// are handled.
	Topics []TopicPath
	// AnnotationTopics carries the topic assignments declared by a
	// kind: annotation entry. Each item is either a plain label (Members nil
	// — applies to all of the annotation's Refs) or a label with explicit
	// member sub-selection (Members must be a subset of Refs; checked at
	// pre-flight time).
	AnnotationTopics []AnnotationTopic
	// FocusActors is the focus-level default actor list (canonical-only),
	// used for involvement triples that don't carry their own actors override.
	// Only meaningful on kind: focus decisions.
	FocusActors []string
	// FocusWhen is the focus-level default temporal scope. Per-involvement
	// `when:` overrides this; involvement without `when:` inherits this value.
	// Only meaningful on kind: focus decisions.
	FocusWhen *FocusWhen
	// Involvement is the list of involvement triples on a kind: focus
	// decision. Each triple binds a target entry to (resolved) actors and
	// (resolved) when scope.
	Involvement []Involvement
	Preflight   string    // "skipped" or "error" annotation from pre-flight validation
	Attachments []string  // filenames discovered from the co-located attachment directory
	Summary     string    // LLM-generated summary: this entry + direct relationships
	SummaryHash string    // hex-encoded hash of the rendered summary prompt inputs
	Warnings    []Warning // validation issues found during graph construction
	// Embedded marks a base entry compiled into the sdd binary (base
	// procedures) rather than loaded from the graph directory. Set by the
	// loader, never serialized. Write-side surfaces (summary regeneration,
	// summary-hash lint) skip embedded entries — there is no file to write.
	Embedded bool
}

// IsContract returns true if this decision is a standing constraint.
func (e *Entry) IsContract() bool {
	return e.Kind == KindContract
}

// IsPlan returns true if this decision is an implementation plan.
func (e *Entry) IsPlan() bool {
	return e.Kind == KindPlan
}

// IsAspiration returns true if this decision is a perpetual direction.
func (e *Entry) IsAspiration() bool {
	return e.Kind == KindAspiration
}

// IsActor returns true if this signal records a participant identity.
func (e *Entry) IsActor() bool {
	return e.Type == TypeSignal && e.Kind == KindActor
}

// IsRole returns true if this decision commits a participation pattern.
func (e *Entry) IsRole() bool {
	return e.Type == TypeDecision && e.Kind == KindRole
}

// IsProcedure returns true if this decision defines a playbook move — a
// canonical-named, process-pinned state machine executed by the workflow
// engine.
func (e *Entry) IsProcedure() bool {
	return e.Type == TypeDecision && e.Kind == KindProcedure
}

// ProcedureClass classifies a procedure's execution role. See Entry.Class.
type ProcedureClass string

const (
	ProcedureClassMove  ProcedureClass = "move"
	ProcedureClassShell ProcedureClass = "shell"
	// ProcedureClassTask is a procedure a move delegates work to: dispatched
	// with resolved params, no user choosers, kept off the shell's move
	// enumeration and junction offers, and preferring a disposable (forked)
	// context. Explore is its first member.
	ProcedureClassTask ProcedureClass = "task"
)

// IsShellProcedure returns true if this procedure is a session shell —
// auto-started by the session door rather than startable as a move.
func (e *Entry) IsShellProcedure() bool {
	return e.IsProcedure() && e.Class == ProcedureClassShell
}

// IsTaskProcedure returns true if this procedure is a task — a delegate move
// dispatched to a disposable context, kept off the shell's offered moves.
func (e *Entry) IsTaskProcedure() bool {
	return e.IsProcedure() && e.Class == ProcedureClassTask
}

// FirstSummarySentence returns the leading sentence of the entry's stored
// summary, falling back to the body when no summary is stored — the
// one-line micro-summary shared by show trees, brief entry lines, and
// serve-side enumerations. A sentence ends at the first ". " or line
// break, whichever comes first.
func (e *Entry) FirstSummarySentence() string {
	src := e.Summary
	if src == "" {
		src = e.Content
	}
	src = strings.TrimSpace(src)
	if i := strings.IndexByte(src, '\n'); i >= 0 {
		src = strings.TrimSpace(src[:i])
	}
	if i := strings.Index(src, ". "); i >= 0 {
		src = src[:i+1]
	}
	return src
}

// IsSettled returns true if this is a directive born terminal via
// intent: settled — a decision that needs no follow-up and carries no closing
// edge. Intent is only meaningful on directives, so the kind guard keeps a
// stray intent on another kind from being read as terminal.
func (e *Entry) IsSettled() bool {
	return e.Type == TypeDecision && e.Kind == KindDirective && e.Intent == IntentSettled
}

// frontmatter is the YAML structure in the file header.
//
// Per-kind fields (Canonical/Aliases on actor; Actor on role; Topics on
// non-annotation; AnnotationTopics on annotation; FocusActors/FocusWhen/
// Involvement on focus) are kept on a single struct because YAML decoding
// has no knowledge of kind at parse time. The kind-conditional shape rules
// are enforced after the fact in ParseEntry — we let YAML decode whatever
// is present, then route fields into the Entry based on Kind.
type frontmatter struct {
	Type         string            `yaml:"type"`
	Layer        string            `yaml:"layer"`
	Kind         string            `yaml:"kind,omitempty"`
	Refs         []Ref             `yaml:"refs,omitempty"`
	Supersedes   idOnlyList        `yaml:"supersedes,omitempty"`
	Closes       idOnlyList        `yaml:"closes,omitempty"`
	Participants []string          `yaml:"participants,omitempty"`
	Confidence   string            `yaml:"confidence,omitempty"`
	Intent       string            `yaml:"intent,omitempty"`
	Canonical    string            `yaml:"canonical,omitempty"`
	Aliases      []string          `yaml:"aliases,omitempty"`
	Class        string            `yaml:"class,omitempty"`
	Actor        string            `yaml:"actor,omitempty"`
	Topics       []AnnotationTopic `yaml:"topics,omitempty"`
	FocusActors  []string          `yaml:"actors,omitempty"`
	FocusWhen    *FocusWhen        `yaml:"when,omitempty"`
	Involvement  []involvementYAML `yaml:"involvement,omitempty"`
	Params       yaml.Node         `yaml:"params,omitempty"`
	State        yaml.Node         `yaml:"state,omitempty"`
	Steps        yaml.Node         `yaml:"steps,omitempty"`
	Preflight    string            `yaml:"preflight,omitempty"`
	Summary      string            `yaml:"summary,omitempty"`
	SummaryHash  string            `yaml:"summary_hash,omitempty"`
}

// involvementYAML mirrors the on-disk shape for involvement triples. The
// `actorsSet` flag is computed during parse based on whether the YAML
// contained the `actors:` key at all — distinguishing "inherit focus-level
// default" (omitted) from "deliberately empty / pull-available" (explicit
// `actors: []`).
type involvementYAML struct {
	Target string     `yaml:"target"`
	Actors *[]string  `yaml:"actors,omitempty"`
	When   *FocusWhen `yaml:"when,omitempty"`
}

// ParseEntry parses a graph entry from its filename and file content.
func ParseEntry(filename, content string) (*Entry, error) {
	id := strings.TrimSuffix(filename, ".md")

	idParts, err := ParseID(id)
	if err != nil {
		return nil, fmt.Errorf("parsing ID %q: %w", id, err)
	}

	fm, body, err := parseFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("parsing frontmatter in %q: %w", filename, err)
	}

	entryType, err := parseEntryType(fm.Type)
	if err != nil {
		return nil, fmt.Errorf("parsing frontmatter in %q: %w", filename, err)
	}

	layer, ok := LayerFromAbbrev[fm.Layer]
	if !ok {
		layer = Layer(fm.Layer)
	}

	e := &Entry{
		ID:           id,
		Type:         entryType,
		Layer:        layer,
		Kind:         Kind(fm.Kind),
		Refs:         fm.Refs,
		Supersedes:   []string(fm.Supersedes),
		Closes:       []string(fm.Closes),
		Participants: fm.Participants,
		Confidence:   fm.Confidence,
		Intent:       Intent(fm.Intent),
		Canonical:    fm.Canonical,
		Aliases:      fm.Aliases,
		Class:        ProcedureClass(fm.Class),
		Actor:        fm.Actor,
		FocusActors:  fm.FocusActors,
		FocusWhen:    fm.FocusWhen,
		Preflight:    fm.Preflight,
		Summary:      fm.Summary,
		SummaryHash:  fm.SummaryHash,
		Content:      strings.TrimSpace(body),
		Time:         idParts.Time,
	}

	// Route topics:[] based on kind. Annotation entries keep the rich shape
	// (label, optional members); everything else flattens to plain labels and
	// any members entry indicates a malformed inline use.
	if e.IsAnnotation() {
		e.AnnotationTopics = fm.Topics
	} else if len(fm.Topics) > 0 {
		paths := make([]TopicPath, 0, len(fm.Topics))
		for _, t := range fm.Topics {
			if len(t.Members) > 0 {
				e.Warnings = append(e.Warnings, Warning{
					Field:   "topics",
					Value:   t.Label,
					Message: fmt.Sprintf("inline topics on non-annotation entry must be plain labels; got members on %q", t.Label),
				})
				continue
			}
			p, err := ParseTopicPath(t.Label)
			if err != nil {
				e.Warnings = append(e.Warnings, Warning{
					Field:   "topics",
					Value:   t.Label,
					Message: err.Error(),
				})
				continue
			}
			paths = append(paths, p)
		}
		e.Topics = paths
	}

	// Retain the machine part of a procedure's frontmatter as raw YAML.
	// Routed by kind like the other per-kind fields; on any other kind the
	// keys are ignored, matching how unknown frontmatter keys behave.
	if e.IsProcedure() && (!fm.Params.IsZero() || !fm.State.IsZero() || !fm.Steps.IsZero()) {
		e.ProcedureSpec = &ProcedureSpecRaw{
			Params: fm.Params,
			State:  fm.State,
			Steps:  fm.Steps,
		}
	}

	// Lift involvement triples into Entry.Involvement, preserving the
	// "actors omitted vs empty" distinction.
	if len(fm.Involvement) > 0 {
		e.Involvement = make([]Involvement, 0, len(fm.Involvement))
		for _, iv := range fm.Involvement {
			out := Involvement{
				Target: iv.Target,
				When:   iv.When,
			}
			if iv.Actors != nil {
				out.Actors = *iv.Actors
				out.ActorsSet = true
			}
			e.Involvement = append(e.Involvement, out)
		}
	}

	return e, nil
}

// IDParts holds the parsed components of a document ID.
type IDParts struct {
	Timestamp string
	Time      time.Time
	TypeCode  string // abbreviation: "s" or "d"
	LayerCode string // abbreviation: "stg", "cpt", "tac", "ops", "prc"
	Suffix    string
}

// ParseID parses a document ID into its components.
// ID format: {YYYYMMDD}-{HHmmss}-{type}-{layer}-{suffix}
func ParseID(id string) (IDParts, error) {
	dashes := []int{}
	for i, c := range id {
		if c == '-' {
			dashes = append(dashes, i)
		}
	}
	if len(dashes) < 4 {
		return IDParts{}, fmt.Errorf("invalid ID format: %q (need at least 4 dashes)", id)
	}

	timestamp := id[:dashes[1]]
	t, err := time.Parse("20060102-150405", timestamp)
	if err != nil {
		return IDParts{}, fmt.Errorf("parsing time from %q: %w", id, err)
	}

	return IDParts{
		Timestamp: timestamp,
		Time:      t,
		TypeCode:  id[dashes[1]+1 : dashes[2]],
		LayerCode: id[dashes[2]+1 : dashes[3]],
		Suffix:    id[dashes[3]+1:],
	}, nil
}

// IDToRelPath converts an entry ID to its relative file path in the hierarchical layout.
// ID format: YYYYMMDD-HHmmss-type-layer-suffix
// Path format: YYYY/MM/DD-HHmmss-type-layer-suffix.md
func IDToRelPath(id string) (string, error) {
	if len(id) < 8 {
		return "", fmt.Errorf("ID too short: %q", id)
	}
	yyyy := id[0:4]
	mm := id[4:6]
	shortName := id[6:] // DD-HHmmss-type-layer-suffix
	return filepath.Join(yyyy, mm, shortName+".md"), nil
}

// RelPathToID converts a relative path (YYYY/MM/DD-HHmmss-type-layer-suffix.md) to a full entry ID.
func RelPathToID(rel string) (string, error) {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) != 3 {
		return "", fmt.Errorf("expected YYYY/MM/filename.md, got %q", rel)
	}
	yyyy := parts[0]
	mm := parts[1]
	filename := strings.TrimSuffix(parts[2], ".md")
	return yyyy + mm + filename, nil
}

// AttachDirRelPath returns the relative path to the attachment directory for an entry ID.
// This is the entry's file path without the .md extension.
func AttachDirRelPath(id string) (string, error) {
	rel, err := IDToRelPath(id)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(rel, ".md"), nil
}

// ResolveAttachmentLinks replaces {{attachments}} placeholders in content with the
// actual relative directory path for markdown links.
func ResolveAttachmentLinks(content, id string) string {
	if len(id) < 8 {
		return content
	}
	// The short filename (without YYYYMM prefix and .md) serves as the directory name
	// relative to the entry file in the same directory.
	shortName := id[6:] // DD-HHmmss-type-layer-suffix
	return strings.ReplaceAll(content, "{{attachments}}", "./"+shortName)
}

// parseEntryType resolves a frontmatter type string to a canonical EntryType.
// Accepts both abbreviated ("s", "d") and full ("signal", "decision") forms.
// Unknown values return an error — the loader is the last line of defence
// against stale or malformed graph entries.
func parseEntryType(s string) (EntryType, error) {
	switch s {
	case "s", "signal":
		return TypeSignal, nil
	case "d", "decision":
		return TypeDecision, nil
	case "":
		return "", fmt.Errorf("missing type field")
	default:
		return "", fmt.Errorf("unknown type %q (expected signal or decision)", s)
	}
}

// parseFrontmatter splits content into YAML frontmatter and body.
func parseFrontmatter(content string) (*frontmatter, string, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return nil, content, fmt.Errorf("missing frontmatter delimiter")
	}

	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, content, fmt.Errorf("missing closing frontmatter delimiter")
	}

	yamlContent := rest[:idx]
	body := rest[idx+4:] // skip \n---

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return nil, "", fmt.Errorf("parsing YAML: %w", err)
	}

	return &fm, body, nil
}

// FormatFrontmatter creates the YAML frontmatter string for an entry.
func FormatFrontmatter(e *Entry) string {
	fm := frontmatter{
		Type:         string(e.Type),
		Layer:        string(e.Layer),
		Kind:         string(e.Kind),
		Refs:         e.Refs,
		Supersedes:   idOnlyList(e.Supersedes),
		Closes:       idOnlyList(e.Closes),
		Participants: e.Participants,
		Confidence:   e.Confidence,
		Intent:       string(e.Intent),
		Canonical:    e.Canonical,
		Aliases:      e.Aliases,
		Class:        string(e.Class),
		Actor:        e.Actor,
		FocusActors:  e.FocusActors,
		FocusWhen:    e.FocusWhen,
		Preflight:    e.Preflight,
		Summary:      e.Summary,
		SummaryHash:  e.SummaryHash,
	}

	// Topics: annotation entries emit AnnotationTopics verbatim (preserves
	// the per-item string-or-mapping form via AnnotationTopic.MarshalYAML).
	// Other kinds emit plain labels by lifting Topics into the same field
	// shape with empty Members.
	switch {
	case e.IsAnnotation() && len(e.AnnotationTopics) > 0:
		fm.Topics = e.AnnotationTopics
	case len(e.Topics) > 0:
		fm.Topics = make([]AnnotationTopic, 0, len(e.Topics))
		for _, p := range e.Topics {
			fm.Topics = append(fm.Topics, AnnotationTopic{Label: p.String()})
		}
	}

	if len(e.Involvement) > 0 {
		fm.Involvement = make([]involvementYAML, 0, len(e.Involvement))
		for _, inv := range e.Involvement {
			out := involvementYAML{
				Target: inv.Target,
				When:   inv.When,
			}
			if inv.ActorsSet {
				actors := inv.Actors
				out.Actors = &actors
			}
			fm.Involvement = append(fm.Involvement, out)
		}
	}

	if e.ProcedureSpec != nil {
		fm.Params = e.ProcedureSpec.Params
		fm.State = e.ProcedureSpec.State
		fm.Steps = e.ProcedureSpec.Steps
	}

	data, _ := yaml.Marshal(&fm)
	return "---\n" + string(data) + "---\n"
}

// GenerateID creates a new document ID with the current timestamp and a random suffix.
func GenerateID(typ EntryType, layer Layer, suffix string) string {
	return GenerateIDAt(typ, layer, suffix, time.Now())
}

// RewriteID returns the id with its type abbreviation replaced by newType's
// abbreviation. Timestamp, layer, and suffix are preserved. Used by the
// sdd rewrite command for mechanical type changes.
func RewriteID(id string, newType EntryType) (string, error) {
	parts, err := ParseID(id)
	if err != nil {
		return "", err
	}
	newAbbrev, ok := TypeAbbrev[newType]
	if !ok {
		return "", fmt.Errorf("unknown type: %s", newType)
	}
	return fmt.Sprintf("%s-%s-%s-%s", parts.Timestamp, newAbbrev, parts.LayerCode, parts.Suffix), nil
}

// GenerateIDAt creates a new document ID with the given timestamp and a random suffix.
// Accepts the time explicitly so callers can inject a clock for testability.
func GenerateIDAt(typ EntryType, layer Layer, suffix string, t time.Time) string {
	ta := TypeAbbrev[typ]
	la := LayerAbbrev[layer]
	return fmt.Sprintf("%s-%s-%s-%s", t.Format("20060102-150405"), ta, la, suffix)
}

// RandomSuffix returns an n-character lowercase alphanumeric string suitable
// for use as the trailing random portion of a document ID.
func RandomSuffix(n int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[b[i]%byte(len(charset))]
	}
	return string(b), nil
}

// TypeLabel returns a display label for the entry type.
func (e *Entry) TypeLabel() string {
	switch e.Type {
	case TypeSignal:
		return "signal"
	case TypeDecision:
		return "decision"
	default:
		return string(e.Type)
	}
}

// LayerLabel returns a display label for the layer.
func (e *Entry) LayerLabel() string {
	return string(e.Layer)
}

// ShortContent returns content truncated to maxLen, preferring sentence boundaries.
// Accumulates complete sentences up to the limit. If no sentence fits, accumulates words.
func (e *Entry) ShortContent(maxLen int) string {
	line := e.Content
	if idx := strings.Index(line, "\n"); idx >= 0 {
		line = line[:idx]
	}
	if len(line) <= maxLen {
		return line
	}

	// Try to accumulate sentences
	sentences := splitSentences(line)
	if len(sentences) > 1 {
		result := sentences[0]
		included := 1
		for _, s := range sentences[1:] {
			candidate := result + " " + s
			if len(candidate) > maxLen {
				break
			}
			result = candidate
			included++
		}
		if included < len(sentences) && len(result)+4 <= maxLen {
			result += " ..."
		}
		if len(result) <= maxLen {
			return result
		}
	}

	// Fall back to accumulating words
	words := strings.Fields(line)
	result := words[0]
	for _, w := range words[1:] {
		candidate := result + " " + w
		if len(candidate)+4 > maxLen { // +4 for " ..."
			break
		}
		result = candidate
	}
	return result + " ..."
}

// splitSentences splits text on sentence-ending punctuation followed by a space.
func splitSentences(text string) []string {
	var sentences []string
	start := 0
	for i := 0; i < len(text)-1; i++ {
		if (text[i] == '.' || text[i] == '!' || text[i] == '?') && text[i+1] == ' ' {
			sentences = append(sentences, text[start:i+1])
			start = i + 2
		}
	}
	if start < len(text) {
		sentences = append(sentences, text[start:])
	}
	return sentences
}
