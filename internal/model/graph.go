package model

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Graph holds all entries and their reference indexes.
type Graph struct {
	Entries      []*Entry
	ByID         map[string]*Entry
	RefsTo       map[string][]string // reverse index: entry ID -> IDs that reference it
	ClosedBy     map[string][]string // reverse index: entry ID -> IDs that close it
	SupersededBy map[string][]string // reverse index: entry ID -> IDs that supersede it
	// InboundRefs is RefsTo resolved through supersession, keyed by live head
	// with hop distance. Ranking-only: every other consumer needs RefsTo's
	// literal keying (d-cpt-x6z).
	InboundRefs map[string][]InboundRef
	// LoadIssues records entries the I/O loader could not parse. Reading the
	// graph never stops on a malformed entry: everything parseable still
	// loads, and each failure is kept here so read surfaces (lint, the
	// session serve) can surface it — the errors are recorded, not swallowed.
	LoadIssues []LoadIssue
	graphDir   string
	// multi back-wires the cross-graph assembly this graph belongs to (nil
	// for a standalone graph). Set by NewMultiGraph so traversal code
	// holding any *Graph can resolve cross-repo references.
	multi *MultiGraph
	// repoPrefix qualifies this graph's entries on cross-graph surfaces:
	// "<repo-id>:" for a member graph loaded from a connected repo's cache,
	// "" for the local graph. Embedded (binary-scoped) entries stay bare
	// regardless — they are identical in every member graph.
	repoPrefix string
}

// nodeKeyFor is the graph-qualified identity of an entry for rendering and
// cross-graph dedup: a member graph's own entries carry its repo prefix
// (the (repo-id, entry-id) dedup key in colon form), while embedded entries
// key by bare ID so exactly one copy ever surfaces.
func (g *Graph) nodeKeyFor(e *Entry) string {
	if g.repoPrefix == "" || e.Embedded {
		return e.ID
	}
	return g.repoPrefix + e.ID
}

// qualifyID prefixes a member-graph entry ID for display outside its graph;
// embedded entries and local-graph entries stay bare.
func (g *Graph) qualifyID(id string) string {
	if g.repoPrefix == "" {
		return id
	}
	if e, ok := g.ByID[id]; ok && e.Embedded {
		return id
	}
	return g.repoPrefix + id
}

// LoadIssue records one entry the I/O loader could not parse: the entry ID or
// path it was reading and the parse error message. It rides on the graph so a
// malformed entry is surfaced rather than aborting the whole read.
type LoadIssue struct {
	Ref     string
	Message string
}

// NewGraph builds a graph from the given entries without touching the filesystem.
func NewGraph(entries []*Entry) *Graph {
	return NewGraphWithLoadIssues(entries, nil)
}

// NewGraphWithLoadIssues builds a graph and records load issues the I/O loader
// collected while reading — entries that failed to parse. Every entry that did
// parse is still present; issues are carried on the graph for read surfaces to
// report. Production (finders.LoadGraph) and tests share this one path.
func NewGraphWithLoadIssues(entries []*Entry, issues []LoadIssue) *Graph {
	g := &Graph{
		Entries:      entries,
		ByID:         make(map[string]*Entry, len(entries)),
		RefsTo:       make(map[string][]string),
		ClosedBy:     make(map[string][]string),
		SupersededBy: make(map[string][]string),
		InboundRefs:  make(map[string][]InboundRef),
		LoadIssues:   issues,
	}

	for _, e := range entries {
		g.ByID[e.ID] = e
	}

	// Build reverse indexes. Cross-repo IDs are excluded: a remote target is
	// not part of the local reverse index, and lifecycle edges never cross
	// the repo boundary (validation warns on cross-repo closes/supersedes).
	for _, e := range entries {
		for _, ref := range e.Refs {
			if IsCrossRepoID(ref.ID) {
				continue
			}
			g.RefsTo[ref.ID] = append(g.RefsTo[ref.ID], e.ID)
		}
		for _, c := range e.Closes {
			if IsCrossRepoID(c) {
				continue
			}
			g.ClosedBy[c] = append(g.ClosedBy[c], e.ID)
		}
		for _, s := range e.Supersedes {
			if IsCrossRepoID(s) {
				continue
			}
			g.SupersededBy[s] = append(g.SupersededBy[s], e.ID)
		}
	}

	// Second pass: SupersededBy must be complete before any chain can be walked.
	for _, e := range entries {
		for _, ref := range e.Refs {
			if IsCrossRepoID(ref.ID) {
				continue
			}
			r := g.ResolveRef(ref.ID)
			head := r.Head()
			g.InboundRefs[head] = append(g.InboundRefs[head], InboundRef{Source: e.ID, Hops: r.Hops()})
		}
	}

	// Sort entries by time
	sort.Slice(g.Entries, func(i, j int) bool {
		return g.Entries[i].Time.Before(g.Entries[j].Time)
	})

	// Validate all entries and populate warnings
	g.validate()

	return g
}

// SetGraphDir records the directory the graph was loaded from. Used by IO callers
// (e.g. sdd.LoadGraph) to attach provenance after constructing the in-memory graph.
func (g *Graph) SetGraphDir(dir string) {
	g.graphDir = dir
}

// The five per-kind active-decision accessors are thin wrappers over Filter
// with OpenOnly set, so the active-set definition (closed, superseded, settled,
// role-cascade) lives in one place. Named accessors are kept over a generic
// ActiveDecisions(kind) so call sites read clearly (e.g. graph.Contracts()).

