//go:build eval

// Eval specimens in testdata/eval/specimens (20261102-101250-d-tac-s98):
// drafts with problem characters and the content a response must carry. Every
// eval call is also recorded raw, so the judge of `sdd eval`
// (20261031-174216-d-tac-p83) can score it later without new model calls.
//
// SDD_EVAL_RESPONSES_DIR selects where the rows append (<dir>/responses.jsonl)
// and where the cost summary goes (<dir>/cost-summary.md); unset, the run
// writes to a fresh temp dir and prints its path.

package llmops_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/networkteam/sdd/internal/llmops"
	"github.com/networkteam/sdd/internal/model"
)

const specimenDir = "testdata/eval/specimens"

type evalSpecimen struct {
	ID        string          `yaml:"id"`
	Origin    string          `yaml:"origin"`
	Class     string          `yaml:"class"`
	Language  string          `yaml:"language"`
	Operation string          `yaml:"operation"`
	Expect    specimenExpect  `yaml:"expect"`
	Graph     []specimenEntry `yaml:"graph"`
	Draft     specimenEntry   `yaml:"draft"`
}

// specimenExpect is the finding a response must carry and the draft passage
// its quote, or an excerpt its observation cites, must locate.
type specimenExpect struct {
	Axis       string `yaml:"axis"`
	Severity   string `yaml:"severity"`
	Passage    string `yaml:"passage"`
	NoBlocking bool   `yaml:"no_blocking"`
}

type specimenEntry struct {
	ID      string      `yaml:"id"`
	Type    string      `yaml:"type"`
	Kind    string      `yaml:"kind"`
	Layer   string      `yaml:"layer"`
	Refs    []model.Ref `yaml:"refs"`
	Content string      `yaml:"content"`
}

// loadSpecimens returns the specimens for operation, failing the test on any
// malformed file so a typo cannot silently shrink the set.
func loadSpecimens(t *testing.T, operation string) []evalSpecimen {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(specimenDir, "*.yaml"))
	if err != nil {
		t.Fatalf("listing specimens: %v", err)
	}
	var out []evalSpecimen
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		dec := yaml.NewDecoder(strings.NewReader(string(raw)))
		dec.KnownFields(true)
		var s evalSpecimen
		if err := dec.Decode(&s); err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		if want := strings.TrimSuffix(filepath.Base(path), ".yaml"); s.ID != want {
			t.Fatalf("%s: id %q does not match the file name", path, s.ID)
		}
		if err := s.validate(); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if s.Operation == operation {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		t.Fatalf("no specimens for %s in %s", operation, specimenDir)
	}
	return out
}

func (s evalSpecimen) validate() error {
	e := s.Expect
	switch s.Operation {
	case "writing-guide":
		if e.Axis == "" || e.Passage == "" || e.Severity != "" || e.NoBlocking {
			return fmt.Errorf("a writing-guide specimen expects an axis and a passage")
		}
	case "preflight":
		clean := e.NoBlocking && e.Severity == "" && e.Passage == ""
		finding := !e.NoBlocking && e.Severity != "" && e.Passage != ""
		if e.Axis != "" || !(clean || finding) {
			return fmt.Errorf("a preflight specimen expects either no_blocking or a severity and a passage")
		}
	default:
		return fmt.Errorf("unknown operation %q", s.Operation)
	}
	if e.Passage != "" && !quoteLocatesIn(s.Draft.Content, e.Passage, "") {
		return fmt.Errorf("passage %q is not in the draft", e.Passage)
	}
	return nil
}

// tier holds a clean draft to the blocking bar and an expected finding to the
// advisory one.
func (e specimenExpect) tier() passRate {
	if e.NoBlocking {
		return blockingTier
	}
	return advisoryTier
}

func (e specimenExpect) checkGuide(content string, r *llmops.WritingGuideResult) error {
	for _, f := range r.Findings {
		if f.Axis == e.Axis && quoteLocatesIn(content, f.Quote, e.Passage) {
			return nil
		}
	}
	return fmt.Errorf("no %s finding whose quote locates %q", e.Axis, e.Passage)
}

func (e specimenExpect) checkPreflight(content string, r *llmops.PreflightResult) error {
	if e.NoBlocking {
		return noBlocking(r)
	}
	for _, f := range r.Findings {
		if string(f.Severity) != e.Severity {
			continue
		}
		for _, excerpt := range citedExcerpts(f.Observation) {
			if quoteLocatesIn(content, excerpt, e.Passage) {
				return nil
			}
		}
	}
	return fmt.Errorf("no %s finding citing an excerpt that locates %q", e.Severity, e.Passage)
}

func (e specimenEntry) entry(t *testing.T) *model.Entry {
	t.Helper()
	typ := model.EntryType(e.Type)
	if _, ok := model.TypeAbbrev[typ]; !ok {
		t.Fatalf("specimen entry %q: unknown type %q", e.ID, e.Type)
	}
	return &model.Entry{
		ID:      e.ID,
		Type:    typ,
		Kind:    model.Kind(e.Kind),
		Layer:   model.Layer(e.Layer),
		Refs:    e.Refs,
		Content: e.Content,
		Time:    time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC),
	}
}

