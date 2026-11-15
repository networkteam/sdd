package cliapp_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCLIHelpAndFlags(t *testing.T) {
	f := newAppFixture(t)
	for _, tt := range []struct {
		name     string
		args     []string
		stdout   []string
		stderr   []string
		errText  string
		exitCode int
	}{
		{name: "root help", args: []string{"--help"}, stdout: []string{"Signal-Dialogue-Decision graph tool", "--graph-dir", "show", "serve"}},
		{name: "command help", args: []string{"show", "--help"}, stdout: []string{"Show entry with upstream and downstream summary chains", "--with-summary"}},
		{name: "configured version", args: []string{"--version"}, stdout: []string{"sdd version " + appVersion + "\n"}},
		{name: "short verbose flag is not version", args: []string{"-v", "info"}, stdout: []string{"Local participant: Local Author"}},
		{name: "unknown flag reports usage on error stream", args: []string{"--does-not-exist"}, stdout: []string{"USAGE:"}, stderr: []string{"Incorrect Usage:", "does-not-exist"}, errText: "flag provided but not defined", exitCode: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := f.run(t, tt.args...)
			if result.exitCode() != tt.exitCode {
				t.Fatalf("exit = %d, want %d: %v\n%s", result.exitCode(), tt.exitCode, result.err, result.stderr)
			}
			if tt.errText != "" && (result.err == nil || !strings.Contains(result.err.Error(), tt.errText)) {
				t.Errorf("error = %v, want %q", result.err, tt.errText)
			}
			assertContainsOutput(t, "stdout", result.stdout, tt.stdout...)
			assertContainsOutput(t, "stderr", result.stderr, tt.stderr...)
			if strings.Contains(result.stdout, "Incorrect Usage:") {
				t.Errorf("usage diagnostic leaked into stdout: %s", result.stdout)
			}
		})
	}
}

func TestCLIReadsConfiguredGraph(t *testing.T) {
	f := newAppFixture(t)
	for _, tt := range []struct {
		name    string
		args    []string
		want    []string
		exclude string
	}{
		{name: "info uses local config overlay", args: []string{"info"}, want: []string{"Local participant: Local Author", "Language: de", "Search: text"}, exclude: "Global Author"},
		{name: "show resolves configured graph directory", args: []string{"show", "--format", "text", appEntryID}, want: []string{appEntryID, appBody}, exclude: appSummary},
		{name: "show forwards summary flag to renderer", args: []string{"show", "--format", "text", "--with-summary", appEntryID}, want: []string{appEntryID, appBody, appSummary}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := f.run(t, tt.args...)
			if result.err != nil {
				t.Fatalf("sdd %v: %v\n%s", tt.args, result.err, result.stderr)
			}
			assertContainsOutput(t, "stdout", result.stdout, tt.want...)
			if tt.exclude != "" && strings.Contains(result.stdout, tt.exclude) {
				t.Errorf("stdout unexpectedly contains %q: %s", tt.exclude, result.stdout)
			}
			if strings.Contains(result.stdout, "sync:") {
				t.Errorf("sync diagnostic leaked into command output: %s", result.stdout)
			}
		})
	}
}

func TestCLIReturnsCommandErrorsWithoutResult(t *testing.T) {
	f := newAppFixture(t)
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "show requires an entry", args: []string{"show"}, want: "usage: sdd show <id>"},
		{name: "view requires a layout", args: []string{"view"}, want: "--layout is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := f.run(t, tt.args...)
			if result.err == nil || !strings.Contains(result.err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", result.err, tt.want)
			}
			if result.stdout != "" {
				t.Errorf("failed command emitted a result: %s", result.stdout)
			}
		})
	}
}

func TestCLIParticipantWarningUsesErrorStream(t *testing.T) {
	f := newAppFixture(t)
	f.write(t, "config/sdd/config.yaml", "{}\n")
	f.write(t, ".sdd/config.local.yaml", "{}\n")

	result := f.run(t, "info")

	if result.err != nil {
		t.Fatalf("info: %v\n%s", result.err, result.stderr)
	}
	assertContainsOutput(t, "stdout", result.stdout, "Local participant: (not configured", "Language: de")
	assertContainsOutput(t, "stderr", result.stderr, "sdd: no participant configured", "sdd init")
	if strings.Contains(result.stdout, "sdd: no participant configured") {
		t.Errorf("warning leaked into command output: %s", result.stdout)
	}
}

