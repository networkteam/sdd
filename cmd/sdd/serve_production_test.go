package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/networkteam/sdd/internal/index"
)

// fakeOllama is an Ollama-compatible embedding server (POST /api/embed). It
// returns deterministic vectors per input text — stable across processes so a
// warm store never needs re-embedding — and records every input it saw so a
// test can prove document vs query embedding activity.
type fakeOllama struct {
	mu     sync.Mutex
	inputs []string
	calls  int
}

func (f *fakeOllama) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/embed" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var req struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.calls++
	f.inputs = append(f.inputs, req.Input...)
	f.mu.Unlock()

	embeddings := make([][]float32, len(req.Input))
	for i, text := range req.Input {
		embeddings[i] = fakeVector(text)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": embeddings, "prompt_eval_count": len(req.Input)})
}

func (f *fakeOllama) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inputs, f.calls = nil, 0
}

func (f *fakeOllama) snapshot() (calls int, inputs []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, append([]string(nil), f.inputs...)
}

func fakeVector(text string) []float32 {
	sum := sha256.Sum256([]byte(text))
	v := make([]float32, 8)
	for i := range v {
		v[i] = float32(binary.BigEndian.Uint32(sum[i*4:i*4+4]))/float32(^uint32(0)) + 0.01
	}
	return v
}

