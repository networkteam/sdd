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

	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
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
	embedder   llm.Embedder // nil disables vector and hybrid modes
	indexStore *index.Index // nil disables vector and hybrid modes
}

// SearchFinderOptions configures NewSearchFinder. GraphDir is required
// (used by text mode to resolve attachment paths). Embedder + IndexStore
// are required for vector and hybrid modes; their absence makes those
// modes return an error.
type SearchFinderOptions struct {
	GraphDir   string
	Embedder   llm.Embedder
	IndexStore *index.Index
}

// NewSearchFinder constructs a SearchFinder.
func NewSearchFinder(opts SearchFinderOptions) *SearchFinder {
	return &SearchFinder{
		graphDir:   opts.GraphDir,
		embedder:   opts.Embedder,
		indexStore: opts.IndexStore,
	}
}

// VectorAvailable reports whether the configured dependencies allow
// vector or hybrid mode. Used by the CLI to render the
// `Search: text` vs `Search: vector,text` capability line in
// `sdd status`.
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

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].matchCount != hits[j].matchCount {
			return hits[i].matchCount > hits[j].matchCount
		}
		return hits[i].firstMatchPos < hits[j].firstMatchPos
	})

	limit := q.EffectiveLimit()
	if len(hits) > limit {
		hits = hits[:limit]
	}

	out := &query.SearchResult{Mode: query.SearchModeText}
	for _, h := range hits {
		out.Entries = append(out.Entries, query.SearchEntry{
			Entry: h.entry,
			Score: float32(h.matchCount),
			Citation: query.Citation{
				Snippet: h.bestSnippet,
			},
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
	chunkLimit := limit * 4
	if chunkLimit < 20 {
		chunkLimit = 20
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

	type entryAccumulator struct {
		entry      *model.Entry
		bestScore  float32
		bestHit    index.Hit
	}
	acc := map[string]*entryAccumulator{}

	for _, h := range indexHits {
		e, ok := candidateSet[h.EntryID]
		if !ok {
			continue
		}
		adjusted := adjustVectorScore(h.Score, h.IsSummary, h.Depth)
		if cur, ok := acc[h.EntryID]; ok {
			if adjusted > cur.bestScore {
				cur.bestScore = adjusted
				cur.bestHit = h
			}
		} else {
			acc[h.EntryID] = &entryAccumulator{entry: e, bestScore: adjusted, bestHit: h}
		}
	}

	out := make([]query.SearchEntry, 0, len(acc))
	for _, a := range acc {
		out = append(out, query.SearchEntry{
			Entry: a.entry,
			Score: a.bestScore,
			Citation: query.Citation{
				Breadcrumb:           a.bestHit.Breadcrumb,
				Snippet:              snippetAround(a.bestHit.Body, 0, snippetWindow),
				SourceAttachmentPath: a.bestHit.SourceAttachmentPath,
				IsSummary:            a.bestHit.IsSummary,
				IsAttachment:         a.bestHit.IsAttachment,
			},
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

// snippetWindow is the half-width (in bytes) of the citation snippet
// extracted around a match. ~150 chars total per d-tac-lqr's AC.
const snippetWindow = 75

// snippetAround returns up to 2*window+a few chars of context around
// pos. Snippets are trimmed to whitespace boundaries when possible.
// pos<0 returns an empty snippet (no match position known).
func snippetAround(text string, pos int, window int) string {
	if text == "" || pos < 0 {
		// Fall back to the leading window when no position is known
		// (e.g. vector hits where pos is irrelevant).
		if pos < 0 && text != "" {
			return collapseWS(headSnippet(text, 2*window))
		}
		return ""
	}
	start := pos - window
	if start < 0 {
		start = 0
	}
	end := pos + window
	if end > len(text) {
		end = len(text)
	}
	return collapseWS(text[start:end])
}

func headSnippet(text string, max int) string {
	if len(text) <= max {
		return text
	}
	return text[:max]
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
		entry    *model.Entry
		score    float32
		citation query.Citation
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
				entry:    se.Entry,
				score:    contribution,
				citation: se.Citation,
				textRank: rank,
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
				// Take vector citation only if it ranked higher than text
				// (smaller rank). Keeps the citation aligned with the
				// stronger signal.
				if cur.vecRank > 0 && (cur.textRank == 0 || cur.vecRank < cur.textRank) {
					cur.citation = se.Citation
				}
			} else {
				merged[se.Entry.ID] = &fused{
					entry:    se.Entry,
					score:    contribution,
					citation: se.Citation,
					vecRank:  rank,
				}
			}
		}
	}

	out := make([]query.SearchEntry, 0, len(merged))
	for _, m := range merged {
		out = append(out, query.SearchEntry{
			Entry:    m.entry,
			Score:    m.score,
			Citation: m.citation,
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
