package handlers

import (
	"context"
	"fmt"
	"reflect"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/repos"
)

// ConfigSet writes one key to the chosen config layer, rejecting a key that
// does not belong in that file before anything lands on disk.
func (h *Handler) ConfigSet(ctx context.Context, cmd *command.ConfigSetCmd) error {
	var (
		file  configFile
		probe any
		typ   reflect.Type
		layer string
		err   error
	)
	switch cmd.Target {
	case "global":
		file, err = h.globalConfigFile()
		probe, typ, layer = &repos.GlobalConfig{}, reflect.TypeFor[repos.GlobalConfig](), "global"
	case "local":
		file, err = h.localConfigFile()
		probe, typ, layer = &model.PerRepoConfig{}, reflect.TypeFor[model.PerRepoConfig](), "per-repo"
	default:
		return fmt.Errorf("unknown config target %q (use global or local)", cmd.Target)
	}
	if err != nil {
		return err
	}

	return file.patch(func(existing []byte) ([]byte, error) {
		patched, err := model.SetYAMLField(existing, cmd.Key, model.ParseConfigScalar(cmd.Value))
		if err != nil {
			return nil, fmt.Errorf("setting %s: %w", cmd.Key, err)
		}
		// Judged per key rather than over the document, so a key some other
		// version wrote does not make this file unwritable.
		if !model.KnownYAMLKey(cmd.Key, typ) {
			return nil, fmt.Errorf("%q is not a valid %s config key", cmd.Key, layer)
		}
		if err := model.UnmarshalYAML(patched, probe); err != nil {
			return nil, fmt.Errorf("%s = %s is not valid for the %s config: %w", cmd.Key, cmd.Value, layer, err)
		}
		return patched, nil
	})
}