// Directives returns active directive decisions. Filter's OpenOnly excludes
// closed and superseded directives, and also settled ones — they are born
// terminal, dropped from the active set even without a closing edge.
func (g *Graph) Directives() []*Entry {
	return g.Filter(GraphFilter{Type: TypeDecision, Kind: KindDirective, OpenOnly: true})
}

// Activities returns active activity decisions. Activities are THAT-shaped
// commitments — capturing that specific work happens, independent of the
// directive-style choice of *what* to do.
func (g *Graph) Activities() []*Entry {
	return g.Filter(GraphFilter{Type: TypeDecision, Kind: KindActivity, OpenOnly: true})
}

// Plans returns active plan decisions.
func (g *Graph) Plans() []*Entry {
	return g.Filter(GraphFilter{Type: TypeDecision, Kind: KindPlan, OpenOnly: true})
}

// Contracts returns active contract decisions. Contracts retire via a same-kind
// supersede or a directive-kind decision closing them with rationale (universal
// retirement rule).
func (g *Graph) Contracts() []*Entry {
	return g.Filter(GraphFilter{Type: TypeDecision, Kind: KindContract, OpenOnly: true})
}

// Aspirations returns active aspiration decisions. Like contracts, aspirations
// are durable — they retire via supersede or close-by-directive with rationale.
func (g *Graph) Aspirations() []*Entry {
	return g.Filter(GraphFilter{Type: TypeDecision, Kind: KindAspiration, OpenOnly: true})
}

// OpenSignals returns signals that are closure-gated attention items — gaps
// awaiting a decision/done and questions awaiting dissolution. Facts,
// insights, and done signals are deliberately excluded: facts and insights
// are stable observational records (retired via directive close, not
// resolved), and done signals are terminal facts of execution. The
// allow-list shape means new signal kinds default to "not an attention
// item" rather than silently flooding the open set.
// openAttentionKinds are the signal kinds whose open entries demand
// resolution — the attention set OpenSignals filters on, declared once so
// served kind facts render the same enumeration.
var openAttentionKinds = []Kind{KindGap, KindQuestion}

// OpenAttentionKinds lists the attention kinds for surfaces that render or
// generate from the declaration instead of restating it.
func OpenAttentionKinds() []Kind {
	return append([]Kind(nil), openAttentionKinds...)
}

func (g *Graph) OpenSignals() []*Entry {
	closed := g.closedSet()
	superseded := g.supersededSet()

	var open []*Entry
	for _, e := range g.Entries {
		if e.Type != TypeSignal {
			continue
		}
		if !slices.Contains(openAttentionKinds, e.Kind) {
			continue
		}
		if !closed[e.ID] && !superseded[e.ID] {
			open = append(open, e)
		}
	}
	return open
}

// RecentDone returns the last n kind: done signals by timestamp — the activity
// stream of what was recently accomplished. Replaces the pre-two-type
// RecentActions; actions no longer exist in the two-type model.
func (g *Graph) RecentDone(n int) []*Entry {
	var done []*Entry
	for _, e := range g.Entries {
		if e.Type == TypeSignal && e.Kind == KindDone {
			done = append(done, e)
		}
	}
	if len(done) > n {
		done = done[len(done)-n:]
	}
	return done
}

// RecentInsights returns the last n kind: insight signals by timestamp —
// observational records that inform current thinking. Insights have no
// closure gate (they're retired via directive-close, not resolved), so they
// surface as their own stream rather than mixing into the actionable Open
// Signals view.
func (g *Graph) RecentInsights(n int) []*Entry {
	var insights []*Entry
	for _, e := range g.Entries {
		if e.Type == TypeSignal && e.Kind == KindInsight {
			insights = append(insights, e)
		}
	}
	if len(insights) > n {
		insights = insights[len(insights)-n:]
	}
	return insights
}

