package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/networkteam/sdd"
)

const stdioServeHelperEnv = "SDD_STDIO_SERVE_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(stdioServeHelperEnv) == "1" {
		os.Args = []string{"sdd", "--graph-dir", ".sdd/graph", "serve", "--transport", "stdio"}
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type recordingLocalGit struct {
	committed bool
	messages  []string
	paths     [][]string
}

func (g *recordingLocalGit) HasCommitMessage(context.Context, string) (bool, error) {
	return g.committed, nil
}

func (g *recordingLocalGit) Commit(message string, paths ...string) error {
	g.committed = true
	g.messages = append(g.messages, message)
	g.paths = append(g.paths, append([]string(nil), paths...))
	return nil
}

func TestLocalGitFinalizerCommitsBatchOnce(t *testing.T) {
	git := &recordingLocalGit{}
	graphDir := t.TempDir()
	finalizer := localGitFinalizer{graphDir: graphDir, git: git}
	mutation := sdd.AppliedMutation{
		BatchID: "mutation-1",
		Batch: sdd.MutationBatch{
			Message: "sdd: signal tactical captured",
			Changes: []sdd.DocumentChange{{LogicalPath: "2026/07/13-120000-s-tac-api.md"}},
			Attachments: []sdd.AttachmentMaterialization{
				{LogicalPath: "2026/07/13-120000-s-tac-api/evidence.md"},
				{LogicalPath: "2026/07/13-120000-s-tac-api/evidence.md"},
			},
		},
	}
	if err := finalizer.Finalize(t.Context(), mutation); err != nil {
		t.Fatal(err)
	}
	if err := finalizer.Finalize(t.Context(), mutation); err != nil {
		t.Fatal(err)
	}
	if len(git.messages) != 1 || !strings.Contains(git.messages[0], "SDD-Mutation: mutation-1") {
		t.Fatalf("commits = %q", git.messages)
	}
	want := []string{
		filepath.Join(graphDir, "2026/07/13-120000-s-tac-api.md"),
		filepath.Join(graphDir, "2026/07/13-120000-s-tac-api/evidence.md"),
	}
	if len(git.paths) != 1 || !slices.Equal(git.paths[0], want) {
		t.Fatalf("paths = %v, want %v", git.paths, want)
	}
}

func TestLocalHTTPBearerAuth(t *testing.T) {
	server := httptest.NewServer(localBearerAuth("secret-token", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))
	defer server.Close()

	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request should 401, got %d", response.StatusCode)
	}
}

func TestServeStdioTransport(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".sdd", "graph"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".sdd", "config.yaml"), []byte("graph_dir: .sdd/graph\nrepo_id: example.test/stdio-smoke\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".sdd", "config.local.yaml"), []byte("participant: Stdio Smoke\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		stdioServeHelperEnv+"=1",
		"XDG_CONFIG_HOME="+filepath.Join(root, "xdg-config"),
		"XDG_CACHE_HOME="+filepath.Join(root, "xdg-cache"),
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-smoke", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect to stdio server: %v\nstderr:\n%s", err, stderr.String())
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "info", Arguments: map[string]any{}})
	if err != nil {
		_ = session.Close()
		t.Fatalf("call info over stdio: %v\nstderr:\n%s", err, stderr.String())
	}
	if result.IsError {
		_ = session.Close()
		t.Fatalf("info returned a tool error: %+v\nstderr:\n%s", result, stderr.String())
	}
	if err := session.Close(); err != nil {
		t.Fatalf("close stdio session: %v\nstderr:\n%s", err, stderr.String())
	}
}
