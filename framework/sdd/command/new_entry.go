// Package command holds domain command structs — write intent. Commands are
// dispatched to handlers for execution; results flow back through optional
// callback functions on the command struct (handlers themselves return only
// errors so write paths and read paths stay distinct).
package command

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/networkteam/resonance/framework/sdd/model"
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
	Kind         model.Kind // only meaningful for decisions
	Description  string
	Refs         []string
	Supersedes   []string
	Closes       []string
	Participants []string
	Confidence   string
	Attachments  []Attachment

	SkipPreflight    bool
	DryRun           bool
	PreflightTimeout time.Duration
	PreflightModel   string

	// OnNewEntry is invoked with the new entry's ID on successful creation.
	// Not invoked on dry-run or any failure path. The callback receives
	// only the ID — for richer data (path, content), the caller issues a
	// query against the appropriate finder.
	OnNewEntry func(id string)
}

// Validate checks that required fields are populated and internally
// consistent. Richer validation (refs exist in graph, type constraints on
// closes, etc.) is the handler's job against a loaded graph.
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
	if c.Kind != "" && c.Type != model.TypeDecision {
		return fmt.Errorf("kind is only meaningful for decisions (got type %s)", c.Type)
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
		Content:      c.Description,
	}

	// Decisions default to directive when no kind is specified.
	if entry.Type == model.TypeDecision && entry.Kind == "" {
		entry.Kind = model.KindDirective
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
