//go:build search_eval

// Manual recall benchmark for the search index. Hits a real embedding
// provider, so it is gated behind the `search_eval` build tag and skipped
// from the default test run. Driven by `testdata/eval_pairs.csv`:
// hand-curated should-be-related entry pairs from this repo's own graph.
//
// Run with Ollama:
//
//	SDD_EVAL_PROVIDER=ollama \
//	SDD_EVAL_OLLAMA_ENDPOINT=http://localhost:11434 \
//	SDD_EVAL_OLLAMA_MODEL=nomic-embed-text \
//	go test -tags=search_eval -run TestEval ./internal/finders/... -v
//
// Run with OpenAI:
//
//	SDD_EVAL_PROVIDER=openai \
//	SDD_EVAL_OPENAI_API_KEY=sk-... \
//	SDD_EVAL_OPENAI_MODEL=text-embedding-3-small \
//	go test -tags=search_eval -run TestEval ./internal/finders/... -v
//
// The graph corpus defaults to this repo's `.sdd/graph`; override with
// SDD_EVAL_GRAPH_DIR to point at a different graph.

package finders_test

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/index"
	localembed "github.com/networkteam/sdd/internal/llm/embed"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

const evalTopN = 10

type evalPair struct {
	SourceID  string
	RelatedID string
	Modes     map[query.SearchMode]bool
	Note      string
}

func loadEvalPairs(t *testing.T) []evalPair {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "eval_pairs.csv"))
	if err != nil {
		t.Fatalf("open eval_pairs.csv: %v", err)
	}
	defer f.Close()

	rdr := csv.NewReader(f)
	rdr.TrimLeadingSpace = true
	records, err := rdr.ReadAll()
	if err != nil {
		t.Fatalf("read eval pairs: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("eval_pairs.csv has %d rows; expected header plus at least 10 pairs", len(records))
	}
	var pairs []evalPair
	for i, row := range records[1:] { // skip header
		if len(row) < 4 {
			t.Fatalf("row %d malformed: %v", i+2, row)
		}
		modes := map[query.SearchMode]bool{}
		for _, m := range strings.Split(row[2], "|") {
			modes[query.SearchMode(strings.TrimSpace(m))] = true
		}
		pairs = append(pairs, evalPair{
			SourceID:  strings.TrimSpace(row[0]),
			RelatedID: strings.TrimSpace(row[1]),
			Modes:     modes,
			Note:      row[3],
		})
	}
	return pairs
}

func evalEmbedder(t *testing.T) handlers.IndexEmbedder {
	t.Helper()
	provider := os.Getenv("SDD_EVAL_PROVIDER")
	if provider == "" {
		t.Skip("SDD_EVAL_PROVIDER not set (use openai or ollama)")
	}

	// Explicit timeout: an 8b local model can take >30s per batch even
	// at the default batch size; leave headroom for cold-start
	// per-batch latency.
	cfg := model.EmbeddingConfig{Provider: provider, Timeout: "5m"}
	switch provider {
	case "ollama":
		cfg.Model = os.Getenv("SDD_EVAL_OLLAMA_MODEL")
		if cfg.Model == "" {
			t.Skip("SDD_EVAL_OLLAMA_MODEL not set (e.g. nomic-embed-text)")
		}
		cfg.OllamaEndpoint = os.Getenv("SDD_EVAL_OLLAMA_ENDPOINT")
	case "openai":
		cfg.Model = os.Getenv("SDD_EVAL_OPENAI_MODEL")
		if cfg.Model == "" {
			t.Skip("SDD_EVAL_OPENAI_MODEL not set (e.g. text-embedding-3-small)")
		}
		key := os.Getenv("SDD_EVAL_OPENAI_API_KEY")
		if key == "" {
			t.Skip("SDD_EVAL_OPENAI_API_KEY not set")
		}
		cfg.APIKeys = map[string]string{"openai": key}
		cfg.Endpoint = os.Getenv("SDD_EVAL_OPENAI_ENDPOINT")
	default:
		t.Fatalf("unknown SDD_EVAL_PROVIDER %q", provider)
	}
	emb, err := localembed.New(cfg)
	if err != nil {
		t.Fatalf("build embedder: %v", err)
	}
	return handlers.IndexEmbedder{Embedder: emb, BatchSize: localembed.BatchSize(cfg)}
}

