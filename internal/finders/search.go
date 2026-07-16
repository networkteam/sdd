package finders

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/networkteam/sdd/internal/chunking"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/repos"
)

// SearchFinder is the read-side handler for sdd search. It composes
// three retrieval modes — text (live grep over .sdd/graph/), vector
// (chromem-go cosine search over chunks), and hybrid (RRF fusion when
// both are present) — and returns ranked entries with citation chunks.
//
// Pure read: no side effects. The lazy-fill that precedes a query is
// the IndexHandler's job; this finder consumes the index as-is.
type SearchFinder struct {
	graphDir   string
	embedder   llm.Embedder    // nil disables vector and hybrid modes
	indexStore *index.Index    // nil disables vector and hybrid modes
	repos      *repos.Registry // nil disables cross-repo search
}

// SearchFinderOptions configures NewSearchFinder. GraphDir is required
// (used by text mode to resolve attachment paths). Embedder + IndexStore
// are required for vector and hybrid modes; their absence makes those
// modes return an error. Repos is required for cross-repo search
// (MultiSearch).
type SearchFinderOptions struct {
	GraphDir   string
	Embedder   llm.Embedder
	IndexStore *index.Index
	Repos      *repos.Registry
}

// NewSearchFinder constructs a SearchFinder.
func NewSearchFinder(opts SearchFinderOptions) *SearchFinder {
	return &SearchFinder{
		graphDir:   opts.GraphDir,
		embedder:   opts.Embedder,
		indexStore: opts.IndexStore,
		repos:      opts.Repos,
	}
}

// VectorAvailable reports whether the configured dependencies allow
// vector or hybrid mode. Used by the CLI to render the
// `Search: text` vs `Search: vector,text` capability line in
// `sdd info`'s header.
func (f *SearchFinder) VectorAvailable() bool {
	return f.embedder != nil && f.indexStore != nil
}

// Search dispatches the query to the appropriate mode.
func (f *SearchFinder) Search(ctx context.Context, q query.SearchQuery) (*query.SearchResult, error) {
	if q.Graph == nil {
		return nil, errors.New("SearchQuery.Graph is required")
	}
	switch q.Mode() {
	case query.SearchModeText:
		return f.textSearch(q)
	case query.SearchModeVector:
		if !f.VectorAvailable() {
			return nil, errors.New("vector search requires an embedding provider — set embedding.provider in .sdd/config.local.yaml")
		}
		return f.vectorSearch(ctx, q)
	case query.SearchModeHybrid:
		if !f.VectorAvailable() {
			return nil, errors.New("hybrid search requires an embedding provider — set embedding.provider in .sdd/config.local.yaml")
		}
		return f.hybridSearch(ctx, q)
	default:
		return nil, fmt.Errorf("unknown search mode: %q", q.Mode())
	}
}

