package finders

import (
	"os"
	"path/filepath"
	"reflect"

	"github.com/networkteam/sdd/internal/meta"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/repos"
)

// EffectiveConfig resolves the config overlay layer by layer and reports
// each effective value with the layer that supplied it. The finder reads the
// layers itself (rather than using the merged construction-time snapshot)
// because provenance is exactly the information merging discards.
func (f *Finder) EffectiveConfig(q query.EffectiveConfigQuery) (*query.EffectiveConfigResult, error) {
	var global model.BaseConfig
	if f.repos != nil {
		gcfg, err := f.repos.Load()
		if err != nil {
			return nil, err
		}
		global = gcfg.BaseConfig
	}
	var project, local *model.PerRepoConfig
	if q.SDDDir != "" {
		var err error
		project, local, err = meta.ReadConfigLayers(q.SDDDir)
		if err != nil {
			return nil, err
		}
	}

	values := model.EffectiveConfigValues(global, project, local)
	result := &query.EffectiveConfigResult{}
	for _, v := range values {
		if q.Key != "" && v.Key != q.Key {
			continue
		}
		result.Entries = append(result.Entries, query.ConfigEntry{
			Key:    v.Key,
			Value:  v.Value,
			Source: string(v.Source),
			Secret: v.Secret,
		})
	}
	return result, nil
}

// UnknownConfigKeys reports the keys sdd read past because it does not know
// them — the one computation behind every surface that says so, rather than
// each deciding for itself what counts as unknown.
func (f *Finder) UnknownConfigKeys(q query.UnknownConfigKeysQuery) (*query.UnknownConfigKeysResult, error) {
	type layer struct {
		path string
		typ  reflect.Type
	}
	var layers []layer
	if f.repos != nil {
		layers = append(layers, layer{f.repos.ConfigPath(), reflect.TypeFor[repos.GlobalConfig]()})
	}
	if q.SDDDir != "" {
		for _, name := range []string{"config.yaml", "config.local.yaml"} {
			layers = append(layers, layer{filepath.Join(q.SDDDir, name), reflect.TypeFor[model.PerRepoConfig]()})
		}
	}

	result := &query.UnknownConfigKeysResult{}
	for _, l := range layers {
		data, err := os.ReadFile(l.path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		keys, err := model.UnknownYAMLKeys(data, l.typ)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			result.Keys = append(result.Keys, query.UnknownConfigKey{File: l.path, Key: k})
		}
	}
	return result, nil
}
