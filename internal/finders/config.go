package finders

import (
	"github.com/networkteam/sdd/internal/meta"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
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