// AllParticipants returns the sorted unique set of participant names that
// appear on any entry in the graph. Empty strings are excluded. Used by
// the pre-flight participant-drift check to flag names that don't match
// any established spelling — the caller compares each proposed name
// against this set.
func (g *Graph) AllParticipants() []string {
	seen := make(map[string]struct{})
	for _, e := range g.Entries {
		for _, p := range e.Participants {
			if p == "" {
				continue
			}
			seen[p] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// RefChain returns the entry and all entries it transitively references, in dependency order.
func (g *Graph) RefChain(id string) []*Entry {
	seen := make(map[string]bool)
	var chain []*Entry

	var walk func(string)
	walk = func(eid string) {
		if seen[eid] {
			return
		}
		seen[eid] = true
		e, ok := g.ByID[eid]
		if !ok {
			return
		}
		for _, ref := range e.Refs {
			walk(ref.ID)
		}
		chain = append(chain, e)
	}

	walk(id)
	return chain
}

// GraphFilter specifies criteria for filtering graph entries.
type GraphFilter struct {
	Type     EntryType
	Layer    Layer
	Kind     Kind
	OpenOnly bool // when true, exclude closed/superseded signals and decisions
}

// Filter returns entries matching the given filter criteria. Zero-value
// fields match all. Kind matches across both signal and decision kinds —
// the two sets are disjoint, so `Kind: KindGap` selects signals and
// `Kind: KindPlan` selects decisions without further narrowing.
func (g *Graph) Filter(f GraphFilter) []*Entry {
	var closed, superseded map[string]bool
	if f.OpenOnly {
		closed = g.closedSet()
		superseded = g.supersededSet()
	}

	var result []*Entry
	for _, e := range g.Entries {
		if f.Type != "" && e.Type != f.Type {
			continue
		}
		if f.Layer != "" && e.Layer != f.Layer {
			continue
		}
		if f.Kind != "" && e.Kind != f.Kind {
			continue
		}
		if f.OpenOnly {
			switch e.Type {
			case TypeSignal, TypeDecision:
				if closed[e.ID] || superseded[e.ID] {
					continue
				}
				// A settled directive is born terminal — drop it from active
				// listings the same way closed/superseded entries are dropped,
				// even though it carries no closing edge.
				if e.IsSettled() {
					continue
				}
				// Role cascade: a role whose bound actor chain is closed
				// or whose canonical doesn't resolve is treated as not-open.
				if e.IsRole() {
					status := g.DerivedStatus(e).Kind
					if status != StatusActive {
						continue
					}
				}
			}
		}
		result = append(result, e)
	}
	return result
}

// closedSet returns the set of entry IDs that are closed by another entry.
func (g *Graph) closedSet() map[string]bool {
	set := make(map[string]bool)
	for id := range g.ClosedBy {
		set[id] = true
	}
	return set
}

// supersededSet returns the set of entry IDs that are superseded by another entry.
func (g *Graph) supersededSet() map[string]bool {
	set := make(map[string]bool)
	for _, e := range g.Entries {
		for _, s := range e.Supersedes {
			set[s] = true
		}
	}
	return set
}

// Downstream returns entries that reference, close, or supersede the given ID.
// Results are sorted by time (oldest first).
func (g *Graph) Downstream(id string) []*Entry {
	seen := make(map[string]bool)
	var result []*Entry

	add := func(eid string) {
		if seen[eid] {
			return
		}
		if e, ok := g.ByID[eid]; ok {
			seen[eid] = true
			result = append(result, e)
		}
	}

	// Entries that reference this ID
	for _, eid := range g.RefsTo[id] {
		add(eid)
	}

	// Entries that close this ID
	for _, eid := range g.ClosedBy[id] {
		add(eid)
	}

	// Entries that supersede this ID
	for _, eid := range g.SupersededBy[id] {
		add(eid)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Time.Before(result[j].Time)
	})

	return result
}

// GraphDir returns the directory the graph was loaded from.
func (g *Graph) GraphDir() string {
	return g.graphDir
}

// TopologicalOrder returns entries sorted so that every entry appears after
// all entries it references (refs, closes, supersedes). Entries with no
// references come first. This is a stable sort — entries at the same depth
// are ordered by time.
func (g *Graph) TopologicalOrder() []*Entry {
	// Compute depth: max distance from any root (entry with no refs/closes/supersedes).
	depth := make(map[string]int, len(g.Entries))
	var computeDepth func(id string) int
	computeDepth = func(id string) int {
		if d, ok := depth[id]; ok {
			return d
		}
		// Temporarily mark to detect cycles (treat as depth 0).
		depth[id] = 0
		e, ok := g.ByID[id]
		if !ok {
			return 0
		}
		maxDep := 0
		for _, ref := range e.Refs {
			if d := computeDepth(ref.ID) + 1; d > maxDep {
				maxDep = d
			}
		}
		for _, c := range e.Closes {
			if d := computeDepth(c) + 1; d > maxDep {
				maxDep = d
			}
		}
		for _, s := range e.Supersedes {
			if d := computeDepth(s) + 1; d > maxDep {
				maxDep = d
			}
		}
		depth[id] = maxDep
		return maxDep
	}
	for _, e := range g.Entries {
		computeDepth(e.ID)
	}

	// Copy and sort: by depth first, then by time (stable).
	ordered := make([]*Entry, len(g.Entries))
	copy(ordered, g.Entries)
	sort.SliceStable(ordered, func(i, j int) bool {
		di, dj := depth[ordered[i].ID], depth[ordered[j].ID]
		if di != dj {
			return di < dj
		}
		return ordered[i].Time.Before(ordered[j].Time)
	})
	return ordered
}

// ResolveID resolves a user-supplied ID string (full or short form) to a
// full entry ID. Full IDs pass through unchanged. Short form
// {type}-{layer}-{suffix} is matched against entries; a unique match
// returns the full ID, ambiguous matches return an error listing all
// candidates (sorted). Inputs that do not recognize as either shape,
// that use unknown type or layer abbreviations, or that match zero
// entries pass through unchanged so the caller's existing
// "entry not found" surface fires against the user's original text.
func (g *Graph) ResolveID(input string) (string, error) {
	if input == "" {
		return "", nil
	}
	// Cross-repo IDs (<repo-id>:<entry-id>) pass through verbatim: short-form
	// resolution only covers the local graph, and the colon form must never
	// be misread as a short ID.
	if IsCrossRepoID(input) {
		return input, nil
	}
	if _, err := ParseID(input); err == nil {
		return input, nil
	}
	parts := strings.SplitN(input, "-", 3)
	if len(parts) != 3 || parts[2] == "" {
		return input, nil
	}
	typeCode, layerCode, suffix := parts[0], parts[1], parts[2]
	if _, ok := TypeFromAbbrev[typeCode]; !ok {
		return input, nil
	}
	if _, ok := LayerFromAbbrev[layerCode]; !ok {
		return input, nil
	}
	var matches []string
	for _, e := range g.Entries {
		if TypeAbbrev[e.Type] != typeCode || LayerAbbrev[e.Layer] != layerCode {
			continue
		}
		p, err := ParseID(e.ID)
		if err != nil {
			continue
		}
		if p.Suffix == suffix {
			matches = append(matches, e.ID)
		}
	}
	switch len(matches) {
	case 0:
		return input, nil
	case 1:
		return matches[0], nil
	default:
		sort.Strings(matches)
		return "", fmt.Errorf("ambiguous short ID %q matches %d entries:\n  %s",
			input, len(matches), strings.Join(matches, "\n  "))
	}
}

// ResolveIDs resolves a slice of user-supplied IDs. Ambiguous inputs
// stop resolution and return the error; other inputs pass through the
// same semantics as ResolveID.
func (g *Graph) ResolveIDs(inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return inputs, nil
	}
	out := make([]string, len(inputs))
	for i, in := range inputs {
		resolved, err := g.ResolveID(in)
		if err != nil {
			return nil, err
		}
		out[i] = resolved
	}
	return out, nil
}

// ResolveRefIDs resolves the ID component of each Ref, preserving Kind and
// Desc. Refs resolve across the union of the local graph and its declared
// dependencies (ResolveUnionID): a bare ID matching a foreign entry expands to
// its full repo-id-prefixed form, so a ref written to an entry is always
// stored in canonical cross-repo form. Local short IDs still expand to their
// full local form, and an already-prefixed cross-repo ID passes through.
func (g *Graph) ResolveRefIDs(refs []Ref) ([]Ref, error) {
	if len(refs) == 0 {
		return refs, nil
	}
	out := make([]Ref, len(refs))
	for i, r := range refs {
		resolved, err := g.ResolveUnionID(r.ID)
		if err != nil {
			return nil, err
		}
		out[i] = Ref{ID: resolved, Kind: r.Kind, Desc: r.Desc}
	}
	return out, nil
}

// ClosureTarget describes an entry a draft closes or supersedes, at the depth
// a reader needs to tell which act the draft performs — which entry, of what
// kind, and its leading summary sentence — without the target's body.
type ClosureTarget struct {
	Relation string // closes | supersedes
	ID       string
	Type     EntryType
	Kind     Kind
	Summary  string
}

// ClosureTargets describes an entry's closure edges. An ID with no entry
// behind it is carried by ID alone rather than dropped: a reader must still
// see that the edge was declared, and an unresolvable edge is the write
// gate's business (d-cpt-uh0), not a reason to hide it here.
func (g *Graph) ClosureTargets(e *Entry) []ClosureTarget {
	if e == nil || (len(e.Closes) == 0 && len(e.Supersedes) == 0) {
		return nil
	}
	groups := []struct {
		relation string
		ids      []string
	}{{"closes", e.Closes}, {"supersedes", e.Supersedes}}
	targets := make([]ClosureTarget, 0, len(e.Closes)+len(e.Supersedes))
	for _, group := range groups {
		for _, id := range group.ids {
			t := ClosureTarget{Relation: group.relation, ID: id}
			if target := g.ByID[id]; target != nil {
				t.Type, t.Kind, t.Summary = target.Type, target.Kind, target.FirstSummarySentence()
			}
			targets = append(targets, t)
		}
	}
	return targets
}

// ResolveUnionID resolves a bare ID (short {type}-{layer}-{suffix} or
// unprefixed full form) against the flat union of the local graph and its
// declared dependencies, with no local-first precedence. Exactly one distinct
// match resolves to its canonical ID (bare for a local entry, full
// repo-id-prefixed for a foreign one); zero matches pass through unchanged so
// the caller's "not found" surface fires; more than one is a genuine
// ambiguity and errors with the candidates listed. An already-prefixed
// cross-repo ID (<repo-id>:<entry-id>) passes through verbatim — it is already
// explicit. Framework-shipped entries that appear identically in the local
// graph and every dependency collapse to their single local instance rather
// than false-colliding.
func (g *Graph) ResolveUnionID(input string) (string, error) {
	if input == "" {
		return "", nil
	}
	if IsCrossRepoID(input) {
		return input, nil
	}
	match := entryIDMatcher(input)
	if match == nil {
		// Not a recognizable ID shape — pass through so the caller's
		// "entry not found" surface fires against the original text.
		return input, nil
	}
	candidates := g.resolveCandidates(match)
	switch len(candidates) {
	case 0:
		return input, nil
	case 1:
		return candidates[0].owner.nodeKeyFor(candidates[0].entry), nil
	default:
		labels := make([]string, len(candidates))
		for i, c := range candidates {
			labels[i] = c.owner.nodeKeyFor(c.entry)
		}
		sort.Strings(labels)
		return "", fmt.Errorf("ambiguous ID %q matches %d entries across the local graph and its dependencies:\n  %s",
			input, len(candidates), strings.Join(labels, "\n  "))
	}
}

// ResolveUnionIDs resolves a slice through ResolveUnionID, stopping on the
// first ambiguity. Used by read surfaces that accept a typed ID from a user or
// agent — `sdd show` and the `show` MCP tool.
func (g *Graph) ResolveUnionIDs(inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return inputs, nil
	}
	out := make([]string, len(inputs))
	for i, in := range inputs {
		resolved, err := g.ResolveUnionID(in)
		if err != nil {
			return nil, err
		}
		out[i] = resolved
	}
	return out, nil
}

