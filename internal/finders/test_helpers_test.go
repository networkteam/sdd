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
	return func(e *model.Entry) { e.Refs = refsOf(refs...) }
}

// withRefObjs sets the entry's refs to the given Ref literals, for tests that
// need specific kinds or desc values (expand(refs) rendering, ref-kind verbs).
func withRefObjs(refs ...model.Ref) entryOpt {
	return func(e *model.Entry) { e.Refs = refs }
}

// withCanonical sets the actor canonical (kind: actor only).
func withCanonical(c string) entryOpt {
	return func(e *model.Entry) { e.Canonical = c }
}

// withActor sets the role's bound actor canonical (kind: role only).
func withActor(c string) entryOpt {
	return func(e *model.Entry) { e.Actor = c }
}

// refsOf builds a []model.Ref from bare IDs, tagging each with
// RefKindRelated so tests construct entries that satisfy the new-capture
// contract. Tests asserting on specific kinds use Ref literals.
func refsOf(ids ...string) []model.Ref {
	out := make([]model.Ref, len(ids))
	for i, id := range ids {
		out[i] = model.Ref{ID: id, Kind: model.RefKindRelated}
	}
	return out
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

func withIntent(in model.Intent) entryOpt {
	return func(e *model.Entry) { e.Intent = in }
}

// withParticipants sets the entry's canonical participants list.
func withParticipants(names ...string) entryOpt {
	return func(e *model.Entry) { e.Participants = names }
}

func withContent(c string) entryOpt {
	return func(e *model.Entry) { e.Content = c }
}

// withTopics sets inline topic labels (any non-annotation kind). Labels are
// parsed into TopicPaths so tests fail loudly on malformed fixtures.
func withTopics(labels ...string) entryOpt {
	return func(e *model.Entry) {
		for _, l := range labels {
			p, err := model.ParseTopicPath(l)
			if err != nil {
				panic(fmt.Sprintf("bad test topic %q: %v", l, err))
			}
			e.Topics = append(e.Topics, p)
		}
	}
}

// withAnnotationTopics sets an annotation's own topic assignments (kind:
// annotation only). Each label applies to all of the annotation's refs.
func withAnnotationTopics(labels ...string) entryOpt {
	return func(e *model.Entry) {
		for _, l := range labels {
			e.AnnotationTopics = append(e.AnnotationTopics, model.AnnotationTopic{Label: l})
		}
	}
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