func TestCLIWriteHandlerUsesErrorStream(t *testing.T) {
	f := newAppFixture(t)
	result := f.run(t, "new", "--confidence", "high", "--skip-preflight", "--dry-run", "s", "tac", "A dry-run signal.")
	if result.err != nil {
		t.Fatalf("new --dry-run: %v\n%s", result.err, result.stderr)
	}
	assertContainsOutput(t, "stderr", result.stderr, "warning: pre-flight validation skipped")
	if result.stdout != "" {
		t.Errorf("dry run emitted a result: %s", result.stdout)
	}
}

func TestNewRefusesABlankTitle(t *testing.T) {
	for _, title := range []string{"", "   "} {
		t.Run(fmt.Sprintf("%q", title), func(t *testing.T) {
			f := newFillFixture(t)
			result := f.run(t, "new", "--confidence", "high", "--skip-preflight", "--summary", "A supplied summary.", "--title", title, "s", "tac", "A signal with a blank title.")
			if result.err == nil || !strings.Contains(result.err.Error(), "--title value is empty") {
				t.Fatalf("a blank --title should be refused, got %v\n%s", result.err, result.stderr)
			}
			if result.stdout != "" {
				t.Errorf("no entry should be written, got %s", result.stdout)
			}
		})
	}
}

func TestNewDryRunKeepsPipedStdinOnAnEmptyTitle(t *testing.T) {
	f := newFillFixture(t)
	var stdout, stderr bytes.Buffer
	command := f.command(strings.NewReader("piped notes"), &stdout, &stderr)
	err := command.Run(t.Context(), []string{"sdd", "new", "--confidence", "high", "--skip-preflight", "--dry-run", "--attach", "-:notes.md", "--title", "", "s", "tac", "A dry run with an empty title."})
	if err == nil || !strings.Contains(err.Error(), "--title value is empty") {
		t.Fatalf("an empty --title should be refused, got %v", err)
	}
	if !strings.Contains(stderr.String(), "stdin attachment saved (dry-run)") {
		t.Errorf("the piped attachment should be saved for the retry:\n%s", stderr.String())
	}
}

func TestNewBuildsTheEmbedderOnlyForATitleCall(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantFails bool
	}{
		{name: "a supplied summary and title", args: []string{"--summary", "A supplied summary.", "--title", "Supplied title"}},
		{name: "a dry run", args: []string{"--dry-run"}},
		{name: "a title call", args: []string{"--summary", "A supplied summary."}, wantFails: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFillFixture(t)
			f.write(t, ".sdd/config.yaml", f.read(t, ".sdd/config.yaml")+"embedding:\n  provider: ollama\n")
			args := append([]string{"new", "--confidence", "high", "--skip-preflight"}, tc.args...)
			result := f.run(t, append(args, "s", "tac", "A signal beside a broken embedding config.")...)
			failed := result.err != nil && strings.Contains(result.err.Error(), "embedding.model is required")
			if failed != tc.wantFails || (!tc.wantFails && result.err != nil) {
				t.Errorf("only a title call should read the embedding config, got %v\n%s", result.err, result.stderr)
			}
		})
	}
}

func TestCLIServeUsesSuppliedStreams(t *testing.T) {
	f := newAppFixture(t)
	f.serve(t, func(client *mcp.ClientSession) {
		if version := client.InitializeResult().ServerInfo.Version; version != appVersion {
			t.Errorf("MCP version = %q, want supplied CLI version %q", version, appVersion)
		}
		tools, err := client.ListTools(t.Context(), nil)
		if err != nil || len(tools.Tools) == 0 {
			t.Fatalf("CLI serve did not expose tools: %+v, %v", tools, err)
		}
	})
}
