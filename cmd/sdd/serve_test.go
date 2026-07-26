package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	sdd "github.com/networkteam/sdd/application"
	localadapter "github.com/networkteam/sdd/local"
)

const (
	stdioServeHelperEnv = "SDD_STDIO_SERVE_HELPER"
	// mainHelperArgsEnv carries a space-separated sdd argv for a subprocess
	// that runs the real CLI (e.g. the production-path test seeds the index
	// via `sdd index`). The subprocess re-enters main() with these args.
	mainHelperArgsEnv = "SDD_MAIN_HELPER_ARGS"
)

func TestMain(m *testing.M) {
	if args := os.Getenv(mainHelperArgsEnv); args != "" {
		os.Args = append([]string{"sdd"}, strings.Fields(args)...)
		main()
		os.Exit(0)
	}
	if os.Getenv(stdioServeHelperEnv) == "1" {
		os.Args = []string{"sdd", "--graph-dir", ".sdd/graph", "serve", "--transport", "stdio"}
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestLocalGitFinalizerCommitsBatchOnce(t *testing.T) {
	checkout := canonicalTempDir(t)
	runGit := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", checkout}, args...)...)
		out, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s (%v)", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	runGit("init", "-b", "main")
	runGit("config", "user.name", "Test")
	runGit("config", "user.email", "test@example.invalid")
	runGit("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(checkout, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "README.md")
	runGit("commit", "-m", "fixture")
	graphDir := filepath.Join(checkout, ".sdd", "graph")
	entryPath := filepath.Join(graphDir, "2026/07/13-120000-s-tac-api.md")
	attachmentPath := filepath.Join(graphDir, "2026/07/13-120000-s-tac-api", "evidence.md")
	if err := os.MkdirAll(filepath.Dir(attachmentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entryPath, []byte("entry\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(attachmentPath, []byte("evidence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	finalizer := localadapter.GitFinalizer{Checkout: checkout, GraphDir: ".sdd/graph", Branch: "main"}
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
	if count := runGit("log", "--fixed-strings", "--grep=SDD-Mutation: mutation-1", "--format=%H"); len(strings.Fields(count)) != 1 {
		t.Fatalf("matching commits = %q", count)
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
	root := canonicalTempDir(t)
	if err := os.MkdirAll(filepath.Join(root, ".sdd", "graph"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".sdd", "config.yaml"), []byte("graph_dir: .sdd/graph\ndefault_branch: main\nrepo_id: example.test/stdio-smoke\n"), 0o644); err != nil {
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
