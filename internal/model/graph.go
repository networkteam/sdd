package model

import (
	"fmt"
	"path/filepath"
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
	graphDir     string
}

// NewGraph builds a graph from the given entries without touching the filesystem.
func NewGraph(entries []*Entry) *Graph {
	g := &Graph{
		Entries:      entries,
		ByID:         make(map[string]*Entry, len(entries)),
		RefsTo:       make(map[string][]string),
		ClosedBy:     make(map[string][]string),
		SupersededBy: make(map[string][]string),
	}

	for _, e := range entries {
		g.ByID[e.ID] = e
	}

	// Build reverse indexes
	for _, e := range entries {
		for _, ref := range e.Refs {
			g.RefsTo[ref] = append(g.RefsTo[ref], e.ID)
		}
		for _, c := range e.Closes {
			g.ClosedBy[c] = append(g.ClosedBy[c], e.ID)
		}
		for _, s := range e.Supersedes {
			g.SupersededBy[s] = append(g.SupersededBy[s], e.ID)
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

// Directives returns active directive decisions (not closed, not superseded).
// Allow-list shape keeps future decision kinds from silently flooding this
// set — each kind gets surfaced deliberately with its own accessor.
func (g *Graph) Directives() []*Entry {
	closed := g.closedSet()
	superseded := g.supersededSet()

	var directives []*Entry
	for _, e := range g.Entries {
		if e.Type != TypeDecision || e.Kind != KindDirective {
			continue
		}
		if !closed[e.ID] && !superseded[e.ID] {
			directives = append(directives, e)
		}
	}
	return directives
}

// Activities returns active activity decisions (not closed, not superseded).
// Activities are THAT-shaped commitments — capturing that specific work
// happens, independent of the directive-style choice of *what* to do.
func (g *Graph) Activities() []*Entry {
	closed := g.closedSet()
	superseded := g.supersededSet()

	var activities []*Entry
	for _, e := range g.Entries {
		if e.Type != TypeDecision || e.Kind != KindActivity {
			continue
		}
		if !closed[e.ID] && !superseded[e.ID] {
			activities = append(activities, e)
		}
	}
	return activities
}

// Plans returns active plan decisions (not closed, not superseded).
func (g *Graph) Plans() []*Entry {
	closed := g.closedSet()
	superseded := g.supersededSet()

	var plans []*Entry
	for _, e := range g.Entries {
		if e.Type != TypeDecision || !e.IsPlan() {
			continue
		}
		if !closed[e.ID] && !superseded[e.ID] {
			plans = append(plans, e)
		}
	}
	return plans
}

// Contracts returns active contract decisions (not superseded, not closed).
// Contracts retire via a same-kind supersede or a directive-kind decision
// closing them with rationale (universal retirement rule).
func (g *Graph) Contracts() []*Entry {
	closed := g.closedSet()
	superseded := g.supersededSet()

	var contracts []*Entry
	for _, e := range g.Entries {
		if e.Type != TypeDecision || !e.IsContract() {
			continue
		}
		if !closed[e.ID] && !superseded[e.ID] {
			contracts = append(contracts, e)
		}
	}
	return contracts
}

// Aspirations returns active aspiration decisions (not superseded, not closed).
// Like contracts, aspirations are durable — they retire via supersede or
// close-by-directive with rationale (see dissolution/retirement calibration).
func (g *Graph) Aspirations() []*Entry {
	closed := g.closedSet()
	superseded := g.supersededSet()

	var aspirations []*Entry
	for _, e := range g.Entries {
		if e.Type != TypeDecision || !e.IsAspiration() {
			continue
		}
		if !closed[e.ID] && !superseded[e.ID] {
			aspirations = append(aspirations, e)
		}
	}
	return aspirations
}

// OpenSignals returns signals that are closure-gated attention items — gaps
// awaiting a decision/done and questions awaiting dissolution. Facts,
// insights, and done signals are deliberately excluded: facts and insights
// are stable observational records (retired via directive close, not
// resolved), and done signals are terminal facts of execution. The
// allow-list shape means new signal kinds default to "not an attention
// item" rather than silently flooding the open set.
func (g *Graph) OpenSignals() []*Entry {
	closed := g.closedSet()
	superseded := g.supersededSet()

	var open []*Entry
	for _, e := range g.Entries {
		if e.Type != TypeSignal {
			continue
		}
		if e.Kind != KindGap && e.Kind != KindQuestion {
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
			walk(ref)
		}
		chain = append(chain, e)
	}

	walk(id)
	return chain
}

// GraphFilter specifies criteria for filtering graph entries.
type GraphFilter struct {
	Type        EntryType
	Layer       Layer
	Kind        Kind
	MissingKind bool // when true, only include entries whose stored kind field is empty
	OpenOnly    bool // when true, exclude closed/superseded signals and decisions
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
		if f.MissingKind && e.Kind != "" {
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
			if d := computeDepth(ref) + 1; d > maxDep {
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

// validate checks all entries for integrity issues and populates their Warnings fields.
func (g *Graph) validate() {
	for _, e := range g.Entries {
		ValidateEntry(e, g)
	}
}

// ValidateEntry checks a single entry for integrity issues and populates its Warnings field.
// Used both at lint time (all entries) and at write time (new entry before commit).
func ValidateEntry(e *Entry, g *Graph) {
	validateIDRefs(e, g, "refs", e.Refs)
	validateIDRefs(e, g, "closes", e.Closes)
	validateIDRefs(e, g, "supersedes", e.Supersedes)
	validateCloses(e, g)
	validateSupersedes(e, g)
	validateKind(e)
	validateDoneSignalRefs(e)
	validateAttachmentLinks(e)
}

// validateIDRefs checks that all IDs in the given field are well-formed and exist in the graph.
func validateIDRefs(e *Entry, g *Graph, field string, ids []string) {
	for _, id := range ids {
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

// validateCloses checks type constraints on closes references.
// Valid: decision closes signal; done-kind signal closes decision or signal;
// kind: directive decision closes a stable-kind decision (contract or
// aspiration) as retirement.
// Invalid: non-done signal closes anything; any decision-closes-decision
// other than directive→{contract|aspiration}.
func validateCloses(e *Entry, g *Graph) {
	for _, id := range e.Closes {
		target, ok := g.ByID[id]
		if !ok {
			continue // already reported by validateIDRefs
		}

		switch {
		case e.Type == TypeSignal && e.Kind != KindDone:
			e.Warnings = append(e.Warnings, Warning{
				Field:   "closes",
				Value:   id,
				Message: fmt.Sprintf("only done-kind signals may close entries (got %s signal closing %s %s)", e.Kind, target.Type, id),
			})
		case e.Type == TypeDecision && target.Type == TypeDecision:
			// Retirement exception: a kind: directive decision may close a
			// kind: contract or kind: aspiration decision with rationale.
			// Every other decision-closes-decision pattern uses supersedes.
			if e.Kind == KindDirective && (target.Kind == KindContract || target.Kind == KindAspiration) {
				continue
			}
			e.Warnings = append(e.Warnings, Warning{
				Field:   "closes",
				Value:   id,
				Message: fmt.Sprintf("decision cannot close another decision — use supersedes instead (closes decision %s)", id),
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

		if target.Type != e.Type {
			e.Warnings = append(e.Warnings, Warning{
				Field:   "supersedes",
				Value:   id,
				Message: fmt.Sprintf("type mismatch in supersedes: %s supersedes %s %s (expected %s)", e.Type, target.Type, id, e.Type),
			})
		}
	}
}

// validateKind checks that signals and decisions have a kind consistent with
// their type.
func validateKind(e *Entry) {
	switch e.Type {
	case TypeSignal:
		if e.Kind == "" {
			e.Warnings = append(e.Warnings, Warning{
				Field:   "kind",
				Message: "signal missing kind field (expected gap, fact, question, insight, or done)",
			})
			return
		}
		if !IsValidKindForType(TypeSignal, e.Kind) {
			e.Warnings = append(e.Warnings, Warning{
				Field:   "kind",
				Value:   string(e.Kind),
				Message: fmt.Sprintf("invalid signal kind %q (expected gap, fact, question, insight, or done)", e.Kind),
			})
		}
	case TypeDecision:
		if e.Kind == "" {
			e.Warnings = append(e.Warnings, Warning{
				Field:   "kind",
				Message: "decision missing kind field (expected directive, activity, plan, contract, or aspiration)",
			})
			return
		}
		if !IsValidKindForType(TypeDecision, e.Kind) {
			e.Warnings = append(e.Warnings, Warning{
				Field:   "kind",
				Value:   string(e.Kind),
				Message: fmt.Sprintf("invalid decision kind %q (expected directive, activity, plan, contract, or aspiration)", e.Kind),
			})
		}
	}
}

// validateDoneSignalRefs checks that a done-kind signal carries at least one
// closes or refs entry. Required structurally because a done signal is a
// fact-of-completion pointing at the commitment it fulfills — a target is the
// minimum anchor for the claim.
func validateDoneSignalRefs(e *Entry) {
	if e.Type != TypeSignal || e.Kind != KindDone {
		return
	}
	if len(e.Closes) == 0 && len(e.Refs) == 0 {
		e.Warnings = append(e.Warnings, Warning{
			Field:   "closes",
			Message: "done signal must carry at least one closes or refs (target of the completion claim)",
		})
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