// resolvedCandidate is one entry matched during union resolution, paired with
// the graph that owns it (local or a dependency member) so the resolved ID can
// be qualified correctly.
type resolvedCandidate struct {
	entry *Entry
	owner *Graph
}

// resolveCandidates gathers the distinct entries matching the predicate across
// the local graph and its declared-dependency members. Candidates are deduped
// by full entry ID, preferring the local owner: a framework-shipped entry (the
// identical full ID merged into every member graph) collapses to its single
// local instance — so a project-superseded base entry keeps its local status —
// while entries that merely share a short ID across repos keep distinct full
// IDs and remain separate candidates, surfacing as an ambiguity.
func (g *Graph) resolveCandidates(match func(*Entry) bool) []resolvedCandidate {
	byFullID := map[string]resolvedCandidate{}
	var order []string
	collect := func(owner *Graph) {
		for _, e := range owner.Entries {
			if !match(e) {
				continue
			}
			existing, seen := byFullID[e.ID]
			if !seen {
				byFullID[e.ID] = resolvedCandidate{entry: e, owner: owner}
				order = append(order, e.ID)
				continue
			}
			// Same full ID already collected — the framework-entry / project
			// -override case. Prefer the local instance so its local status
			// (and any project supersession of a base entry) is what resolves.
			if existing.owner.repoPrefix != "" && owner.repoPrefix == "" {
				byFullID[e.ID] = resolvedCandidate{entry: e, owner: owner}
			}
		}
	}
	collect(g)
	for _, member := range g.multi.dependencyGraphs() {
		collect(member)
	}
	out := make([]resolvedCandidate, 0, len(order))
	for _, id := range order {
		out = append(out, byFullID[id])
	}
	return out
}

