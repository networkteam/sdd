package presenters

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/colorprofile"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// Stats styling. Colors render only on the TTY path (RenderStatsTable); the
// agent path is plain JSON. The concrete look — colors, spacing, borders — is
// the starting point for live design review per the plan's UX-design AC, not a
// frozen spec.
var (
	statsHeaderStyle  = lipgloss.NewStyle().Bold(true)
	statsDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	statsColHeadStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
)

// RenderStatsTable writes the styled, human-facing usage report: a header and
// source line, an overall totals block, then per-model and per-op tables. Used
// on the interactive TTY path. The empty cases (no sink yet, or nothing in the
// selected range) render a single explanatory line instead of blank tables.
//
// lipgloss styles always emit ANSI at Render() time; the colorprofile writer
// downsamples to the destination's actual capability and strips color when
// NO_COLOR is set or the writer is not a terminal — so a plain io.Writer (a
// test buffer, a pipe) gets clean text and a real TTY keeps color (d-cpt-mvb).
func RenderStatsTable(dst io.Writer, r *query.StatsResult) {
	w := colorprofile.NewWriter(dst, os.Environ())
	fmt.Fprintln(w, " "+statsHeaderStyle.Render("sdd stats")+statsDimStyle.Render(" — "+rangeLabel(r.Since)))
	fmt.Fprintln(w, " "+statsDimStyle.Render(fmt.Sprintf("source: %s · %d calls", r.Source, r.Report.Totals.Calls)))
	fmt.Fprintln(w)

	if r.SinkEmpty {
		fmt.Fprintln(w, " no stats recorded yet")
		return
	}
	if r.Report.Totals.Calls == 0 {
		fmt.Fprintln(w, " no calls in the selected range")
		return
	}

	renderTotals(w, r.Report.Totals)
	fmt.Fprintln(w)
	renderModelTable(w, r.Report.ByModel)
	fmt.Fprintln(w)
	renderOpTable(w, r.Report.ByOp)
}

func renderTotals(w io.Writer, t model.StatMetrics) {
	fmt.Fprintln(w, " "+statsHeaderStyle.Render("Totals"))
	fmt.Fprintf(w, "   tokens in   %-8s  cache read   %-8s  calls  %s\n",
		humanCount(t.InputTokens), humanCount(t.CacheReadTokens), strconv.Itoa(t.Calls))
	fmt.Fprintf(w, "   tokens out  %-8s  cache write  %-8s  time   %s\n",
		humanCount(t.OutputTokens), humanCount(t.CacheCreateTokens), humanDuration(t.DurationMS))
}

func renderModelTable(w io.Writer, rows []model.ModelRollup) {
	fmt.Fprintln(w, " "+statsHeaderStyle.Render("By model"))
	data := make([][]string, 0, len(rows))
	for _, m := range rows {
		data = append(data, []string{
			m.Model, m.Provider,
			strconv.Itoa(m.Calls),
			humanCount(m.InputTokens), humanCount(m.OutputTokens),
			humanCount(m.CacheReadTokens), humanCount(m.CacheCreateTokens),
			humanDuration(m.DurationMS),
		})
	}
	fmt.Fprintln(w, statsTable(
		[]string{"MODEL", "PROVIDER", "CALLS", "IN", "OUT", "CACHE R", "CACHE W", "TIME"},
		data, 2))
}

func renderOpTable(w io.Writer, rows []model.OpRollup) {
	fmt.Fprintln(w, " "+statsHeaderStyle.Render("By operation"))
	data := make([][]string, 0, len(rows))
	for _, o := range rows {
		itemsCell, itemsPerSec := "—", "—"
		if o.HasItems() {
			itemsCell = humanCount(o.Items)
			itemsPerSec = humanRate(o.ItemsPerSec())
		}
		data = append(data, []string{
			o.Op,
			strconv.Itoa(o.Calls),
			itemsCell,
			humanCount(o.InputTokens),
			humanRate(o.TokensPerSec()),
			strconv.Itoa(int(math.Round(o.MsPerCall()))),
			itemsPerSec,
		})
	}
	fmt.Fprintln(w, statsTable(
		[]string{"OP", "CALLS", "ITEMS", "IN", "TOK/S", "MS/CALL", "ITEMS/S"},
		data, 1))
}

// statsTable builds a header-ruled, borderless table. The first leftCols
// columns are left-aligned text; the rest are right-aligned numerics.
func statsTable(headers []string, rows [][]string, leftCols int) string {
	return table.New().
		Border(lipgloss.NormalBorder()).
		BorderTop(false).BorderBottom(false).BorderLeft(false).
		BorderRight(false).BorderColumn(false).BorderRow(false).
		BorderHeader(true).
		BorderStyle(statsDimStyle).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
			if row == table.HeaderRow {
				s = s.Inherit(statsColHeadStyle)
			}
			if col >= leftCols {
				s = s.Align(lipgloss.Right)
			}
			return s
		}).
		Render()
}

// rangeLabel describes the report's time window for the header line.
func rangeLabel(since *time.Time) string {
	if since == nil {
		return "all time"
	}
	return "since " + since.Format("2006-01-02")
}

// --- JSON path ---

type statsJSON struct {
	Range   rangeJSON   `json:"range"`
	Source  string      `json:"source"`
	Totals  totalsJSON  `json:"totals"`
	ByModel []modelJSON `json:"by_model"`
	ByOp    []opJSON    `json:"by_op"`
}