// TestServeUsesPersistentMachineGlobalIndex is the production-path regression
// guard for s-tac-ex4: a fresh `sdd serve` must answer phrase search from the
// machine-global index the CLI built, without re-embedding graph documents.
// It drives the real command wiring as subprocesses against a fake
// Ollama-compatible server and a real temporary XDG cache — counters and store
// identity, never elapsed time.
func TestServeUsesPersistentMachineGlobalIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess production-path test")
	}
	ollama := &fakeOllama{}
	server := httptest.NewServer(ollama)
	defer server.Close()

	root := t.TempDir()
	graphRel := filepath.Join(".sdd", "graph")
	graphDir := filepath.Join(root, graphRel)
	if err := os.MkdirAll(graphDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const repoID = "example.test/prod"
	config := "graph_dir: .sdd/graph\n" +
		"default_branch: main\n" +
		"repo_id: " + repoID + "\n" +
		"embedding:\n" +
		"  provider: ollama\n" +
		"  model: fake-model\n" +
		"  ollama_endpoint: " + server.URL + "\n"
	if err := os.WriteFile(filepath.Join(root, ".sdd", "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".sdd", "config.local.yaml"), []byte("participant: Prod Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeProdEntry(t, graphDir, "20260101-100000-s-tac-aaa", "Alpha orchard notes", "## Section\nThe alpha entry is about apple orchards.")
	writeProdEntry(t, graphDir, "20260101-100001-s-tac-bbb", "Beta cultivation notes", "## Section\nThe beta entry is about beta cultivation.")

	cacheRoot := filepath.Join(root, "xdg-cache", "sdd")

	// Seed the index through the real CLI indexing path.
	runProdCLI(t, root, "--graph-dir .sdd/graph index")
	if calls, _ := ollama.snapshot(); calls == 0 {
		t.Fatal("CLI index made no embedding calls — nothing was seeded")
	}

	// MCP and CLI must resolve the same machine-global store directory.
	storeDir := index.StoreDir(cacheRoot, index.RepoKey(repoID, root), "ollama/fake-model")
	manifest, err := index.LoadManifest(storeDir)
	if err != nil {
		t.Fatalf("load seeded manifest at %s: %v", storeDir, err)
	}
	if len(manifest.Entries) < 2 {
		t.Fatalf("seeded manifest has %d entries at %s, want >= 2", len(manifest.Entries), storeDir)
	}
	seededEntries := len(manifest.Entries)

	// First fresh server: phrase search answers warm — one query embed, zero
	// document embeds.
	ollama.reset()
	first := prodServeSearch(t, root)
	if !strings.Contains(first, "s-tac-aaa") && !strings.Contains(first, "s-tac-bbb") {
		t.Fatalf("first serve search surfaced no seeded entry: %q", first)
	}
	assertWarm(t, ollama, "first server")

	// Second fresh server: still warm.
	ollama.reset()
	prodServeSearch(t, root)
	assertWarm(t, ollama, "second server")

	// Add one entry; a third fresh server embeds only that entry's chunks.
	writeProdEntry(t, graphDir, "20260101-100002-s-tac-ccc", "Zeta topic notes", "## Section\nThe zeta entry mentions zeta only.")
	ollama.reset()
	prodServeSearch(t, root)
	calls, inputs := ollama.snapshot()
	var docInputs, queryInputs int
	for _, in := range inputs {
		if in == "alpha" {
			queryInputs++
			continue
		}
		docInputs++
		if !strings.Contains(strings.ToLower(in), "zeta") {
			t.Errorf("third server embedded a document that is not the new entry: %q", in)
		}
	}
	if queryInputs != 1 {
		t.Errorf("third server query embeds = %d, want 1", queryInputs)
	}
	if docInputs == 0 {
		t.Error("third server embedded no document for the new entry")
	}
	if calls < 2 {
		t.Errorf("third server made %d embed calls, want >= 2 (new entry + query)", calls)
	}
	if got := manifestEntryCount(t, storeDir); got != seededEntries+1 {
		t.Errorf("store grew to %d entries, want %d (only the new entry added)", got, seededEntries+1)
	}

	// Fourth fresh server: warm again — the new entry persisted.
	ollama.reset()
	prodServeSearch(t, root)
	assertWarm(t, ollama, "fourth server")

	// No second store topology was created — the production path did not fall
	// back to a process-local memory store.
	if dirs := storeDirsUnder(t, filepath.Join(cacheRoot, "index")); len(dirs) != 1 {
		t.Errorf("expected exactly one machine-global store directory, found %v", dirs)
	}
}

// assertWarm asserts a serve process made exactly one embedding call carrying
// one input (the query) — zero document embeds.
func assertWarm(t *testing.T, ollama *fakeOllama, label string) {
	t.Helper()
	calls, inputs := ollama.snapshot()
	if calls != 1 || len(inputs) != 1 {
		t.Errorf("%s: %d embed calls with %d inputs %v; want 1 call with 1 input (query only, zero document embeds)", label, calls, len(inputs), inputs)
	}
}

func writeProdEntry(t *testing.T, graphDir, id, summary, body string) {
	t.Helper()
	dir := filepath.Join(graphDir, id[:4], id[4:6])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntype: signal\nkind: gap\nlayer: tactical\nconfidence: high\nparticipants:\n  - Christopher\n"
	content += "summary: " + summary + "\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, id[6:]+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runProdCLI runs the real sdd CLI as a subprocess (via the TestMain helper),
// waits for it to finish, and fails on a non-zero exit.
func runProdCLI(t *testing.T, root, args string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Dir = root
	cmd.Env = prodEnv(root, mainHelperArgsEnv+"="+args)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sdd %s: %v\n%s", args, err, out)
	}
}

// prodServeSearch starts a fresh `sdd serve` subprocess, calls the search tool
// over MCP stdio, and returns the rendered result text.
func prodServeSearch(t *testing.T, root string) string {
	t.Helper()
	const phrase = "alpha"
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Dir = root
	cmd.Env = prodEnv(root, stdioServeHelperEnv+"=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	client := mcp.NewClient(&mcp.Implementation{Name: "prod-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect serve: %v\nstderr:\n%s", err, stderr.String())
	}
	defer func() { _ = session.Close() }()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search", Arguments: map[string]any{"query": phrase}})
	if err != nil {
		t.Fatalf("search over stdio: %v\nstderr:\n%s", err, stderr.String())
	}
	if result.IsError {
		t.Fatalf("search returned a tool error: %+v\nstderr:\n%s", result, stderr.String())
	}
	var text strings.Builder
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	return text.String()
}

func prodEnv(root string, extra ...string) []string {
	env := append(os.Environ(),
		"XDG_CONFIG_HOME="+filepath.Join(root, "xdg-config"),
		"XDG_CACHE_HOME="+filepath.Join(root, "xdg-cache"),
	)
	return append(env, extra...)
}

func manifestEntryCount(t *testing.T, storeDir string) int {
	t.Helper()
	manifest, err := index.LoadManifest(storeDir)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	return len(manifest.Entries)
}

// storeDirsUnder returns the leaf store directories (those holding a
// manifest.json) beneath root.
func storeDirsUnder(t *testing.T, root string) []string {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "manifest.json" {
			dirs = append(dirs, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return dirs
}
