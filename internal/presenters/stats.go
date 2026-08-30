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

// RenderStatsTable writes the styled, human-facing usage report: a header and
// source line, a totals block of the counters that genuinely aggregate, then
// the per-model and per-op tables where the rates and latency distributions
// live — those cuts group calls that are alike, which is what makes a
// distribution mean anything. Used
// on the interactive TTY path. The empty cases (no sink yet, or nothing in the
// selected range) render a single explanatory line instead of blank tables.
//
// lipgloss styles always emit ANSI at Render() time; the colorprofile writer
// downsamples to the destination's actual capability and strips color when
// NO_COLOR is set or the writer is not a terminal — so a plain io.Writer (a
// test buffer, a pipe) gets clean text and a real TTY keeps color (d-cpt-mvb).
func RenderStatsTable(dst io.Writer, r *query.StatsResult) {
	w := colorprofile.NewWriter(dst, os.Environ())
	fmt.Fprintln(w, " "+clrHeading.Render("sdd stats")+clrBody.Render(" — "+rangeLabel(r.Since)))
	fmt.Fprintln(w, " "+clrBody.Render(fmt.Sprintf("source: %s · %d calls", r.Source, r.Report.Totals.Calls)))
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
	fmt.Fprintln(w, " "+clrHeading.Render("Totals"))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "   tokens in   %-8s  cache read   %-8s  calls  %s\n",
		humanCount(t.InputTokens), humanCount(t.CacheReadTokens), strconv.Itoa(t.Calls))
	fmt.Fprintf(w, "   tokens out  %-8s  cache write  %-8s  time   %s\n",
		humanCount(t.OutputTokens), humanCount(t.CacheCreateTokens), humanDuration(t.DurationMS))
	if t.Errors > 0 {
		fmt.Fprintf(w, "   %s\n", clrWarn.Render(fmt.Sprintf("failed      %d of %d calls", t.Errors, t.Calls)))
	}
}

func renderModelTable(w io.Writer, rows []model.ModelRollup) {
	fmt.Fprintln(w, " "+clrHeading.Render("By model"))
	fmt.Fprintln(w)
	data := make([][]string, 0, len(rows))
	for _, m := range rows {
		l := m.Latency()
		data = append(data, []string{
			m.Label(), m.Provider,
			strconv.Itoa(m.Calls),
			humanCount(m.InputTokens), humanCount(m.OutputTokens),
			humanCount(m.CacheReadTokens), humanCount(m.CacheCreateTokens),
			outRate(m.StatMetrics),
			humanLatency(l.P50), humanLatency(l.P90), humanLatency(l.Max),
		})
	}
	fmt.Fprintln(w, statsTable(
		[]string{"MODEL", "PROVIDER", "CALLS", "IN", "OUT", "CACHE R", "CACHE W", "OUT/S", "P50", "P90", "MAX"},
		data, 2))
}

func renderOpTable(w io.Writer, rows []model.OpRollup) {
	fmt.Fprintln(w, " "+clrHeading.Render("By operation"))
	fmt.Fprintln(w)
	data := make([][]string, 0, len(rows))
	for _, o := range rows {
		itemsCell := "—"
		if o.HasItems() {
			itemsCell = humanCount(o.Items)
		}
		failedCell := "—"
		if o.Errors > 0 {
			failedCell = strconv.Itoa(o.Errors)
		}
		l := o.Latency()
		data = append(data, []string{
			o.Op,
			strconv.Itoa(o.Calls),
			failedCell,
			itemsCell,
			humanCount(o.InputTokens),
			humanCount(o.OutputTokens),
			outRate(o.StatMetrics),
			humanLatency(l.P50),
			humanLatency(l.P90),
			humanLatency(l.Max),
		})
	}
	fmt.Fprintln(w, statsTable(
		[]string{"OP", "CALLS", "FAILED", "ITEMS", "IN", "OUT", "OUT/S", "P50", "P90", "MAX"},
		data, 1))
}

// outRate renders the generation rate, blank for the embedding ops that
// generate nothing rather than printing a misleading zero.
func outRate(m model.StatMetrics) string {
	if m.OutputTokens == 0 {
		return "—"
	}
	return humanRate(m.OutputTokensPerSec())
}

