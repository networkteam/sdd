// Package command holds domain command structs — write intent. Commands are
// dispatched to handlers for execution; results flow back through optional
// callback functions on the command struct (handlers themselves return only
// errors so write paths and read paths stay distinct).
package command

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// Attachment describes a file to attach to a new entry. For stdin attachments
// the source is "-" and Data holds the already-read bytes — the CLI layer
// materializes stdin before constructing the command so the handler operates
// on a self-contained value.
type Attachment struct {
	Source string // file path, or "-" for stdin
	Target string // destination filename inside the attachment directory
	Data   []byte // populated when Source == "-"
}

// NewEntryCmd captures intent to create a new graph entry.
// The handler is responsible for graph loading, validation, pre-flight,
// stdin persistence on rejection/dry-run, writing the entry file, copying
// attachments, and committing. On success the handler invokes OnNewEntry
// with the new entry's ID; the caller queries a finder for any richer data.
type NewEntryCmd struct {
	Type         model.EntryType
	Layer        model.Layer
	Kind         model.Kind // empty is replaced by the type's default in BuildEntry
	Description  string
	Refs         []model.Ref
	Supersedes   []string
	Closes       []string
	Participants []string
	Confidence   string

	// Intent is the directive lifecycle posture (pending|guiding|settled).
	// Required on new directive captures, rejected on any other kind. Validated
	// in Validate so every write path (CLI and the MCP server) enforces it.
	Intent string

	Attachments []Attachment

	// Canonical and Aliases are only meaningful for kind: actor signals —
	// copied into the written frontmatter so downstream readers can
	// resolve participant identity. Ignored on non-actor entries; the
	// model-layer validator flags a missing canonical on an actor.
	Canonical string
	Aliases   []string

	// Actor is only meaningful for kind: role decisions — names the
	// canonical of the actor-identity chain the role binds to. Ignored
	// on non-role entries.
	Actor string

	// Class is only meaningful for kind: procedure decisions — the
	// procedure's execution role (move or shell; empty means move). Lets a
	// project supersede a shell procedure through normal capture.
	Class string

	// TopicLabels carries inline topic labels (CSV form on the CLI; flat
	// list here). Valid on any non-annotation entry. Stored verbatim;
	// parsing into TopicPath happens during BuildEntry so the command
	// surface stays simple.
	TopicLabels []string
	Index       *model.FactIndex

	// AnnotationTopics carries the topic assignments for a kind: annotation
	// entry. Each --topic flag at the CLI parses into one entry here. The
	// item shape supports plain label (members empty → applies to all refs)
	// or label + member sub-selection.
	AnnotationTopics []model.AnnotationTopic

	// FocusActors is the focus-level default actor list (canonical-only).
	// Only meaningful on kind: focus decisions.
	FocusActors []string

	// FocusWhen is the focus-level default temporal scope. Per-involvement
	// when overrides this. Only meaningful on kind: focus decisions.
	FocusWhen *model.FocusWhen

	// Involvement is the list of involvement triples for a kind: focus
	// decision. Each --involvement flag parses into one entry here.
	Involvement []model.Involvement

	SkipPreflight    bool
	DryRun           bool
	PreflightTimeout time.Duration
	PreflightModel   string

	// Summary supplies the entry's stored summary verbatim, skipping LLM
	// generation entirely. For callers that already hold a faithful
	// summary — or environments without a reachable LLM. Empty means
	// "generate via the configured runner".
	Summary string

	// OnNewEntry is invoked with the new entry's ID and the LLM-generated
	// summary on successful creation. Summary is empty when the LLM call
	// failed or was skipped (dry-run path returns before invoking). Not
	// invoked on dry-run or any failure path. For richer data (path,
	// content), the caller issues a query against the appropriate finder.
	OnNewEntry func(id, summary string)

	// OnPreflight is invoked with the pre-flight result whenever the
	// validator ran to completion — including when its findings block the
	// entry. Not invoked on skip, hard validator errors, or paths that never
	// reach pre-flight. Transport layers that need findings structurally
	// (MCP) hook this; the CLI relies on the handler's stderr rendering.
	OnPreflight func(result *query.PreflightResult)
}

// Validate checks what must hold before an entry ID can even be generated:
// type and layer. Everything per-kind — intent, class, index, kind validity —
// is the construction boundary's job (model.EntryConstruction.ValidateForWrite),
// which the handler runs against the loaded graph.
func (c *NewEntryCmd) Validate() error {
	if c.Type == "" {
		return fmt.Errorf("type is required")
	}
	if _, ok := model.TypeAbbrev[c.Type]; !ok {
		return fmt.Errorf("invalid type: %s", c.Type)
	}
	if c.Layer == "" {
		return fmt.Errorf("layer is required")
	}
	if _, ok := model.LayerAbbrev[c.Layer]; !ok {
		return fmt.Errorf("invalid layer: %s", c.Layer)
	}
	return nil
}

// BuildEntry constructs a model.Entry from the command fields, applying
// defaults (Kind → directive for decisions) and resolving attachment paths
// and content links. The caller provides the generated entry ID.
func (c *NewEntryCmd) BuildEntry(id string) (*model.Entry, error) {
	entry := &model.Entry{
		ID:           id,
		Type:         c.Type,
		Layer:        c.Layer,
		Kind:         c.Kind,
		Refs:         c.Refs,
		Supersedes:   c.Supersedes,
		Closes:       c.Closes,
		Participants: c.Participants,
		Confidence:   c.Confidence,
		Intent:       model.Intent(c.Intent),
		Canonical:    c.Canonical,
		Aliases:      c.Aliases,
		Class:        model.ProcedureClass(c.Class),
		Actor:        c.Actor,
		Index:        c.Index,
		FocusActors:  c.FocusActors,
		FocusWhen:    c.FocusWhen,
		Involvement:  c.Involvement,
		Content:      c.Description,
	}

	// Apply per-type default when no kind is specified.
	if entry.Kind == "" {
		entry.Kind = model.DefaultKindForType(entry.Type)
	}

	// Topics routing: annotation entries take the rich AnnotationTopics
	// form; everything else parses TopicLabels into inline TopicPaths. A
	// label that fails to parse returns an error here so the user gets
	// actionable feedback at command construction rather than as a load-
	// time warning on the written entry.
	if entry.IsAnnotation() {
		entry.AnnotationTopics = c.AnnotationTopics
	} else if len(c.TopicLabels) > 0 {
		paths := make([]model.TopicPath, 0, len(c.TopicLabels))
		for _, label := range c.TopicLabels {
			p, err := model.ParseTopicPath(label)
			if err != nil {
				return nil, fmt.Errorf("topics: %w", err)
			}
			paths = append(paths, p)
		}
		entry.Topics = paths
	}

	if len(c.Attachments) > 0 {
		attachRel, err := model.AttachDirRelPath(id)
		if err != nil {
			return nil, fmt.Errorf("computing attachment dir for %s: %w", id, err)
		}
		for _, a := range c.Attachments {
			entry.Attachments = append(entry.Attachments, filepath.Join(attachRel, a.Target))
		}
	}

	entry.Content = model.ResolveAttachmentLinks(entry.Content, id)

	return entry, nil
}

// StdinAttachment returns the single stdin attachment, or nil if none is
// present. (parseAttachFlags enforces at most one stdin attachment at the
// CLI layer.)
func (c *NewEntryCmd) StdinAttachment() *Attachment {
	for i := range c.Attachments {
		if c.Attachments[i].Source == "-" {
			return &c.Attachments[i]
		}
	}
	return nil
}
