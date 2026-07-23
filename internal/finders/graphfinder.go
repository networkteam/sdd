package finders

import (
	"fmt"
	"sort"

	"github.com/networkteam/sdd/internal/model"
	"gopkg.in/yaml.v3"
)

// DocumentSource is the storage-neutral seam every graph read enters through:
// it supplies the documents a graph is built from, regardless of where the
// bytes live. The filesystem source reads .md files, a structured source wraps
// application.SnapshotData, a future DB source queries rows — each owns its own
// encoding and decoding. Content a source cannot decode is reported as an
// unreadable document (data the read serves), never a hard failure that kills
// the whole load.
//
// "Store = where bytes live (host-specific); GraphFinder = what the graph
// means (shared)." The DocumentSource is the store's face; the GraphFinder is
// the meaning.
type DocumentSource interface {
	GraphDocuments() (GraphDocuments, error)
}

// GraphDocuments is the full storage-neutral document set a source hands the
// GraphFinder: entry documents, WIP marker documents, and issues for content
// the source could not decode into a document.
type GraphDocuments struct {
	Entries    []EntryDocument
	WIP        []WIPDocument
	Unreadable []DocumentIssue
}

// EntryDocument is the storage-neutral form of one graph entry. It is a union
// mirroring application.DocumentChange: it carries EITHER raw canonical content
// (Raw: markdown + frontmatter bytes) OR structured fields (Frontmatter + Body).
// Exactly one form is set. Sources own their encoding — the filesystem source
// hands raw content (so the CLI path does no double YAML parse and stays
// byte-identical to the direct loader), a structured source (SnapshotData, a
// future DB) hands structured fields.
type EntryDocument struct {
	// ID is the entry's full logical ID (no ".md"). Both forms carry it.
	ID string
	// Raw is the canonical stored content (frontmatter + body). When non-nil,
	// this form is used and the structured fields are ignored.
	Raw []byte
	// Frontmatter and Body are the structured form, used when Raw is nil.
	Frontmatter map[string]any
	Body        string
	// Attachments are graph-relative attachment paths, carried in both forms.
	Attachments []string
}

// WIPDocument is the storage-neutral form of one WIP marker: its file name
// (e.g. "handle.md") and raw content.
type WIPDocument struct {
	Name    string
	Content string
}

// DocumentIssue records content a source could not decode into a document: its
// logical path (or ref) and the decode error message. The GraphFinder turns
// each into a model.LoadIssue.
type DocumentIssue struct {
	Ref     string
	Message string
}

// GraphFinder is the single shared graph reader. It holds a built model.Graph
// (and any WIP markers) and serves every read against it; the query-struct
// methods carry pure intent while the finder supplies the graph. Load errors
// are data it serves through Health, never a reason it dies: a document that
// fails to parse becomes a model.LoadIssue on the held graph and the rest still
// loads. It composes an inner *Finder for read dependencies (config, pre-flight
// runner, connected-repos registry), so each read is a single implementation.
type GraphFinder struct {
	finder *Finder
	graph  *model.Graph
	wip    []*model.WIPMarker
}

// OnGraph wraps an already-built graph so the finder's read methods operate on
// it with intent-only queries. Used by callers that already hold a graph — the
// engine's *model.Graph, a MultiGraph-assembled clone, the CLI's CurrentGraph,
// tests — rather than building one from a document source. WIP markers are
// resolved lazily from the graph's directory when a view needs them; callers
// with markers in hand attach them via WithWIP.
func (f *Finder) OnGraph(g *model.Graph) *GraphFinder {
	return &GraphFinder{finder: f, graph: g}
}

// WithWIP attaches WIP markers the caller already holds (the snapshot path,
// whose graph carries no on-disk directory to lazy-load from). Returns the
// receiver for chaining.
func (gf *GraphFinder) WithWIP(wip []*model.WIPMarker) *GraphFinder {
	gf.wip = wip
	return gf
}

// NewGraphFinder builds a GraphFinder from a document source — the one unified
// construction path both graph worlds (filesystem CLI loads, structured
// snapshots) flow through. Per-document parse failures and source-reported
// unreadable documents become LoadIssues on the held graph, never an abort.
// Base-entry assembly stays a hard error (the embedded set is compile-time
// shaped, so a failure is a broken build) and a malformed WIP marker stays a
// hard error (explicitly parked policy — current strictness kept).
func NewGraphFinder(source DocumentSource, opts Options) (*GraphFinder, error) {
	docs, err := source.GraphDocuments()
	if err != nil {
		return nil, err
	}
	graph, wip, err := buildGraph(docs)
	if err != nil {
		return nil, err
	}
	return &GraphFinder{finder: New(opts), graph: graph, wip: wip}, nil
}