// textSearch runs a multi-term AND grep over each entry's searchable
// text (summary + body + attachments). Entries are kept only if every
// term's regex matches at least once. The score is the total match count
// across all terms; ties broken by earliest-match position.
//
// Per the AC: OR semantics are achievable by passing a single regex
// alternation as one --term value (e.g. --term "(X|Y)"). This finder
// does not implement a separate OR flag.
func (f *SearchFinder) textSearch(q query.SearchQuery) (*query.SearchResult, error) {
	regexes, err := compileTerms(q.Terms)
	if err != nil {
		return nil, err
	}
	if len(regexes) == 0 {
		return nil, errors.New("text mode requires at least one --term")
	}

	candidates := f.candidates(q)
	type scored struct {
		entry         *model.Entry
		matchCount    int
		firstMatchPos int
		bestSnippet   string
	}
	var hits []scored

	for _, e := range candidates {
		text, body := searchableText(f.graphDir, e)
		matches := 0
		earliest := -1
		allHit := true
		for _, re := range regexes {
			locs := re.FindAllStringIndex(text, -1)
			if len(locs) == 0 {
				allHit = false
				break
			}
			matches += len(locs)
			if earliest < 0 || locs[0][0] < earliest {
				earliest = locs[0][0]
			}
		}
		if !allHit {
			continue
		}
		snippet := snippetAround(text, earliest, snippetWindow)
		_ = body // body is currently unused for text-mode citation; reserved for future breadcrumb extraction
		hits = append(hits, scored{
			entry:         e,
			matchCount:    matches,
			firstMatchPos: earliest,
			bestSnippet:   snippet,
		})
	}

	// Status-aware sort: status multiplier is the dominant secondary
	// key after match count, demoting closed/cascade-closed entries
	// behind comparable open/active hits. Match count is integer so a
	// halved-by-multiplier match count of 8 still beats a non-demoted
	// match count of 4 — the multiplier bends the order, doesn't
	// invert it.
	type scoredWithStatus struct {
		s        scored
		adjusted float32
	}
	enriched := make([]scoredWithStatus, len(hits))
	for i, h := range hits {
		mult := statusMultiplier(q.Graph.DerivedStatus(h.entry).Kind)
		enriched[i] = scoredWithStatus{s: h, adjusted: float32(h.matchCount) * mult}
	}
	sort.Slice(enriched, func(i, j int) bool {
		if enriched[i].adjusted != enriched[j].adjusted {
			return enriched[i].adjusted > enriched[j].adjusted
		}
		return enriched[i].s.firstMatchPos < enriched[j].s.firstMatchPos
	})

	limit := q.EffectiveLimit()
	if len(enriched) > limit {
		enriched = enriched[:limit]
	}

	// Text mode contributes a single best-snippet citation per entry, unless
	// the caller suppressed citations (--max-citations 0), in which case the
	// entry surfaces as a header line only. The cap path in selectCitations
	// (vector/hybrid) already drops to nil for a zero cap; text mode builds
	// its citation directly, so it honours the cap here.
	showCitations := q.EffectiveMaxCitations() > 0
	out := &query.SearchResult{Mode: query.SearchModeText}
	for _, e := range enriched {
		var citations []query.Citation
		if showCitations {
			citations = []query.Citation{{
				Snippet: e.s.bestSnippet,
				Score:   e.adjusted,
			}}
		}
		out.Entries = append(out.Entries, query.SearchEntry{
			Entry:     e.s.entry,
			Score:     e.adjusted,
			Citations: citations,
		})
	}
	return out, nil
}

// vectorSearch embeds the query phrase, queries the index for an
// oversampled chunk list, then rolls up to per-entry by max chunk score
// (with depth-aware adjustments). Filters apply post-hoc against the
// graph.
func (f *SearchFinder) vectorSearch(ctx context.Context, q query.SearchQuery) (*query.SearchResult, error) {
	hits, err := f.runVector(ctx, q)
	if err != nil {
		return nil, err
	}
	out := &query.SearchResult{Mode: query.SearchModeVector, Entries: hits}
	return out, nil
}

// hybridSearch fuses text and vector ranked lists via RRF (k=60). The
// per-entry citation comes from whichever side ranked the entry higher.
// Multi-term AND on the text arm is preserved before fusion.
func (f *SearchFinder) hybridSearch(ctx context.Context, q query.SearchQuery) (*query.SearchResult, error) {
	textRes, err := f.textSearch(q)
	if err != nil {
		// Text grep with bad regex is a user error and should fall through;
		// other failures are unexpected.
		return nil, err
	}
	// Strip Phrase to skip vector mode reuse; we're calling the same finder.
	vecQ := q
	vecQ.Terms = nil
	vecRes, err := f.vectorSearch(ctx, vecQ)
	if err != nil {
		return nil, err
	}

	out := rrfFuse(textRes, vecRes, q.EffectiveLimit())
	out.Mode = query.SearchModeHybrid
	return out, nil
}

