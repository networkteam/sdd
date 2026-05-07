package finders

import (
	"fmt"
	"sort"

	"github.com/networkteam/sdd/internal/model"
)

// groupableFields lists every entry attribute that `group(by(<field>))`
// can dispatch on. Slice 5 covers the macro-driven need (kind, used by
// the decisions and signals macros) plus layer and type for ad-hoc use.
// Adding a field is a one-line append here plus a case in fieldExtractor.
var groupableFields = []string{"kind", "layer", "type"}

// groupBy buckets entries by the named field and returns the buckets in
// alphabetical order of the bucket key. Within each bucket entries
// preserve their input order — slice 5 doesn't sort within groups (no
// per-group rank in v1; reserved for a later slice).
//
// Unknown fields produce a listed-valid-set error so users have a clear
// recovery path, mirroring how unknown-function errors guide the user
// to the known vocabulary.
func groupBy(entries []*model.Entry, field string) ([]model.Group, error) {
	extract, ok := fieldExtractor(field)
	if !ok {
		return nil, fmt.Errorf("unknown field %q (known: %v)", field, groupableFields)
	}

	// Two-pass: first pass populates the per-key buckets in input order;
	// second pass orders the keys alphabetically. Keeping input-order
	// inside buckets and alphabetical-order across buckets gives stable
	// rendering without coupling to upstream filter ordering.
	buckets := map[string][]*model.Entry{}
	for _, e := range entries {
		key := extract(e)
		buckets[key] = append(buckets[key], e)
	}

	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	groups := make([]model.Group, 0, len(keys))
	for _, k := range keys {
		groups = append(groups, model.Group{Key: k, Entries: buckets[k]})
	}
	return groups, nil
}

// fieldExtractor returns the function that pulls the group key off an
// entry for the named field. Returning a closure avoids the per-entry
// switch on field-name inside groupBy's hot loop.
func fieldExtractor(field string) (func(*model.Entry) string, bool) {
	switch field {
	case "kind":
		return func(e *model.Entry) string { return string(e.Kind) }, true
	case "layer":
		return func(e *model.Entry) string { return string(e.Layer) }, true
	case "type":
		return func(e *model.Entry) string { return string(e.Type) }, true
	default:
		return nil, false
	}
}
