package cliapp

import (
	"context"
	"fmt"
	"sync"

	"github.com/urfave/cli/v3"

	"github.com/networkteam/sdd/internal/cliout"
	clitui "github.com/networkteam/sdd/internal/cliout/tui"
	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/model"
)

// fillMissing runs `sdd summarize --all`: it states what the fill costs in
// model calls and runs it with a progress bar on a terminal and a line per
// entry otherwise.
func fillMissing(ctx context.Context, cmd *cli.Command, handler *handlers.Handler, sumCmd *command.SummarizeCmd) error {
	out := cmd.ErrWriter
	if !sumCmd.Force {
		graph, err := loadGraph(cmd)
		if err != nil {
			return err
		}
		summaryAndTitle, summaryOnly, titleOnly := model.TitleBackfill(graph.Entries)
		fmt.Fprintf(out, "Filling: %d entries get a title beside their stored summary, %d entries get a summary beside their stored title, %d entries get a summary and a title -> %d model calls.\n", titleOnly, summaryOnly, summaryAndTitle, titleOnly+summaryOnly+summaryAndTitle)
	}
	if !cliout.IsInteractive(out) {
		return handler.Summarize(ctx, sumCmd)
	}
	progress := newFillProgress(sumCmd)
	_, err := clitui.Interactive(ctx, transientViewPolicy(),
		clitui.View{Reader: cmd.Reader, Writer: out, InitialPhase: model.PhaseSummarizing, Progress: progress},
		func(ctx context.Context) (struct{}, error) {
			return struct{}{}, handler.Summarize(ctx, sumCmd)
		})
	return err
}

// newFillProgress reroutes a fill's per-entry callbacks onto a footer
// reporter: on a terminal the bar replaces the line per entry. Summarize
// workers fire the callbacks concurrently; the reporter takes one producer.
func newFillProgress(fill *command.SummarizeCmd) *cliout.Reporter {
	reporter := cliout.NewReporter()
	reporter.SetUnit("entries")
	fill.OnPlanned = func(n int) {
		reporter.SetPhase(model.PhaseSummarizing)
		reporter.SetTotal(n)
	}
	var mu sync.Mutex
	advance := func(id, _ string) {
		mu.Lock()
		defer mu.Unlock()
		reporter.Add(1)
		reporter.SetNote(id)
	}
	fill.OnSummarized = advance
	fill.OnTitled = advance
	fill.OnSkipped = nil
	return reporter
}
