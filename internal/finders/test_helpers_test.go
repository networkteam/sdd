package finders

import (
	"fmt"

	"github.com/networkteam/sdd/internal/model"
)

// entry is a test helper that builds a model.Entry from an ID string.
// It parses the type, layer, and time from the ID using model.ParseID.
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

func withAttachments(paths ...string) entryOpt {
	return func(e *model.Entry) { e.Attachments = paths }
}

// withFocusActors sets the focus-level default actor list (kind: focus only).
func withFocusActors(actors ...string) entryOpt {
	return func(e *model.Entry) { e.FocusActors = actors }
}

// withInvolvement appends one involvement triple with optional explicit
// actors. ActorsExplicit must be true when callers want to test the
// "actors: []" pull-available case versus the unset "inherit default" case.
func withInvolvement(target string, actors []string, actorsExplicit bool) entryOpt {
	return func(e *model.Entry) {
		e.Involvement = append(e.Involvement, model.Involvement{
			Target:    target,
			Actors:    actors,
			ActorsSet: actorsExplicit,
		})
	}
}
