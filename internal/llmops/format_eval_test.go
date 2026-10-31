//go:build eval

// Format evals over the example set in testdata/eval/format (plan
// 20261031-141038-d-tac-p8l): every run of every specimen must return a
// response the operation's parser accepts. They are a stopgap that `sdd eval`
// replaces over the same files (20261031-174216-d-tac-p83), so every response
// is recorded raw for its judge to score later without new model calls.
//
//	go test -tags=eval -run 'Eval_Format' -parallel 4 ./internal/llmops/... -v
//
// SDD_EVAL_RESPONSES_DIR selects where the rows append (<dir>/responses.jsonl);
// unset, the run writes to a fresh temp dir and prints its path.

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

	"github.com/networkteam/sdd/internal/model"
)

const formatSpecimenDir = "testdata/eval/format"

type formatSpecimen struct {
	ID         string          `yaml:"id"`
	Origin     string          `yaml:"origin"`
	Class      string          `yaml:"class"`
	Language   string          `yaml:"language"`
	Operations []string        `yaml:"operations"`
	Graph      []specimenEntry `yaml:"graph"`
	Draft      specimenEntry   `yaml:"draft"`
}

type specimenEntry struct {
	ID      string      `yaml:"id"`
	Type    string      `yaml:"type"`
	Kind    string      `yaml:"kind"`
	Layer   string      `yaml:"layer"`
	Refs    []model.Ref `yaml:"refs"`
	Content string      `yaml:"content"`
}

// loadFormatSpecimens returns the specimens listing operation, failing the
// test on any malformed file so a typo cannot silently shrink the set.
func loadFormatSpecimens(t *testing.T, operation string) []formatSpecimen {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(formatSpecimenDir, "*.yaml"))
	if err != nil {
		t.Fatalf("listing format specimens: %v", err)
	}
	var out []formatSpecimen
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		dec := yaml.NewDecoder(strings.NewReader(string(raw)))
		dec.KnownFields(true)
		var s formatSpecimen
		if err := dec.Decode(&s); err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		if want := strings.TrimSuffix(filepath.Base(path), ".yaml"); s.ID != want {
			t.Fatalf("%s: id %q does not match the file name", path, s.ID)
		}
		if slices.Contains(s.Operations, operation) {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		t.Fatalf("no format specimens for %s in %s", operation, formatSpecimenDir)
	}
	return out
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

func (s formatSpecimen) graph(t *testing.T) *model.Graph {
	t.Helper()
	entries := make([]*model.Entry, 0, len(s.Graph))
	for _, e := range s.Graph {
		entries = append(entries, e.entry(t))
	}
	return model.NewGraph(entries)
}

// responseRow is one recorded run: what the judge of `sdd eval` needs to score
// it later, keyed to the specimen file and the prompt revision it ran under.
type responseRow struct {
	Time         time.Time `json:"time"`
	Specimen     string    `json:"specimen"`
	Operation    string    `json:"operation"`
	Run          int       `json:"run"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Variant      string    `json:"variant,omitempty"`
	PromptSHA256 string    `json:"prompt_sha256,omitempty"`
	Raw          string    `json:"raw"`
	Valid        bool      `json:"valid"`
	Error        string    `json:"error,omitempty"`
	ErrorClass   string    `json:"error_class,omitempty"`
}

var (
	responsesOnce sync.Once
	responsesMu   sync.Mutex
	responsesFile *os.File
	responsesErr  error
)

func recordResponse(t *testing.T, row responseRow) {
	t.Helper()
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
			fmt.Printf("eval responses: %s\n", path)
		}
	})
	if responsesErr != nil {
		t.Fatalf("opening eval responses file: %v", responsesErr)
	}
	line, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("encoding response row: %v", err)
	}
	responsesMu.Lock()
	defer responsesMu.Unlock()
	if _, err := responsesFile.Write(append(line, '\n')); err != nil {
		t.Fatalf("writing response row: %v", err)
	}
}

// runFormatEval runs one specimen through an operation Runs times (the strict
// tier, SDD_EVAL_RUNS rescales) and fails unless every run returned a response
// the operation parsed. A transport failure fails the run too, classed apart.
func runFormatEval(t *testing.T, s formatSpecimen, operation string, once func(*testing.T) (*capturingRunner, error)) {
	t.Helper()
	rate := blockingTier.withRunsOverride()
	failed := 0
	for i := 1; i <= rate.Runs; i++ {
		runner, err := once(t)
		row := responseRow{
			Time: time.Now().UTC(), Specimen: s.ID, Operation: operation, Run: i,
			Provider: runner.lastIdentity.Provider, Model: runner.lastIdentity.Model, Variant: runner.lastIdentity.Variant,
			PromptSHA256: runner.lastPromptSHA256, Raw: runner.lastText, Valid: err == nil,
		}
		if err != nil {
			failed++
			row.Error = err.Error()
			row.ErrorClass = "format"
			if runner.lastErr != nil {
				row.ErrorClass = "transport"
			}
			t.Logf("run %d/%d FAIL (%s): %v\nRaw output:\n%s", i, rate.Runs, row.ErrorClass, err, runner.lastText)
		} else {
			t.Logf("run %d/%d valid", i, rate.Runs)
		}
		recordResponse(t, row)
	}
	if failed > 0 {
		t.Errorf("%s on %s (%s): %d of %d runs returned no parseable response", operation, s.ID, s.Class, failed, rate.Runs)
	}
}

func promptSHA256(system, user string) string {
	sum := sha256.Sum256([]byte(system + "\x00" + user))
	return hex.EncodeToString(sum[:])
}