func (s evalSpecimen) graph(t *testing.T) *model.Graph {
	t.Helper()
	entries := make([]*model.Entry, 0, len(s.Graph))
	for _, e := range s.Graph {
		entries = append(entries, e.entry(t))
	}
	return model.NewGraph(entries)
}

// responseRow is one recorded call: what the judge of `sdd eval` needs to
// score it later, keyed to the test and the prompt revision it ran under.
type responseRow struct {
	Time           time.Time `json:"time"`
	Test           string    `json:"test"`
	Purpose        string    `json:"purpose"`
	Provider       string    `json:"provider"`
	Model          string    `json:"model"`
	Variant        string    `json:"variant,omitempty"`
	PromptSHA256   string    `json:"prompt_sha256"`
	ResponseSchema bool      `json:"response_schema"`
	Raw            string    `json:"raw"`
	Error          string    `json:"error,omitempty"`
	InputTokens    int       `json:"input_tokens"`
	OutputTokens   int       `json:"output_tokens"`
	// CostUSD is the cost the provider reported for the call; absent when it
	// reported none (20261101-010604-d-tac-769).
	CostUSD float64 `json:"cost_usd,omitempty"`
}

var (
	responsesOnce sync.Once
	responsesMu   sync.Mutex
	responsesFile *os.File
	responsesDir  string
	responsesErr  error
	costs         = map[costKey]*costRow{}
)

// costKey is one evaluated combination: the role it played, what served the
// call, and at which setting.
type costKey struct{ role, provider, model, variant string }

type costRow struct {
	calls, failed, costed int
	inTokens, outTokens   int
	costUSD               float64
}

func recordResponse(t *testing.T, row responseRow) {
	responsesOnce.Do(func() {
		dir := os.Getenv("SDD_EVAL_RESPONSES_DIR")
		if dir == "" {
			dir, responsesErr = os.MkdirTemp("", "sdd-eval-responses-")
			if responsesErr != nil {
				return
			}
		} else if responsesErr = os.MkdirAll(dir, 0o755); responsesErr != nil {
			return
		}
		path := filepath.Join(dir, "responses.jsonl")
		responsesFile, responsesErr = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if responsesErr == nil {
			responsesDir = dir
			fmt.Printf("eval responses: %s\n", path)
		}
	})
	if responsesErr != nil {
		t.Errorf("opening eval responses file: %v", responsesErr)
		return
	}
	line, err := json.Marshal(row)
	if err != nil {
		t.Errorf("encoding response row: %v", err)
		return
	}
	responsesMu.Lock()
	defer responsesMu.Unlock()
	if _, err := responsesFile.Write(append(line, '\n')); err != nil {
		t.Errorf("writing response row: %v", err)
	}
	role := "candidate"
	if row.Purpose == "eval-judge" {
		role = "judge"
	}
	key := costKey{role, row.Provider, row.Model, row.Variant}
	c := costs[key]
	if c == nil {
		c = &costRow{}
		costs[key] = c
	}
	c.calls++
	if row.Error != "" {
		c.failed++
	}
	if row.CostUSD > 0 {
		c.costed++
		c.costUSD += row.CostUSD
	}
	c.inTokens += row.InputTokens
	c.outTokens += row.OutputTokens
}

// printCostSummary reports calls and cost per evaluated combination, on stdout
// and as cost-summary.md beside the responses (the workflow's job summary).
// Cost reads "not reported" when no call of a combination carried one; a
// partial sum is marked as such.
func printCostSummary() {
	responsesMu.Lock()
	defer responsesMu.Unlock()
	if len(costs) == 0 {
		return
	}
	keys := make([]costKey, 0, len(costs))
	for k := range costs {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b costKey) int {
		return strings.Compare(a.role+a.provider+a.model+a.variant, b.role+b.provider+b.model+b.variant)
	})
	var b strings.Builder
	b.WriteString("| Role | Provider | Model | Setting | Calls (failed) | Tokens in / out | Cost (USD) |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")
	for _, k := range keys {
		c := costs[k]
		cost := "not reported"
		if c.costed > 0 {
			cost = fmt.Sprintf("%.4f", c.costUSD)
			if c.costed < c.calls {
				cost += fmt.Sprintf(" (%d of %d calls)", c.costed, c.calls)
			}
		}
		setting := k.variant
		if setting == "" {
			setting = "defaults"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %d (%d) | %d / %d | %s |\n",
			k.role, k.provider, k.model, setting, c.calls, c.failed, c.inTokens, c.outTokens, cost)
	}
	fmt.Printf("\n=== eval cost — this run ===\n%s", b.String())
	if responsesDir != "" {
		path := filepath.Join(responsesDir, "cost-summary.md")
		if err := os.WriteFile(path, []byte("## Cost\n\n"+b.String()), 0o644); err != nil {
			fmt.Printf("writing %s: %v\n", path, err)
		}
	}
}

func promptSHA256(system, user string) string {
	sum := sha256.Sum256([]byte(system + "\x00" + user))
	return hex.EncodeToString(sum[:])
}
