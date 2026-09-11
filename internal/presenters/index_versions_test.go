package presenters_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
)

func sampleIndexVersions() []*query.IndexVersionsResult {
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	return []*query.IndexVersionsResult{{
		Label: "local", IndexDir: "/cache/index/repo/fp", Entries: 2, Bytes: 3 * 1024 * 1024,
		Groups: []query.IndexVersionGroup{
			{Name: "v1-current", Versions: 2, Entries: 2, Oldest: t0, Newest: t0.Add(24 * time.Hour), Bytes: 2 * 1024 * 1024},
			{Name: "v0", Versions: 1, Entries: 1, Oldest: t0.Add(-48 * time.Hour), Newest: t0.Add(-48 * time.Hour), Bytes: 1024 * 1024, Droppable: true},
		},
	}}
}

func TestRenderIndexVersionsTable(t *testing.T) {
	var buf bytes.Buffer
	presenters.RenderIndexVersionsTable(&buf, sampleIndexVersions())
	out := buf.String()
	for _, want := range []string{"sdd index gc", "local", "3.0 MB · 2 entries", "GROUP", "VERSIONS", "v1-current", "kept", "v0", "droppable", "2026-08-30"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("non-terminal buffer received ANSI styling:\n%s", out)
	}
	t.Logf("rendered table:\n%s", out)
}

func TestRenderIndexVersionsTableEmptyStore(t *testing.T) {
	var buf bytes.Buffer
	presenters.RenderIndexVersionsTable(&buf, []*query.IndexVersionsResult{{Label: "local"}})
	if !strings.Contains(buf.String(), "no stored versions") {
		t.Fatalf("expected empty-store line, got:\n%s", buf.String())
	}
}

func TestRenderIndexVersionsJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := presenters.RenderIndexVersionsJSON(&buf, sampleIndexVersions()); err != nil {
		t.Fatal(err)
	}
	var out []struct {
		Store  string `json:"store"`
		Bytes  int64  `json:"bytes"`
		Groups []struct {
			Group     string  `json:"group"`
			Oldest    *string `json:"oldest"`
			Droppable bool    `json:"droppable"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(out) != 1 || out[0].Store != "local" || len(out[0].Groups) != 2 {
		t.Fatalf("json shape = %+v", out)
	}
	if g := out[0].Groups[1]; g.Group != "v0" || !g.Droppable || g.Oldest == nil || *g.Oldest != "2026-08-30T00:00:00Z" {
		t.Errorf("v0 group = %+v", g)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{0: "0 B", 512: "512 B", 1536: "1.5 KB", 213 * 1024 * 1024: "213 MB", 3 * 1024 * 1024 * 1024: "3.0 GB"}
	for in, want := range cases {
		if got := presenters.HumanBytes(in); got != want {
			t.Errorf("HumanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
