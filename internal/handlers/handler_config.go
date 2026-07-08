package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/repos"
)

// ConfigSet writes one key to the chosen config layer with a
// comment-preserving upsert. The patched document is validated against the
// layer's schema before anything lands on disk, so a key that does not
// belong in that file fails loud and leaves the file untouched. The global
// file is created (with its parent directory) on first write.
func (h *Handler) ConfigSet(ctx context.Context, cmd *command.ConfigSetCmd) error {
	var path string
	switch cmd.Target {
	case "global":
		if h.repos == nil {
			return fmt.Errorf("no global config support configured")
		}
		path = h.repos.Registry().ConfigPath()
	case "local":
		if h.sddDir == "" {
			return fmt.Errorf("not inside an sdd repo — `--local` needs .sdd/ (run `sdd init` first)")
		}
		path = filepath.Join(h.sddDir, "config.local.yaml")
	default:
		return fmt.Errorf("unknown config target %q (use global or local)", cmd.Target)
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	patched, err := model.SetYAMLField(existing, cmd.Key, model.ParseConfigScalar(cmd.Value))
	if err != nil {
		return fmt.Errorf("setting %s: %w", cmd.Key, err)
	}

	// Validate the patched document against the layer's schema before
	// writing — the strict decoder rejects keys foreign to this file.
	switch cmd.Target {
	case "global":
		var probe repos.GlobalConfig
		if err := model.StrictUnmarshalYAML(patched, &probe); err != nil {
			return fmt.Errorf("%q is not a valid global config key: %w", cmd.Key, err)
		}
	case "local":
		if _, err := model.ParseConfig(patched); err != nil {
			return fmt.Errorf("%q is not a valid per-repo config key: %w", cmd.Key, err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	// 0600 like repos.SaveConfigTo — either layer may carry API keys.
	if err := os.WriteFile(path, patched, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
