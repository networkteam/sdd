package model_test

import (
	"reflect"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

// A hand-written field-by-field merge has no way to notice a field nobody
// added a case for: the value is dropped in silence. These walk each config
// section by reflection, set every field on the overlay, and fail naming any
// field the merge did not carry — so adding a field without merging it breaks
// the build rather than a user's configuration.
func TestMergeConfigCarriesEveryField(t *testing.T) {
	for _, tc := range []struct {
		section string
		field   func(*model.PerRepoConfig) any
	}{
		{"LLM", func(c *model.PerRepoConfig) any { return &c.LLM }},
		{"Embedding", func(c *model.PerRepoConfig) any { return &c.Embedding }},
		{"Sync", func(c *model.PerRepoConfig) any { return &c.Sync }},
	} {
		t.Run(tc.section, func(t *testing.T) {
			overlay := &model.PerRepoConfig{}
			v := reflect.ValueOf(tc.field(overlay)).Elem()
			for i := 0; i < v.NumField(); i++ {
				setDistinctive(t, v.Field(i))
			}

			merged := model.MergeConfig(&model.PerRepoConfig{}, overlay)
			got := reflect.ValueOf(tc.field(merged)).Elem()
			typ := got.Type()
			for i := 0; i < got.NumField(); i++ {
				if got.Field(i).IsZero() {
					t.Errorf("merge drops %s.%s: set on the overlay, absent from the result", tc.section, typ.Field(i).Name)
				}
			}
		})
	}
}

// The concrete behaviour the reflective guard cannot state: a map field
// overlays key by key rather than wholesale, and the base layer is not mutated.
func TestMergeConfigParamsOverlayKeyByKey(t *testing.T) {
	base := &model.PerRepoConfig{}
	base.LLM.Params = map[string]string{"think": "high", "seed": "1"}
	overlay := &model.PerRepoConfig{}
	overlay.LLM.Params = map[string]string{"think": "low"}

	merged := model.MergeConfig(base, overlay)

	if got := merged.LLM.Params["think"]; got != "low" {
		t.Errorf("think = %q, want the overlay's low", got)
	}
	if got := merged.LLM.Params["seed"]; got != "1" {
		t.Errorf("seed = %q, want the base's 1 to survive", got)
	}
	if got := base.LLM.Params["think"]; got != "high" {
		t.Errorf("base was mutated: think = %q, want high", got)
	}
}

// setDistinctive puts a non-zero value of the right shape into the field.
func setDistinctive(t *testing.T, f reflect.Value) {
	t.Helper()
	switch f.Kind() {
	case reflect.String:
		f.SetString("x")
	case reflect.Int, reflect.Int64:
		f.SetInt(7)
	case reflect.Float64:
		f.SetFloat(1.5)
	case reflect.Map:
		m := reflect.MakeMap(f.Type())
		m.SetMapIndex(reflect.ValueOf("k"), reflect.ValueOf("v"))
		f.Set(m)
	case reflect.Struct:
		for i := 0; i < f.NumField(); i++ {
			setDistinctive(t, f.Field(i))
		}
	default:
		t.Fatalf("no distinctive value for kind %s — extend this helper", f.Kind())
	}
}
