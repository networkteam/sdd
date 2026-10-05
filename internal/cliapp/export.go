package cliapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
	sddapp "github.com/networkteam/sdd/pkg/application"
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

			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			var dependencyIDs []string
			if mode != query.ExportDependenciesNone && cfg != nil {
				if dependencyIDs, err = dependencyClosure(ctx, cmd.ErrWriter, cfg); err != nil {
					return err
				}
			}

			dir, err := resolveGraphDir(cmd)
			if err != nil {
				return err
			}
			f, err := newReadFinder()
			if err != nil {
				return err
			}
			g, err := f.CurrentGraph(dir)
			if err != nil {
				return err
			}
			result, err := f.OnGraph(g).Export(ctx, query.ExportQuery{
				Dependencies: mode, DependencyIDs: dependencyIDs, Hops: hops, Now: time.Now(),
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

// dependencyClosure resolves the repo's declared dependencies transitively:
// each reached repo's cache is freshened, then its own declarations are read
// from its committed config with the reader `sdd serve` composes dependency
// projects from.
func dependencyClosure(ctx context.Context, errWriter io.Writer, cfg *model.PerRepoConfig) ([]string, error) {
	reg, _, err := defaultRepos()
	if err != nil {
		return nil, err
	}
	return model.DependencyClosure(cfg.RepoID, cfg.Dependencies, func(repoID string) ([]string, error) {
		if err := freshenRepoCaches(ctx, errWriter, []string{repoID}); err != nil {
			return nil, err
		}
		cacheDir, err := reg.CacheDir(repoID)
		if err != nil {
			return nil, err
		}
		dependencyCfg, err := sddapp.ReadProjectConfigFS(os.DirFS(cacheDir))
		if errors.Is(err, sddapp.ErrNotAnSDDProject) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("reading config of dependency %s: %w", repoID, err)
		}
		return dependencyCfg.Dependencies, nil
	})
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