// runVector is the shared core for vector and hybrid: produces ranked
// SearchEntry values from the index. Oversamples by a factor of 4×limit
// at the chunk level so post-hoc filtering still leaves enough seeds.
func (f *SearchFinder) runVector(ctx context.Context, q query.SearchQuery) ([]query.SearchEntry, error) {
	if q.Phrase == "" {
		return nil, errors.New("vector search requires a non-empty --query phrase")
	}
	embeddings, err := f.embedder.EmbedQueries(ctx, []string{q.Phrase})
	if err != nil {
		return nil, fmt.Errorf("embedding query phrase: %w", err)
	}
	if len(embeddings) != 1 {
		return nil, fmt.Errorf("embedder returned %d vectors for 1 input", len(embeddings))
	}

	limit := q.EffectiveLimit()
	// Oversample chunks so the per-entry roll-up still has enough
	// distinct entries to fill top-N. A large entry can contribute many
	// chunks to the chunk-level top-K — observed during the first
	// real-corpus eval, where d-tac-lqr alone owns 24 chunks (its body +
	// design.md attachment) and dominated the top-20 hits, collapsing
	// the entry-rollup to just 1-2 unique entries. 10× the limit (with
	// a floor of 50) keeps the pool diverse without making the chromem
	// scan meaningfully more expensive — flat scan is O(N), not
	// O(K·log N).
	chunkLimit := limit * 10
	if chunkLimit < 50 {
		chunkLimit = 50
	}
	indexHits, err := f.indexStore.Query(ctx, embeddings[0], chunkLimit)
	if err != nil {
		return nil, fmt.Errorf("vector query: %w", err)
	}

	candidates := f.candidates(q)
	candidateSet := make(map[string]*model.Entry, len(candidates))
	for _, e := range candidates {
		candidateSet[e.ID] = e
	}

	// Read-time stale-version filtering: the shared store accumulates a
	// version per entry state, so a hit is valid only when its version equals
	// the entry's current state hash. A new row carries entry_hash metadata; a
	// legacy v1 row's version is recovered from the manifest. The current hash
	// is computed once per hit entry (attachments read from disk, as the
	// indexer does) and cached.
	freshness := f.newVersionFreshness()

	// Collect all chunk hits per entry so the rollup can keep the
	// strongest few rather than just the single best. An entry that
	// matches on multiple dimensions (a long plan whose summary,
	// approach section, and attachment all touch the query) deserves
	// to surface that breadth in its citations.
	perEntry := map[string][]chunkScore{}
	for _, h := range indexHits {
		entry, ok := candidateSet[h.EntryID]
		if !ok {
			continue
		}
		fresh, err := freshness.fresh(ctx, entry, h)
		if err != nil {
			return nil, err
		}
		if !fresh {
			continue
		}
		adjusted := adjustVectorScore(h.Score, h.IsSummary, h.Depth)
		perEntry[h.EntryID] = append(perEntry[h.EntryID], chunkScore{hit: h, score: adjusted})
	}

	out := make([]query.SearchEntry, 0, len(perEntry))
	for entryID, hits := range perEntry {
		sort.Slice(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
		topChunkScore := hits[0].score
		statusMult := statusMultiplier(q.Graph.DerivedStatus(candidateSet[entryID]).Kind)
		entryScore := topChunkScore * statusMult

		citations := selectCitations(hits, topChunkScore, statusMult, q.EffectiveMaxCitations())
		out = append(out, query.SearchEntry{
			Entry:     candidateSet[entryID],
			Score:     entryScore,
			Citations: citations,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// candidates applies the graph filter and the IncludeSuperseded gate.
// Superseded entries are excluded by default — they pollute clusters with
// historical near-duplicates per the design.md rationale.
func (f *SearchFinder) candidates(q query.SearchQuery) []*model.Entry {
	filtered := q.Graph.Filter(q.Filter)
	if q.IncludeSuperseded {
		return filtered
	}
	out := make([]*model.Entry, 0, len(filtered))
	for _, e := range filtered {
		if q.Graph.DerivedStatus(e).Kind == model.StatusSupersededBy {
			continue
		}
		out = append(out, e)
	}
	return out
}

// versionFreshness answers, per hit, whether its stored version equals the
// entry's current state hash — the read-time gate that keeps the shared,
// version-accumulating store from surfacing stale citations. The current hash
// is computed once per hit entry (attachments read from disk, as the indexer
// does) and cached; the manifest is loaded once, and only when a legacy row
// (no entry_hash metadata) needs it.
type versionFreshness struct {
	indexDir    string // "" disables manifest fallback (an in-memory index, tests)
	attachments chunking.AttachmentReader
	current     map[string]string
	manifest    *index.Manifest
}

func (f *SearchFinder) newVersionFreshness() *versionFreshness {
	dir := ""
	if f.indexStore != nil {
		dir = f.indexStore.Path()
	}
	return &versionFreshness{
		indexDir:    dir,
		attachments: chunking.DiskAttachmentReader{GraphDir: f.graphDir},
		current:     map[string]string{},
	}
}

func (v *versionFreshness) fresh(ctx context.Context, entry *model.Entry, hit index.Hit) (bool, error) {
	version := hit.EntryHash
	if version == "" {
		if v.indexDir == "" {
			// No manifest to resolve a legacy row's version — keep the hit.
			return true, nil
		}
		if v.manifest == nil {
			m, err := index.LoadManifest(v.indexDir)
			if err != nil {
				return false, err
			}
			v.manifest = m
		}
		version = v.manifest.VersionHashForChunk(hit.EntryID, hit.ChunkID)
		if version == "" {
			// Unresolvable version — keep the hit rather than drop it blindly.
			return true, nil
		}
	}
	cur, ok := v.current[entry.ID]
	if !ok {
		h, err := chunking.EntryStateHash(ctx, entry, v.attachments)
		if err != nil {
			return false, err
		}
		cur = h
		v.current[entry.ID] = h
	}
	return version == cur, nil
}

// compileTerms compiles each --term into a case-insensitive regex.
// Empty terms are rejected so a stray --term "" doesn't degrade to
// match-all.
func compileTerms(terms []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(terms))
	for _, t := range terms {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		// Wrap with case-insensitivity so token lookups are forgiving.
		// Users opt-out by including (?-i) in the regex itself.
		re, err := regexp.Compile("(?i)" + t)
		if err != nil {
			return nil, fmt.Errorf("compiling --term %q: %w", t, err)
		}
		out = append(out, re)
	}
	return out, nil
}

// searchableText concatenates the entry's summary, body, and (markdown)
// attachments into one string for grep matching. Returns the combined
// text and the body alone (the body is reserved for breadcrumb-aware
// citation in a future iteration; today's text-mode citation uses only
// the snippet).
func searchableText(graphDir string, e *model.Entry) (combined, body string) {
	var b strings.Builder
	if e.Summary != "" {
		b.WriteString(e.Summary)
		b.WriteByte('\n')
	}
	b.WriteString(e.Content)
	body = e.Content

	for _, attRel := range e.Attachments {
		if !strings.HasSuffix(strings.ToLower(attRel), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(graphDir, attRel))
		if err != nil {
			continue
		}
		b.WriteByte('\n')
		b.Write(data)
	}
	return b.String(), body
}

// snippetWindow is the half-width (in runes) of the citation snippet
// extracted around a known match position; vector hits use the leading
// 2*snippetWindow as no specific position applies. ~300 runes total
// gives the agent enough context to judge relevance without forcing a
// follow-up `sdd show`. Originally 75 (~150 total) per d-tac-lqr's AC,
// but real-corpus evaluation showed mid-word truncations and too little
// context — bumped to 150 with word-boundary trimming below.
const snippetWindow = 150

// snippetAround returns up to ~2*window bytes of context around pos,
// trimmed to whitespace boundaries so the citation never starts or
// ends mid-word. pos<0 (or pos==0 for vector hits where no specific
// match position applies) returns the leading window of the chunk.
//
// When the snippet is shorter than the source text on either side, a
// `[...]` marker is added so the agent reading the citation can tell
// whether it's seeing a complete chunk or a window into a longer one.
// The markers sit outside any word-boundary trim so an agent that
// strips them gets clean prose.
func snippetAround(text string, pos int, window int) string {
	if text == "" {
		return ""
	}
	if pos < 0 {
		pos = 0
	}
	start := pos - window
	if start < 0 {
		start = 0
	}
	end := pos + window
	if end > len(text) {
		end = len(text)
	}
	start = trimToWordBoundaryStart(text, start)
	end = trimToWordBoundaryEnd(text, end)

	out := collapseWS(text[start:end])
	if start > 0 {
		out = "[...] " + out
	}
	if end < len(text) {
		out = out + " [...]"
	}
	return out
}

// trimToWordBoundaryStart advances start forward past any partial word
// it lands inside, so the snippet begins on a word edge. start==0 stays
// at 0 (we're already at the beginning of the text). Bounded by a small
// look-ahead to avoid wandering into the next sentence on long
// run-on prose.
func trimToWordBoundaryStart(text string, start int) int {
	if start == 0 {
		return 0
	}
	const maxAdvance = 40
	limit := start + maxAdvance
	if limit > len(text) {
		limit = len(text)
	}
	// If the byte at start is in the middle of a word, advance to the
	// next whitespace.
	for i := start; i < limit; i++ {
		c := text[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			return i + 1
		}
	}
	return start
}

// trimToWordBoundaryEnd retreats end backward to the previous whitespace
// so the snippet ends cleanly. end==len(text) stays at len(text). Same
// look-back bound as the start counterpart.
func trimToWordBoundaryEnd(text string, end int) int {
	if end >= len(text) {
		return len(text)
	}
	const maxRetreat = 40
	limit := end - maxRetreat
	if limit < 0 {
		limit = 0
	}
	for i := end; i > limit; i-- {
		c := text[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			return i
		}
	}
	return end
}

// collapseWS replaces runs of whitespace (including newlines) with a
// single space so the snippet renders on one line.
func collapseWS(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(b.String())
}

// selectCitations picks the chunks whose score is strong enough to be
// worth surfacing alongside the entry's primary citation. The 85%
// threshold (relative to the top chunk's score) keeps near-duplicates
// and weak runs out of the citation slice while letting genuinely-
// strong secondary chunks through. The cap (max) flows from the
// SearchQuery via EffectiveMaxCitations() so callers can dial verbosity
// per-call.
//
// Each citation carries its own status-adjusted score so the presenter
// can render per-citation relative percentages — different chunks from
// the same entry typically score differently, and hiding that under one
// entry-level number loses real information about WHERE inside the
// entry the strongest match landed.
//
// hits must be pre-sorted by score descending — the caller does that.
func selectCitations(hits []chunkScore, topScore, statusMult float32, max int) []query.Citation {
	if len(hits) == 0 || max <= 0 {
		return nil
	}
	threshold := topScore * citationScoreThreshold
	out := make([]query.Citation, 0, max)
	for _, h := range hits {
		if len(out) >= max {
			break
		}
		if h.score < threshold && len(out) > 0 {
			break
		}
		out = append(out, query.Citation{
			Breadcrumb:           h.hit.Breadcrumb,
			Snippet:              snippetAround(h.hit.Body, 0, snippetWindow),
			SourceAttachmentPath: h.hit.SourceAttachmentPath,
			IsSummary:            h.hit.IsSummary,
			IsAttachment:         h.hit.IsAttachment,
			Score:                h.score * statusMult,
		})
	}
	return out
}

// citationScoreThreshold is the per-entry cutoff relative to the entry's
// top chunk score: chunks scoring below this fraction are skipped (the
// primary citation is always kept regardless). This is a heuristic, not
// a user knob — exposing it as configuration would invite over-tuning
// without empirical signal that one value beats another. The value is
// tuned against the first real-corpus eval and revisited when the eval
// set grows.
const citationScoreThreshold = 0.85

// chunkScore is a tiny adapter type used by selectCitations to keep the
// adjusted-score and the index Hit together while sorting and
// thresholding. Lifted to package scope so the helper signature stays
// clean.
type chunkScore struct {
	hit   index.Hit
	score float32
}

// statusMultiplier returns the score adjustment for an entry's derived
// status, applied in vector and text modes after the per-chunk score
// is settled. Active and open entries (and done signals — terminal
// facts of execution) stay at full weight; closed and cascade-closed
// entries get a small penalty so they fall behind comparable open hits
// without being filtered out (closed historical context can still be
// the right answer to a "what did we used to think" query). Cascade-
// orphan roles get a heavier penalty — they signal abnormal state, not
// just lifecycle progress.
//
// Tuned empirically from the first real-corpus eval, where a closed
// gap on output formatting ranked above the open gap on output
// ordering for a query phrased about ordering. Multiplier values are
// intentionally modest so an open hit with weak similarity doesn't
// leapfrog a strongly-similar closed hit.
func statusMultiplier(s model.StatusKind) float32 {
	switch s {
	case model.StatusClosedBy, model.StatusCascadeClosedBy:
		return 0.85
	case model.StatusSupersededBy:
		// Reaches this code path only when --include-superseded is set;
		// otherwise candidates() already filtered.
		return 0.7
	case model.StatusCascadeOrphan:
		return 0.6
	default:
		return 1.0
	}
}

// adjustVectorScore applies depth-aware scoring per d-tac-lqr's AC:
// summary chunks get a small boost; deeper section chunks get a small
// penalty. Coefficients are conservative — the unadjusted cosine score
// dominates, with these adjustments breaking ties between similar hits.
func adjustVectorScore(raw float32, isSummary bool, depth int) float32 {
	score := raw
	if isSummary {
		score *= 1.1
	}
	if depth > 1 {
		penalty := 1.0 - 0.02*float32(depth-1)
		if penalty < 0.6 {
			penalty = 0.6
		}
		score *= penalty
	}
	return score
}

// rrfFuse fuses two ranked SearchResult lists via Reciprocal Rank Fusion
// (k=60). For each entry appearing in either list, score is summed across
// the two reciprocal-rank contributions. The citation is taken from
// whichever side ranked the entry higher (smaller rank = higher).
//
// Per Cormack et al., RRF outperforms weighted-sum fusion without
// per-corpus tuning, so we don't expose the k parameter.
func rrfFuse(textRes, vecRes *query.SearchResult, limit int) *query.SearchResult {
	const k = 60
	type fused struct {
		entry     *model.Entry
		score     float32
		citations []query.Citation
		// rank in each ranker (zero means absent).
		textRank int
		vecRank  int
	}
	merged := map[string]*fused{}

	if textRes != nil {
		for i, se := range textRes.Entries {
			rank := i + 1
			contribution := 1.0 / float32(k+rank)
			merged[se.Entry.ID] = &fused{
				entry:     se.Entry,
				score:     contribution,
				citations: se.Citations,
				textRank:  rank,
			}
		}
	}
	if vecRes != nil {
		for i, se := range vecRes.Entries {
			rank := i + 1
			contribution := 1.0 / float32(k+rank)
			if cur, ok := merged[se.Entry.ID]; ok {
				cur.score += contribution
				cur.vecRank = rank
				// Take the vector citations only if vector ranked the
				// entry higher (smaller rank). Mixing citations from
				// text and vector would mean per-citation scores live
				// on different scales — match-count vs cosine — which
				// the relative-percentage rendering can't reconcile.
				// One side wins, all its citations come along.
				if cur.vecRank > 0 && (cur.textRank == 0 || cur.vecRank < cur.textRank) {
					cur.citations = se.Citations
				}
			} else {
				merged[se.Entry.ID] = &fused{
					entry:     se.Entry,
					score:     contribution,
					citations: se.Citations,
					vecRank:   rank,
				}
			}
		}
	}

	out := make([]query.SearchEntry, 0, len(merged))
	for _, m := range merged {
		out = append(out, query.SearchEntry{
			Entry:     m.entry,
			Score:     m.score,
			Citations: m.citations,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return &query.SearchResult{Entries: out}
}