type rangeJSON struct {
	Since *string `json:"since"`
	Until string  `json:"until"`
}

type totalsJSON struct {
	Calls       int   `json:"calls"`
	TokensIn    int   `json:"tokens_in"`
	TokensOut   int   `json:"tokens_out"`
	CacheRead   int   `json:"cache_read"`
	CacheCreate int   `json:"cache_create"`
	DurationMS  int64 `json:"duration_ms"`
}

type modelJSON struct {
	Model       string `json:"model"`
	Provider    string `json:"provider"`
	Calls       int    `json:"calls"`
	TokensIn    int    `json:"tokens_in"`
	TokensOut   int    `json:"tokens_out"`
	CacheRead   int    `json:"cache_read"`
	CacheCreate int    `json:"cache_create"`
	DurationMS  int64  `json:"duration_ms"`
}

type opJSON struct {
	Op           string   `json:"op"`
	Calls        int      `json:"calls"`
	Items        int      `json:"items"`
	TokensIn     int      `json:"tokens_in"`
	TokensPerSec float64  `json:"tokens_per_s"`
	MsPerCall    float64  `json:"ms_per_call"`
	ItemsPerSec  *float64 `json:"items_per_s"`
	DurationMS   int64    `json:"duration_ms"`
}

// RenderStatsJSON writes the same aggregates as structured JSON on the agent /
// non-TTY path — no styling, no chrome. An empty sink yields valid JSON with
// zeroed totals and empty arrays (a clean result, exit 0), not an error.
func RenderStatsJSON(w io.Writer, r *query.StatsResult) error {
	out := statsJSON{
		Range:   rangeJSON{Since: sinceString(r.Since), Until: r.Until.Format("2006-01-02")},
		Source:  r.Source,
		Totals:  totalsJSONFrom(r.Report.Totals),
		ByModel: []modelJSON{},
		ByOp:    []opJSON{},
	}
	for _, m := range r.Report.ByModel {
		out.ByModel = append(out.ByModel, modelJSON{
			Model: m.Model, Provider: m.Provider, Calls: m.Calls,
			TokensIn: m.InputTokens, TokensOut: m.OutputTokens,
			CacheRead: m.CacheReadTokens, CacheCreate: m.CacheCreateTokens,
			DurationMS: m.DurationMS,
		})
	}
	for _, o := range r.Report.ByOp {
		var itemsPerSec *float64
		if o.HasItems() {
			v := round1(o.ItemsPerSec())
			itemsPerSec = &v
		}
		out.ByOp = append(out.ByOp, opJSON{
			Op: o.Op, Calls: o.Calls, Items: o.Items, TokensIn: o.InputTokens,
			TokensPerSec: round1(o.TokensPerSec()), MsPerCall: round1(o.MsPerCall()),
			ItemsPerSec: itemsPerSec, DurationMS: o.DurationMS,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func totalsJSONFrom(t model.StatMetrics) totalsJSON {
	return totalsJSON{
		Calls: t.Calls, TokensIn: t.InputTokens, TokensOut: t.OutputTokens,
		CacheRead: t.CacheReadTokens, CacheCreate: t.CacheCreateTokens, DurationMS: t.DurationMS,
	}
}

func sinceString(since *time.Time) *string {
	if since == nil {
		return nil
	}
	s := since.Format("2006-01-02")
	return &s
}

// --- formatting helpers ---

// humanCount renders a raw count compactly: bare under 1000, then k / M / B
// with trailing-zero trimming (1_840_000 → "1.84M", 168_000 → "168k").
func humanCount(n int) string {
	switch {
	case n < 1000:
		return strconv.Itoa(n)
	case n < 1_000_000:
		return trimZero(fmt.Sprintf("%.1f", float64(n)/1000)) + "k"
	case n < 1_000_000_000:
		return trimZero(fmt.Sprintf("%.2f", float64(n)/1_000_000)) + "M"
	default:
		return trimZero(fmt.Sprintf("%.2f", float64(n)/1_000_000_000)) + "B"
	}
}

// humanRate renders a throughput value, keeping two significant decimals in the
// k range so comparisons stay meaningful (1390 → "1.39k", 9.2 → "9.2").
func humanRate(v float64) string {
	switch {
	case v < 1000:
		if v == math.Trunc(v) {
			return strconv.Itoa(int(v))
		}
		return trimZero(fmt.Sprintf("%.1f", v))
	case v < 1_000_000:
		return trimZero(fmt.Sprintf("%.2f", v/1000)) + "k"
	default:
		return trimZero(fmt.Sprintf("%.2f", v/1_000_000)) + "M"
	}
}

// humanDuration renders a millisecond span as the coarsest two units that fit
// (2h 14m, 36m 12s, 18s).
func humanDuration(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh %dm", int(d/time.Hour), int((d%time.Hour)/time.Minute))
	case d >= time.Minute:
		return fmt.Sprintf("%dm %ds", int(d/time.Minute), int((d%time.Minute)/time.Second))
	case d >= time.Second:
		return fmt.Sprintf("%ds", int(d/time.Second))
	case d > 0:
		return fmt.Sprintf("%dms", ms)
	default:
		return "0s"
	}
}

func trimZero(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimRight(s, ".")
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