// entryIDMatcher builds the entry predicate for a bare ID input: an exact
// full-ID match, or a short {type}-{layer}-{suffix} match. Returns nil when
// the input is neither shape, so the caller passes it through untouched.
func entryIDMatcher(input string) func(*Entry) bool {
	if _, err := ParseID(input); err == nil {
		return func(e *Entry) bool { return e.ID == input }
	}
	parts := strings.SplitN(input, "-", 3)
	if len(parts) != 3 || parts[2] == "" {
		return nil
	}
	typeCode, layerCode, suffix := parts[0], parts[1], parts[2]
	if _, ok := TypeFromAbbrev[typeCode]; !ok {
		return nil
	}
	if _, ok := LayerFromAbbrev[layerCode]; !ok {
		return nil
	}
	return func(e *Entry) bool {
		if TypeAbbrev[e.Type] != typeCode || LayerAbbrev[e.Layer] != layerCode {
			return false
		}
		p, err := ParseID(e.ID)
		return err == nil && p.Suffix == suffix
	}
}

// Lint returns all entries that have validation warnings.
func (g *Graph) Lint() []*Entry {
	var result []*Entry
	for _, e := range g.Entries {
		if len(e.Warnings) > 0 {
			result = append(result, e)
		}
	}
	return result
}

// HealthIssue is one graph-integrity problem as a displayable line: the entry
// ID (or load ref) it concerns and the human message.
type HealthIssue struct {
	Ref     string
	Message string
}

// GraphHealth is a flat summary of graph-integrity problems: the count of
// entry warnings, the count of unreadable (load-failed) entries, and every
// problem as an ordered line (load failures first, then entry warnings).
type GraphHealth struct {
	Warnings   int
	LoadErrors int
	Issues     []HealthIssue
}

// Clean reports whether the graph carries no integrity problems at all.
func (h GraphHealth) Clean() bool {
	return h.Warnings == 0 && h.LoadErrors == 0
}

// Health flattens the graph's load failures and per-entry warnings into one
// summary. It is a pure read of state already computed at construction
// (validate) and load time; callers format and cap it for display.
func (g *Graph) Health() GraphHealth {
	h := GraphHealth{LoadErrors: len(g.LoadIssues)}
	for _, issue := range g.LoadIssues {
		h.Issues = append(h.Issues, HealthIssue(issue))
	}
	for _, e := range g.Entries {
		for _, w := range e.Warnings {
			h.Warnings++
			h.Issues = append(h.Issues, HealthIssue{Ref: e.ID, Message: w.Message})
		}
	}
	return h
}

// validate checks all entries for integrity issues and populates their Warnings fields.
// Runs per-entry checks first, then graph-level actor/role checks that
// require the full graph context (chain membership, canonical history,
// cross-chain invariants).
func (g *Graph) validate() {
	for _, e := range g.Entries {
		ValidateEntry(e, g)
	}
	validateActorInvariant(g)
	validateRoleOrphans(g)
	validateParticipantCoverage(g)
	validateAliasAmbiguity(g)
	validateSupersedeForks(g)
	validateProcedureInvariant(g)
	validateProcedureForks(g)
}

// validateSupersedeForks flags a live fork — an entry whose supersession
// closure has more than one currently-active head, leaving head resolution
// genuinely ambiguous (ResolveRef would have to choose between competing live
// successors). Per d-cpt-rgx, only live forks warn: a fork whose branches have
// all closed or reconverged to a single head is settled and stays quiet.
// Supersession is meant to be linear, but a historical fork that resolved
// itself is benign and, under immutability, cannot be edited away — so warning
// on branch count alone would be permanent noise. Each immediate superseder is
// resolved to its branch head; distinct heads that are not closed are the live
// successors. The warning attaches to the forked entry, listing those heads in
// sorted order for deterministic output.
func validateSupersedeForks(g *Graph) {
	closed := g.closedSet()
	for id, supers := range g.SupersededBy {
		if len(supers) < 2 {
			continue
		}
		// Procedure forks are owned by validateProcedureForks: for a
		// procedure the fork is resolved (project head wins for execution),
		// so the generic "resolution is ambiguous" message would mislead.
		if e, ok := g.ByID[id]; ok && e.IsProcedure() {
			continue
		}
		liveHeads := make(map[string]bool)
		for _, s := range supers {
			head := g.ResolveRef(s).Head()
			if !closed[head] {
				liveHeads[head] = true
			}
		}
		if len(liveHeads) < 2 {
			continue // settled: branches closed or reconverged to one head
		}
		e, ok := g.ByID[id]
		if !ok {
			continue
		}
		heads := make([]string, 0, len(liveHeads))
		for h := range liveHeads {
			heads = append(heads, h)
		}
		sort.Strings(heads)
		e.Warnings = append(e.Warnings, Warning{
			Field:   "supersedes",
			Value:   id,
			Message: fmt.Sprintf("entry has %d active supersede heads (%s) — a live fork; head resolution is ambiguous until the branches converge or close", len(heads), strings.Join(heads, ", ")),
		})
	}
}

