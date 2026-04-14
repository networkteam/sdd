package presenters_test

import (
	"fmt"

	"github.com/networkteam/resonance/framework/sdd/model"
)

// entry is a test helper that builds a model.Entry from an ID string.
func entry(id string, opts ...entryOpt) *model.Entry {
	parts, err := model.ParseID(id)
	if err != nil {
		panic(fmt.Sprintf("bad test ID %q: %v", id, err))
	}

	e := &model.Entry{
		ID:      id,
		Type:    model.TypeFromAbbrev[parts.TypeCode],
		Layer:   model.LayerFromAbbrev[parts.LayerCode],
		Content: id,
		Time:    parts.Time,
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

type entryOpt func(*model.Entry)

func withRefs(refs ...string) entryOpt {
	return func(e *model.Entry) { e.Refs = refs }
}

func withSupersedes(ids ...string) entryOpt {
	return func(e *model.Entry) { e.Supersedes = ids }
}

func withCloses(ids ...string) entryOpt {
	return func(e *model.Entry) { e.Closes = ids }
}

func withKind(k model.Kind) entryOpt {
	return func(e *model.Entry) { e.Kind = k }
}

func withContent(c string) entryOpt {
	return func(e *model.Entry) { e.Content = c }
}

func withSummary(s string) entryOpt {
	return func(e *model.Entry) { e.Summary = s }
}

func withAttachments(a ...string) entryOpt {
	return func(e *model.Entry) { e.Attachments = a }
}