// buildGraph is the single semantic gate every graph read flows through,
// regardless of where the bytes came from. Raw-form documents parse directly;
// structured-form documents render to canonical YAML first so the one
// validation path (model.ParseEntry, with its custom unmarshalers) stays the
// only entry gate — decoding the frontmatter map straight into an Entry is a
// possible later optimization. Per-document parse failures and source-reported
// unreadable documents become LoadIssues; base-entry failure and malformed WIP
// markers stay hard errors.
func buildGraph(docs GraphDocuments) (*model.Graph, []*model.WIPMarker, error) {
	entries := make([]*model.Entry, 0, len(docs.Entries))
	var loadIssues []model.LoadIssue
	for _, issue := range docs.Unreadable {
		loadIssues = append(loadIssues, model.LoadIssue{Ref: issue.Ref, Message: issue.Message})
	}
	onDisk := make(map[string]bool, len(docs.Entries))
	for _, doc := range docs.Entries {
		entry, err := parseEntryDocument(doc)
		if err != nil {
			// A malformed entry never aborts the build: it is recorded as a
			// load issue on the graph and surfaced by read paths (lint, the
			// session serve). Everything parseable still loads.
			loadIssues = append(loadIssues, model.LoadIssue{Ref: doc.ID, Message: err.Error()})
			continue
		}
		entries = append(entries, entry)
		onDisk[entry.ID] = true
	}

	// Join the embedded base entries (procedures + facts) shipped in the
	// binary, disk-wins: a project owns its IDs. The set is compile-time
	// shaped, so a load error is a broken build.
	base, err := BaseEntries()
	if err != nil {
		return nil, nil, err
	}
	entries = model.MergeEmbedded(entries, onDisk, base)
	graph := model.NewGraphWithLoadIssues(entries, loadIssues)

	markers := make([]*model.WIPMarker, 0, len(docs.WIP))
	for _, doc := range docs.WIP {
		marker, err := model.ParseWIPMarker(doc.Name, doc.Content)
		if err != nil {
			return nil, nil, fmt.Errorf("parsing WIP marker %q: %w", doc.Name, err)
		}
		markers = append(markers, marker)
	}
	sort.Slice(markers, func(i, j int) bool { return markers[i].Time.Before(markers[j].Time) })

	return graph, markers, nil
}

// parseEntryDocument runs one document through the single validation gate. Raw
// content parses directly; a structured document is rendered to canonical YAML
// first so both forms converge on model.ParseEntry.
func parseEntryDocument(doc EntryDocument) (*model.Entry, error) {
	content := string(doc.Raw)
	if doc.Raw == nil {
		frontmatter, err := yaml.Marshal(doc.Frontmatter)
		if err != nil {
			return nil, fmt.Errorf("encoding frontmatter: %w", err)
		}
		content = "---\n" + string(frontmatter) + "---\n\n" + doc.Body
	}
	entry, err := model.ParseEntry(doc.ID+".md", content)
	if err != nil {
		return nil, err
	}
	entry.Attachments = append([]string(nil), doc.Attachments...)
	return entry, nil
}

// Graph returns the held graph. The same-module read seams (the engine Graphs
// provider, presenters) consume a *model.Graph directly.
func (gf *GraphFinder) Graph() *model.Graph {
	return gf.graph
}

// Health flattens the held graph's load failures and per-entry warnings into a
// summary — load errors are data the finder serves, not a reason it dies.
func (gf *GraphFinder) Health() model.GraphHealth {
	return gf.graph.Health()
}

// WIPMarkers returns the finder's WIP markers, resolving them lazily from the
// held graph's directory when none were attached (the CLI path). Callers that
// attached markers via WithWIP (the snapshot path) get those back directly.
func (gf *GraphFinder) WIPMarkers() ([]*model.WIPMarker, error) {
	if gf.wip != nil {
		return gf.wip, nil
	}
	if dir := gf.graph.GraphDir(); dir != "" {
		markers, err := gf.finder.LoadWIPMarkers(dir)
		if err != nil {
			return nil, err
		}
		gf.wip = markers
		return markers, nil
	}
	return nil, nil
}