// validateActorInvariant enforces the write-once-across-chains invariant
// per plan d-cpt-d34 AC 12: a canonical may appear in at most one actor-
// identity chain's history. Defense-in-depth against race conditions or
// validator bypass — the pre-flight mechanical check is the first line.
func validateActorInvariant(g *Graph) {
	chains := g.ActorChains()
	// Map canonical → list of chain head IDs that carry it.
	ownerChains := make(map[string][]string)
	for _, c := range chains {
		headID := ""
		if c.Head != nil {
			headID = c.Head.ID
		}
		for _, canonical := range c.CanonicalHistory {
			ownerChains[canonical] = append(ownerChains[canonical], headID)
		}
	}
	for canonical, headIDs := range ownerChains {
		if len(headIDs) < 2 {
			continue
		}
		// Flag every actor entry in the offending chains.
		for _, c := range chains {
			if c.Head == nil {
				continue
			}
			if !slices.Contains(headIDs, c.Head.ID) {
				continue
			}
			if !c.HasCanonical(canonical) {
				continue
			}
			for _, e := range c.Entries {
				if e.Canonical != canonical {
					continue
				}
				e.Warnings = append(e.Warnings, Warning{
					Field:   "canonical",
					Value:   canonical,
					Message: fmt.Sprintf("canonical %q appears in %d actor-identity chains (write-once invariant violated)", canonical, len(headIDs)),
				})
			}
		}
	}
}

// validateRoleOrphans flags role decisions whose Actor canonical does not
// resolve to any actor-identity chain's canonical history per plan d-cpt-d34
// AC 13. Distinct from the normal-case cascade: the cascade derives-closes
// roles automatically when a chain retires; the orphan lint catches entries
// that shouldn't exist in a healthy graph (direct edits, validator bypass,
// corruption).
func validateRoleOrphans(g *Graph) {
	for _, e := range g.Entries {
		if !e.IsRole() {
			continue
		}
		actor := strings.TrimSpace(e.Actor)
		if actor == "" {
			continue // already flagged by validateRoleFrontmatter
		}
		if g.ChainForCanonical(actor) != nil {
			continue
		}
		e.Warnings = append(e.Warnings, Warning{
			Field:   "actor",
			Value:   actor,
			Message: fmt.Sprintf("role actor %q does not match any actor-identity chain's canonical history (orphan role)", actor),
		})
	}
}

// validateParticipantCoverage surfaces every distinct participant name in
// the graph that does not match any actor canonical per plan d-cpt-d34
// AC 10. The match set is every canonical in every actor-identity chain's
// history — active or retired. A retired chain still uniquely owns its
// canonicals (write-once across chains), so a participant whose name
// resolves to a closed chain is still known to the graph; only truly
// unregistered names surface. This differs from the pre-flight mechanical
// check (AC 6), which filters to active-head canonicals because capture
// time is about currency, not existence.
//
// Unlike pre-flight, lint has no grace mode: an all-historical graph
// with no actor signals is exactly the state AC 10 exists to surface —
// the warnings drive the bootstrap-playbook dialogue that captures the
// actors. The warning attaches to each entry listing the unresolved
// name so lint output clusters naturally by offending entry.
func validateParticipantCoverage(g *Graph) {
	known := make(map[string]struct{})
	for _, chain := range g.ActorChains() {
		for _, c := range chain.CanonicalHistory {
			known[c] = struct{}{}
		}
	}
	for _, e := range g.Entries {
		// Embedded base entries ship with the binary and must stay valid in
		// any project graph; actor canonicals are a project-local namespace,
		// so coverage doesn't apply to them.
		if e.Embedded {
			continue
		}
		for _, p := range e.Participants {
			if p == "" {
				continue
			}
			if _, ok := known[p]; ok {
				continue
			}
			e.Warnings = append(e.Warnings, Warning{
				Field:   "participants",
				Value:   p,
				Message: fmt.Sprintf("participant %q does not match any actor canonical in the graph", p),
			})
		}
	}
}

// validateAliasAmbiguity flags aliases that appear on more than one active
// actor signal per plan d-cpt-d34 AC 11. Informational: flags the actor
// entries whose aliases collide so the read side (mining, dialogue
// comprehension) knows to disambiguate from context.
func validateAliasAmbiguity(g *Graph) {
	active := g.ActiveActorHeads()
	// Map alias → list of actor entry IDs that declare it.
	owners := make(map[string][]string)
	for _, a := range active {
		for _, alias := range a.Aliases {
			alias = strings.TrimSpace(alias)
			if alias == "" {
				continue
			}
			owners[alias] = append(owners[alias], a.ID)
		}
	}
	for alias, ids := range owners {
		if len(ids) < 2 {
			continue
		}
		for _, a := range active {
			if !slices.Contains(ids, a.ID) {
				continue
			}
			a.Warnings = append(a.Warnings, Warning{
				Field:   "aliases",
				Value:   alias,
				Message: fmt.Sprintf("alias %q is also declared by %d other active actor(s) — read-side disambiguation required", alias, len(ids)-1),
			})
		}
	}
}

