package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func runViewCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	app := &cli.Command{
		Name:     "sdd",
		Writer:   &out,
		Commands: []*cli.Command{viewCmd()},
	}
	err := app.Run(context.Background(), append([]string{"sdd", "view"}, args...))
	return out.String(), err
}

func TestViewHelpOwnsLayoutReference(t *testing.T) {
	out, err := runViewCommand(t, "--help")
	if err != nil {
		t.Fatalf("view --help: %v", err)
	}

	for _, want := range []string{
		"Compose custom graph views with filters, ranking, transforms, and renderers",
		"LAYOUT REFERENCE:",
		"Implemented pipeline vocabulary:",
		"Macros (named pipelines",
		"sdd view --layout=top(20)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("view --help output missing %q", want)
		}
	}
	if strings.Contains(out, "mechanical catch-up") {
		t.Errorf("view --help retains catch-up-only command description")
	}
}

func TestBareViewRequiresLayoutAndDirectsToHelp(t *testing.T) {
	out, err := runViewCommand(t)
	if err == nil {
		t.Fatal("bare view succeeded, want missing-layout error")
	}
	if out != "" {
		t.Fatalf("bare view wrote help to stdout: %q", out)
	}
	for _, want := range []string{"--layout is required", "sdd view --help"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("bare view error %q missing %q", err, want)
		}
	}
}

func TestEmptyViewLayoutDirectsToHelp(t *testing.T) {
	_, err := runViewCommand(t, "--layout=")
	if err == nil {
		t.Fatal("empty layout succeeded, want error")
	}
	for _, want := range []string{"--layout: empty value", "sdd view --help"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("empty-layout error %q missing %q", err, want)
		}
	}
}