// statsTable builds a header-ruled, borderless table. The first leftCols
// columns are left-aligned text; the rest are right-aligned numerics.
func statsTable(headers []string, rows [][]string, leftCols int) string {
	return table.New().
		Border(lipgloss.NormalBorder()).
		BorderTop(false).BorderBottom(false).BorderLeft(false).
		BorderRight(false).BorderColumn(false).BorderRow(false).
		BorderHeader(true).
		BorderStyle(clrFaint).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
			if row == table.HeaderRow {
				s = s.Inherit(clrKey)
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

type latencyJSON struct {
	P50MS int64 `json:"p50_ms"`
	P90MS int64 `json:"p90_ms"`
	P99MS int64 `json:"p99_ms"`
	MaxMS int64 `json:"max_ms"`
}

func latencyJSONFrom(m model.StatMetrics) latencyJSON {
	l := m.Latency()
	return latencyJSON{P50MS: l.P50, P90MS: l.P90, P99MS: l.P99, MaxMS: l.Max}
}

type totalsJSON struct {
	Calls       int   `json:"calls"`
	Errors      int   `json:"errors"`
	TokensIn    int   `json:"tokens_in"`
	TokensOut   int   `json:"tokens_out"`
	CacheRead   int   `json:"cache_read"`
	CacheCreate int   `json:"cache_create"`
	DurationMS  int64 `json:"duration_ms"`
}

type modelJSON struct {
	Model              string      `json:"model"`
	Variant            string      `json:"variant,omitempty"`
	Provider           string      `json:"provider"`
	Calls              int         `json:"calls"`
	Errors             int         `json:"errors"`
	TokensIn           int         `json:"tokens_in"`
	TokensOut          int         `json:"tokens_out"`
	CacheRead          int         `json:"cache_read"`
	CacheCreate        int         `json:"cache_create"`
	DurationMS         int64       `json:"duration_ms"`
	Latency            latencyJSON `json:"latency"`
	OutputTokensPerSec float64     `json:"output_tokens_per_s"`
}

type opJSON struct {
	Op           string  `json:"op"`
	Calls        int     `json:"calls"`
	Errors       int     `json:"errors"`
	Items        int     `json:"items"`
	TokensIn     int     `json:"tokens_in"`
	TokensOut    int     `json:"tokens_out"`
	TokensPerSec float64 `json:"tokens_per_s"`

	OutputTokensPerSec float64     `json:"output_tokens_per_s"`
	ItemsPerSec        *float64    `json:"items_per_s"`
	DurationMS         int64       `json:"duration_ms"`
	Latency            latencyJSON `json:"latency"`
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
			Model: m.Model, Variant: m.Variant, Provider: m.Provider, Calls: m.Calls, Errors: m.Errors,
			TokensIn: m.InputTokens, TokensOut: m.OutputTokens,
			CacheRead: m.CacheReadTokens, CacheCreate: m.CacheCreateTokens,
			DurationMS: m.DurationMS, Latency: latencyJSONFrom(m.StatMetrics),
			OutputTokensPerSec: round1(m.OutputTokensPerSec()),
		})
	}
	for _, o := range r.Report.ByOp {
		var itemsPerSec *float64
		if o.HasItems() {
			v := round1(o.ItemsPerSec())
			itemsPerSec = &v
		}
		out.ByOp = append(out.ByOp, opJSON{
			Op: o.Op, Calls: o.Calls, Errors: o.Errors, Items: o.Items,
			TokensIn: o.InputTokens, TokensOut: o.OutputTokens,
			TokensPerSec: round1(o.TokensPerSec()), OutputTokensPerSec: round1(o.OutputTokensPerSec()),
			ItemsPerSec: itemsPerSec, DurationMS: o.DurationMS,
			Latency: latencyJSONFrom(o.StatMetrics),
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func totalsJSONFrom(t model.StatMetrics) totalsJSON {
	return totalsJSON{
		Calls: t.Calls, Errors: t.Errors, TokensIn: t.InputTokens, TokensOut: t.OutputTokens,
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

// humanLatency renders one call's wall-clock time. It keeps a decimal in the
// seconds range where humanDuration rounds to whole seconds — the difference
// between a 1.2s and a 1.9s median is the point of reporting it at all.
func humanLatency(ms int64) string {
	switch d := time.Duration(ms) * time.Millisecond; {
	case ms <= 0:
		return "—"
	case d < time.Second:
		return fmt.Sprintf("%dms", ms)
	case d < time.Minute:
		return trimZero(fmt.Sprintf("%.1f", d.Seconds())) + "s"
	default:
		return fmt.Sprintf("%dm %ds", int(d/time.Minute), int((d%time.Minute)/time.Second))
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