// ValidateEntry checks a single entry for integrity issues and populates its Warnings field.
// Used both at lint time (all entries) and at write time (new entry before commit).
// Per-kind structural rules run through the construction model — the read side
// projects the raw parsed form and keeps the findings that hold on historical
// entries, so it never owns a per-kind rule of its own.
func ValidateEntry(e *Entry, g *Graph) {
	validateEdges(e, g)
	c, findings := ConstructFromEntry(e)
	findings = append(findings, c.Validate(g)...)
	e.Warnings = append(e.Warnings, ReadWarnings(findings)...)
	validateAttachmentLinks(e)
}

// validateEdges runs the graph-edge rules — refs, lifecycle IDs, and the
// kind-conditional closes/supersedes semantics — populating e.Warnings.
func validateEdges(e *Entry, g *Graph) {
	validateRefs(e, g)
	validateIDRefs(e, g, "closes", e.Closes)
	validateIDRefs(e, g, "supersedes", e.Supersedes)
	validateCloses(e, g)
	validateSupersedes(e, g)
}

// ValidateForWrite runs the full write-path rule set on a construction: every
// construction rule including the capture-only ones, plus the graph-edge rules
// on the materialized entry. Every returned finding blocks the write — this is
// the one boundary all write surfaces (CLI, engine capture, base-entry
// assembly) validate through. The materialized entry is returned for the
// caller to carry forward; its Warnings stay empty.
func (c *EntryConstruction) ValidateForWrite(g *Graph) (*Entry, []Finding) {
	e := c.Entry()
	validateEdges(e, g)
	validateAttachmentLinks(e)
	findings := make([]Finding, 0, len(e.Warnings))
	for _, w := range e.Warnings {
		findings = append(findings, Finding{Field: w.Field, Value: w.Value, Message: w.Message})
	}
	e.Warnings = nil
	findings = append(findings, c.Validate(g)...)
	return e, findings
}

// validateRefs checks the refs field with kind awareness. Cross-repo refs
// are validated syntactically only — both portions must be well-formed, but
// the target is never dangling-checked against the local graph (resolution
// happens at capture against the cached remote graph). Dangling detection
// for local refs honors the forward-class exemption (d-cpt-uh0): a
// surfaces/required-by target may legitimately be absent at validation time.
func validateRefs(e *Entry, g *Graph) {
	for _, ref := range e.Refs {
		if IsCrossRepoID(ref.ID) {
			if err := ValidateCrossRepoID(ref.ID); err != nil {
				e.Warnings = append(e.Warnings, Warning{
					Field:   "refs",
					Value:   ref.ID,
					Message: fmt.Sprintf("malformed cross-repo ref in refs: %v", err),
				})
			}
			continue
		}
		if _, err := ParseID(ref.ID); err != nil {
			e.Warnings = append(e.Warnings, Warning{
				Field:   "refs",
				Value:   ref.ID,
				Message: fmt.Sprintf("malformed ID in refs: %s", ref.ID),
			})
			continue
		}
		if _, ok := g.ByID[ref.ID]; !ok && !IsForwardClassRefKind(ref.Kind) {
			e.Warnings = append(e.Warnings, Warning{
				Field:   "refs",
				Value:   ref.ID,
				Message: fmt.Sprintf("dangling ref in refs: %s (entry not found)", ref.ID),
			})
		}
	}
}

// validateIDRefs checks that all IDs in the given field are well-formed and
// exist in the graph. Used for the lifecycle fields (closes, supersedes),
// which never cross the repo boundary — a cross-repo ID here is an error in
// its own right, not a dangling ref.
func validateIDRefs(e *Entry, g *Graph, field string, ids []string) {
	for _, id := range ids {
		if IsCrossRepoID(id) {
			e.Warnings = append(e.Warnings, Warning{
				Field:   field,
				Value:   id,
				Message: fmt.Sprintf("cross-repo ID in %s: lifecycle edges never cross the repo boundary", field),
			})
			continue
		}
		_, err := ParseID(id)
		if err != nil {
			e.Warnings = append(e.Warnings, Warning{
				Field:   field,
				Value:   id,
				Message: fmt.Sprintf("malformed ID in %s: %s", field, id),
			})
			continue
		}
		if _, ok := g.ByID[id]; !ok {
			e.Warnings = append(e.Warnings, Warning{
				Field:   field,
				Value:   id,
				Message: fmt.Sprintf("dangling ref in %s: %s (entry not found)", field, id),
			})
		}
	}
}

// validateCloses checks the three closes refusals that hold whatever the
// entry says (20260820-151100-d-cpt-304): a question, actor, or annotation
// states no findings and closes nothing; a decision other than a directive
// closing a decision must use supersedes; a settled directive is retired
// only by supersession. Every other close is allowed. What makes a close
// valid is the stated rationale, and pre-flight judges that, flagging a
// weak one instead of blocking.
func validateCloses(e *Entry, g *Graph) {
	for _, id := range e.Closes {
		target, ok := g.ByID[id]
		if !ok {
			continue // already reported by validateIDRefs
		}

		// A settled directive is born terminal and carries no closing edge.
		// Reject any attempt to close one regardless of the closer's type —
		// supersession is the only way to retire it (see DerivedStatus).
		if target.IsSettled() {
			e.Warnings = append(e.Warnings, Warning{
				Field:   "closes",
				Value:   id,
				Message: fmt.Sprintf("%s (closing %s)", SettledCloseRule, id),
			})
			continue
		}

		switch {
		case e.Type == TypeSignal && (e.Kind == KindQuestion || e.Kind == KindActor || e.Kind == KindAnnotation):
			e.Warnings = append(e.Warnings, Warning{
				Field:   "closes",
				Value:   id,
				Message: fmt.Sprintf("%s (got %s signal closing %s %s)", SignalCloseRule, e.Kind, target.Type, id),
			})
		case e.Type == TypeDecision && target.Type == TypeDecision:
			// Retirement without replacement: a kind: directive decision may
			// close any decision, stating rationale in its body. closes
			// retires, supersedes replaces with lineage, and which relation
			// holds is the author's judgment about the work — forcing a
			// retirement into supersedes would fabricate a successor that
			// does not exist. Every other decision kind still uses
			// supersedes; a done signal remains the closer for work that
			// actually completed.
			if e.Kind == KindDirective {
				continue
			}
			e.Warnings = append(e.Warnings, Warning{
				Field:   "closes",
				Value:   id,
				Message: fmt.Sprintf("only a kind: directive decision may close another decision — use supersedes instead (%s closing decision %s)", e.Kind, id),
			})
		}
	}
}