func TestEvalRecall(t *testing.T) {
	graphDir := os.Getenv("SDD_EVAL_GRAPH_DIR")
	if graphDir == "" {
		// Default to this repo's own .sdd/graph (where the eval pairs
		// were curated from). Walk up from the test file's working
		// directory to find it.
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		graphDir = filepath.Join(cwd, "..", "..", ".sdd", "graph")
	}
	if _, err := os.Stat(graphDir); err != nil {
		t.Skipf("graph dir %s not accessible: %v", graphDir, err)
	}

	emb := evalEmbedder(t)
	pairs := loadEvalPairs(t)
	t.Logf("loaded %d eval pairs from testdata/eval_pairs.csv", len(pairs))

	// Build a fresh index in a temp dir so the test doesn't pollute the
	// project's .sdd/index.
	indexDir := t.TempDir()

	reader := finders.New(finders.Options{PreflightRunner: noopRunner{}})
	ih := handlers.NewIndexHandler(handlers.IndexHandlerOptions{
		GraphDir: graphDir,
		IndexDir: indexDir,
		Embedder: emb,
		Reader:   reader,
	})

	t.Logf("building index over %s ...", graphDir)
	start := time.Now()
	if err := ih.Build(context.Background(), &command.BuildIndexCmd{
		OnComplete: func(indexed, skipped int) {
			t.Logf("indexed %d entries (%d skipped) in %s",
				indexed, skipped, time.Since(start).Round(time.Millisecond))
		},
	}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	g, err := reader.LoadGraph(graphDir)
	if err != nil {
		t.Fatalf("LoadGraph: %v", err)
	}

	// Open the index after the build so the finder reads the committed rows.
	idxStore, err := index.Open(indexDir)
	if err != nil {
		t.Fatal(err)
	}
	finder := finders.NewSearchFinder(finders.SearchFinderOptions{
		Graph:      g,
		GraphDir:   graphDir,
		Embedder:   emb,
		IndexStore: idxStore,
	})

	type modeStat struct{ hits, total int }
	stats := map[query.SearchMode]*modeStat{
		query.SearchModeText:   {},
		query.SearchModeVector: {},
		query.SearchModeHybrid: {},
	}

	for _, p := range pairs {
		src := g.ByID[p.SourceID]
		if src == nil {
			t.Errorf("source %s not in graph (curate eval_pairs.csv against the same corpus)", p.SourceID)
			continue
		}
		if g.ByID[p.RelatedID] == nil {
			t.Errorf("related %s not in graph", p.RelatedID)
			continue
		}

		phrase := firstSentence(src.Summary)
		// Term derives from the first non-stopword token of the summary
		// — a coarse but reproducible proxy for "what would a user
		// type?" Specific tokens that the source and related entry
		// likely share (per CSV note) are best — the eval CSV is the
		// source of truth here.
		term := firstSignificantToken(src.Summary)

		for mode := range p.Modes {
			st := stats[mode]
			st.total++
			q := query.SearchQuery{
				IncludeSuperseded: true, // eval covers historical pairs too
				Limit:             evalTopN,
			}
			switch mode {
			case query.SearchModeText:
				q.Terms = []string{term}
			case query.SearchModeVector:
				q.Phrase = phrase
			case query.SearchModeHybrid:
				q.Terms = []string{term}
				q.Phrase = phrase
			}
			res, err := finder.Search(context.Background(), q)
			if err != nil {
				t.Errorf("[%s] %s search: %v", mode, p.SourceID, err)
				continue
			}
			rank := -1
			for i, se := range res.Entries {
				if se.Entry.ID == p.RelatedID {
					rank = i + 1
					break
				}
			}
			if rank > 0 {
				st.hits++
				t.Logf("[%s] %s → %s rank=%d  (note: %s)", mode, shortID(p.SourceID), shortID(p.RelatedID), rank, p.Note)
			} else {
				t.Logf("[%s] MISS %s → %s  (note: %s)", mode, shortID(p.SourceID), shortID(p.RelatedID), p.Note)
			}
		}
	}

	for mode, st := range stats {
		if st.total == 0 {
			continue
		}
		recall := float64(st.hits) / float64(st.total)
		t.Logf("recall@%d [%s] = %d/%d = %.0f%%",
			evalTopN, mode, st.hits, st.total, recall*100)
	}
}

// firstSentence trims to the first sentence terminator. Heuristic — same
// rule the splitter applies for Entry: preambles.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	for i, r := range s {
		if r == '.' || r == '!' || r == '?' {
			next := i + 1
			if next >= len(s) {
				return s
			}
			c := s[next]
			if c == ' ' || c == '\t' || c == '\n' {
				return s[:next]
			}
		}
	}
	return s
}

var stopwords = map[string]bool{
	"the": true, "a": true, "an": true, "this": true, "that": true,
	"and": true, "or": true, "but": true, "of": true, "in": true,
	"on": true, "at": true, "to": true, "from": true, "for": true,
	"is": true, "are": true, "was": true, "were": true, "be": true,
}

// firstSignificantToken returns the first non-stopword whitespace token,
// stripped of trailing punctuation. Coarse — the eval CSV ultimately
// dictates which pairs the test grades, and this is just the term we'd
// pass through --term.
func firstSignificantToken(s string) string {
	for _, raw := range strings.Fields(s) {
		w := strings.Trim(raw, ".,!?;:()\"'")
		if w == "" {
			continue
		}
		if stopwords[strings.ToLower(w)] {
			continue
		}
		return w
	}
	return ""
}

// shortID renders the {type}-{layer}-{suffix} portion for log brevity.
func shortID(id string) string {
	if len(id) < 16 {
		return id
	}
	return id[16:] // drop the YYYYMMDD-HHmmss prefix
}

// noopRunner satisfies llm.Runner for the read-only LoadGraph path.
type noopRunner struct{}

func (noopRunner) Run(context.Context, llm.Request) (*llm.RunResult, error) {
	return nil, fmt.Errorf("noop")
}
