package cliapp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
	"github.com/urfave/cli/v3"
)

func exportCmd() *cli.Command {
	return &cli.Command{
		Name:  "export",
		Usage: "EXPERIMENTAL: export the graph and its declared dependencies with derived status and heat as one JSON document (unstable shape)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "format",
				Value: "json",
				Usage: "Output format: json (the only one)",
			},
			&cli.StringFlag{
				Name:  "dependencies",
				Value: string(query.ExportDependenciesReferenced),
				Usage: "What the declared dependency closure contributes: none, referenced (entries the local graph cites, plus --hops upstream steps) or all (whole graphs)",
			},
			&cli.IntFlag{
				Name:  "hops",
				Value: 1,
				Usage: "Upstream steps (refs, closes, supersedes) followed from cited dependency entries with --dependencies referenced; 0 = only the cited ones",
			},
			&cli.StringFlag{
				Name:  "out",
				Usage: "Write the document to this file (replaced atomically) instead of stdout",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if format := cmd.String("format"); format != "json" {
				return fmt.Errorf("--format: unsupported value %q (only json)", format)
			}
			mode, err := query.ParseExportDependencies(cmd.String("dependencies"))
			if err != nil {
				return fmt.Errorf("--dependencies: %w", err)
			}
			hops := int(cmd.Int("hops"))
			if hops < 0 {
				return fmt.Errorf("--hops: must not be negative, got %d", hops)
			}
			if cmd.IsSet("hops") && mode != query.ExportDependenciesReferenced {
				return fmt.Errorf("--hops applies only to --dependencies referenced")
			}

			dir, err := resolveGraphDir(cmd)
			if err != nil {
				return err
			}
			f, err := newReadFinder()
			if err != nil {
				return err
			}
			if mode != query.ExportDependenciesNone {
				if err := freshenDependencyClosure(ctx, cmd.ErrWriter, f); err != nil {
					return err
				}
			}
			g, err := f.CurrentGraph(dir)
			if err != nil {
				return err
			}
			result, err := f.OnGraph(g).Export(ctx, query.ExportQuery{
				Dependencies: mode, Hops: hops, Now: time.Now(),
			})
			if err != nil {
				return err
			}
			if out := cmd.String("out"); out != "" {
				return writeFileAtomic(out, func(w io.Writer) error { return presenters.RenderExportJSON(w, result) })
			}
			return presenters.RenderExportJSON(cmd.Writer, result)
		},
	}
}

// freshenDependencyClosure brings the caches of the declared dependency
// closure up to date before the export assembles its graphs. A freshened cache
// can declare more, so the closure is recomputed until freshening changes
// nothing.
func freshenDependencyClosure(ctx context.Context, errWriter io.Writer, f *finders.Finder) error {
	h, err := repoHandler(errWriter)
	if err != nil {
		return err
	}
	for {
		closure, err := f.DependencyClosure(ctx, query.DependencyClosureQuery{})
		if err != nil {
			return err
		}
		changed, err := h.EnsureReposFresh(ctx, command.EnsureReposFreshCmd{RepoIDs: closure})
		if err != nil || !changed {
			return err
		}
	}
}

// writeFileAtomic renders into a temp file beside path and renames it over
// path, so a reader never sees a partial document. The file keeps the temp
// file's owner-only mode: the document carries entry bodies and attachment
// text.
func writeFileAtomic(path string, render func(io.Writer) error) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if err = render(tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
