package finders

import (
	"fmt"
	"sort"

	"github.com/networkteam/sdd/internal/model"
)

// groupableFields lists every entry attribute that `group(by(<field>))`
// can dispatch on. kind drives the decisions and signals macros; layer
// and type cover ad-hoc use; participant buckets entries by author.
// Adding a field is a one-line append here plus a case in fieldExtractor.
var groupableFields = []string{"kind", "layer", "type", "participant"}

// groupBy buckets entries by the named field and returns the buckets in
// alphabetical order of the bucket key. Within each bucket entries
// preserve their input order (no per-group rank in v1).
//
// A field may be multi-valued (participant): an entry co-authored by two
// participants lands in both buckets, so per-bucket counts can exceed the
// entry total and an entry with no values for the field contributes to no
// bucket. Single-valued fields yield exactly one bucket per entry.
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
		for _, key := range extract(e) {
			buckets[key] = append(buckets[key], e)
		}
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

// fieldExtractor returns the function that pulls the group key(s) off an
// entry for the named field. Returning a closure avoids the per-entry
// switch on field-name inside groupBy's hot loop. Most fields are
// single-valued (one key); participant is multi-valued — it returns the
// entry's full participant list so co-authored entries bucket under each.
func fieldExtractor(field string) (func(*model.Entry) []string, bool) {
	switch field {
	case "kind":
		return func(e *model.Entry) []string { return []string{string(e.Kind)} }, true
	case "layer":
		return func(e *model.Entry) []string { return []string{string(e.Layer)} }, true
	case "type":
		return func(e *model.Entry) []string { return []string{string(e.Type)} }, true
	case "participant":
		return func(e *model.Entry) []string { return e.Participants }, true
	default:
		return nil, false
	}
}