// validateSupersedes checks that supersedes references point at the same entry type.
func validateSupersedes(e *Entry, g *Graph) {
	for _, id := range e.Supersedes {
		target, ok := g.ByID[id]
		if !ok {
			continue // already reported by validateIDRefs
		}

		if target.Override == OverrideClosed {
			e.Warnings = append(e.Warnings, Warning{
				Field:   "supersedes",
				Value:   id,
				Message: fmt.Sprintf("supersede refused: %s declares override: closed — its content renders from the running version's declarations, so a superseding copy would freeze stale truth; narrow through project rules instead", id),
			})
		}

		if target.Type != e.Type {
			e.Warnings = append(e.Warnings, Warning{
				Field:   "supersedes",
				Value:   id,
				Message: fmt.Sprintf("type mismatch in supersedes: %s supersedes %s %s (expected %s)", e.Type, target.Type, id, e.Type),
			})
		}
	}
}

// validateProcedureInvariant enforces write-once-across-chains for procedure
// canonicals, mirroring validateActorInvariant: a canonical may appear in at
// most one procedure chain's history. Procedure canonicals are their own
// namespace — actor canonicals are checked separately. Defense-in-depth; the
// pre-flight mechanical check is the first line.
func validateProcedureInvariant(g *Graph) {
	chains := g.ProcedureChains()
	ownerChains := make(map[string]int)
	for _, c := range chains {
		for _, canonical := range c.CanonicalHistory {
			ownerChains[canonical]++
		}
	}
	for canonical, count := range ownerChains {
		if count < 2 {
			continue
		}
		for _, c := range chains {
			if !c.HasCanonical(canonical) {
				continue
			}
			for _, e := range c.Entries {
				if e.Canonical != canonical {
					continue
				}
				e.Warnings = append(e.Warnings, Warning{
					Field:   "canonical",
					Value:   canonical,
					Message: fmt.Sprintf("canonical %q appears in %d procedure chains (write-once invariant violated)", canonical, count),
				})
			}
		}
	}
}

// validateProcedureForks flags procedure chains with more than one live head
// — a shipped base successor and a project override (or two overrides)
// competing. Unlike the generic supersede-fork warning, resolution is not
// ambiguous: the project head wins for execution (see ProcedureChain.Head).
// The fork is still flagged on every live head as a grooming candidate,
// because reconciling the branches is a deliberate, merge-style step —
// never automatic.
func validateProcedureForks(g *Graph) {
	for _, chain := range g.ProcedureChains() {
		if !chain.Forked() {
			continue
		}
		headIDs := make([]string, 0, len(chain.LiveHeads))
		for _, h := range chain.LiveHeads {
			headIDs = append(headIDs, h.ID)
		}
		sort.Strings(headIDs)
		winner := "<unresolved>"
		if chain.Head != nil {
			winner = chain.Head.ID
		}
		for _, h := range chain.LiveHeads {
			h.Warnings = append(h.Warnings, Warning{
				Field:   "canonical",
				Value:   h.Canonical,
				Message: fmt.Sprintf("procedure chain is forked: %d live heads (%s); %s wins for execution (project head over base) — groom to resolve the fork deliberately", len(headIDs), strings.Join(headIDs, ", "), winner),
			})
		}
	}
}

// validateAttachmentLinks checks that markdown links referencing the entry's attachment
// directory point to files that exist in the entry's Attachments list.
func validateAttachmentLinks(e *Entry) {
	if len(e.ID) < 8 {
		return
	}
	shortName := e.ID[6:] // DD-HHmmss-type-layer-suffix
	prefix := "./" + shortName + "/"

	if !strings.Contains(e.Content, prefix) {
		return
	}

	// Build set of known attachment filenames
	knownFiles := make(map[string]bool)
	for _, a := range e.Attachments {
		knownFiles[filepath.Base(a)] = true
	}

	// Find all references to the attachment directory in content
	rest := e.Content
	for {
		idx := strings.Index(rest, prefix)
		if idx < 0 {
			break
		}
		after := rest[idx+len(prefix):]
		// Extract filename until a markdown/whitespace delimiter
		end := strings.IndexAny(after, ") \n\t\"'")
		var filename string
		if end > 0 {
			filename = after[:end]
		} else if end < 0 {
			filename = after // rest of string
		}
		if filename != "" && !knownFiles[filename] {
			e.Warnings = append(e.Warnings, Warning{
				Field:   "attachments",
				Value:   prefix + filename,
				Message: fmt.Sprintf("broken attachment link: %s%s (file not found in attachment directory)", prefix, filename),
			})
		}
		if end < 0 {
			break
		}
		rest = after[end:]
	}
}
