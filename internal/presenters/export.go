package presenters

import (
	"encoding/json"
	"io"
	"time"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// wallClockLayout renders the timestamp an entry or WIP ID carries: the
// capturer's local wall clock, which records no zone, so none is claimed.
const wallClockLayout = "2006-01-02T15:04:05"

type exportJSON struct {
	Experimental bool             `json:"experimental"`
	GeneratedAt  string           `json:"generated_at"`
	Repos        []exportRepoJSON `json:"repos"`
	// Embedded lists the base entries once; an ID from any repo that names
	// one resolves here (query.ExportResult).
	Embedded []exportEntryJSON `json:"embedded"`
}

type exportRepoJSON struct {
	RepoID      string            `json:"repo_id"`
	Local       bool              `json:"local"`
	Unavailable bool              `json:"unavailable,omitempty"`
	Revision    string            `json:"revision,omitempty"`
	Selection   *selectionJSON    `json:"selection,omitempty"`
	Entries     []exportEntryJSON `json:"entries"`
	WIP         []exportWIPJSON   `json:"wip"`
	LoadIssues  []loadIssueJSON   `json:"load_issues,omitempty"`
}

type exportEntryJSON struct {
	ID               string                 `json:"id"`
	FullID           string                 `json:"full_id,omitempty"`
	ShortID          string                 `json:"short_id,omitempty"`
	Type             model.EntryType        `json:"type"`
	Kind             model.Kind             `json:"kind,omitempty"`
	Layer            model.Layer            `json:"layer"`
	Confidence       string                 `json:"confidence,omitempty"`
	Intent           model.Intent           `json:"intent,omitempty"`
	Participants     []string               `json:"participants,omitempty"`
	Topics           []string               `json:"topics,omitempty"`
	Time             string                 `json:"time"`
	Status           model.StatusKind       `json:"status,omitempty"`
	StatusBy         string                 `json:"status_by,omitempty"`
	ClosedBy         []string               `json:"closed_by,omitempty"`
	SupersededBy     []string               `json:"superseded_by,omitempty"`
	Closes           []string               `json:"closes,omitempty"`
	Supersedes       []string               `json:"supersedes,omitempty"`
	Refs             []refJSON              `json:"refs,omitempty"`
	Heat             float64                `json:"heat"`
	InDegree         int                    `json:"in_degree"`
	Summary          string                 `json:"summary,omitempty"`
	Body             string                 `json:"body,omitempty"`
	Canonical        string                 `json:"canonical,omitempty"`
	Aliases          []string               `json:"aliases,omitempty"`
	Class            model.ProcedureClass   `json:"class,omitempty"`
	Actor            string                 `json:"actor,omitempty"`
	Override         string                 `json:"override,omitempty"`
	Index            *factIndexJSON         `json:"index,omitempty"`
	AnnotationTopics []annotationTopicJSON  `json:"annotation_topics,omitempty"`
	FocusActors      []string               `json:"focus_actors,omitempty"`
	FocusWhen        *focusWhenJSON         `json:"focus_when,omitempty"`
	Involvement      []involvementJSON      `json:"involvement,omitempty"`
	Preflight        string                 `json:"preflight,omitempty"`
	Warnings         []warningJSON          `json:"warnings,omitempty"`
	Attachments      []exportAttachmentJSON `json:"attachments,omitempty"`
}

type selectionJSON struct {
	Mode         query.ExportDependencies `json:"mode"`
	Hops         *int                     `json:"hops,omitempty"`
	TotalEntries int                      `json:"total_entries"`
}

type refJSON struct {
	ID   string        `json:"id"`
	Kind model.RefKind `json:"kind"`
	Desc string        `json:"desc,omitempty"`
}

type factIndexJSON struct {
	Title string `json:"title"`
	Topic string `json:"topic,omitempty"`
}

type annotationTopicJSON struct {
	Label   string   `json:"label"`
	Members []string `json:"members,omitempty"`
}

// involvementJSON carries the effective actors and scope, focus-level
// defaults already applied.
type involvementJSON struct {
	Target string         `json:"target"`
	Actors []string       `json:"actors"`
	When   *focusWhenJSON `json:"when,omitempty"`
}

type focusWhenJSON struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type warningJSON struct {
	Field   string `json:"field"`
	Value   string `json:"value,omitempty"`
	Message string `json:"message"`
}

type exportAttachmentJSON struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	Content     string `json:"content,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
}

type exportWIPJSON struct {
	ID          string `json:"id"`
	Entry       string `json:"entry"`
	Participant string `json:"participant"`
	Exclusive   bool   `json:"exclusive,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Description string `json:"description,omitempty"`
	Time        string `json:"time"`
}

type loadIssueJSON struct {
	Ref     string `json:"ref"`
	Message string `json:"message"`
}

// RenderExportJSON writes the experimental whole-graph export document.
func RenderExportJSON(w io.Writer, r *query.ExportResult) error {
	out := exportJSON{
		Experimental: true, GeneratedAt: r.GeneratedAt.Format(time.RFC3339),
		Repos: []exportRepoJSON{}, Embedded: []exportEntryJSON{},
	}
	for _, repo := range r.Repos {
		rj := exportRepoJSON{
			RepoID: repo.RepoID, Local: repo.Local, Unavailable: repo.Unavailable, Revision: repo.Revision,
			Entries: []exportEntryJSON{}, WIP: []exportWIPJSON{},
		}
		if sel := repo.Selection; sel != nil {
			rj.Selection = &selectionJSON{Mode: sel.Mode, TotalEntries: sel.TotalEntries}
			if sel.Mode == query.ExportDependenciesReferenced {
				rj.Selection.Hops = &sel.Hops
			}
		}
		for _, e := range repo.Entries {
			rj.Entries = append(rj.Entries, exportEntryJSONFrom(e))
		}
		for _, m := range repo.WIP {
			rj.WIP = append(rj.WIP, exportWIPJSON{
				ID: m.ID, Entry: m.Entry, Participant: m.Participant, Exclusive: m.Exclusive,
				Branch: m.Branch, Description: m.Content, Time: m.Time.Format(wallClockLayout),
			})
		}
		for _, issue := range repo.LoadIssues {
			rj.LoadIssues = append(rj.LoadIssues, loadIssueJSON(issue))
		}
		out.Repos = append(out.Repos, rj)
	}
	for _, e := range r.Embedded {
		out.Embedded = append(out.Embedded, exportEntryJSONFrom(e))
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func exportEntryJSONFrom(x query.ExportEntry) exportEntryJSON {
	e := x.Entry
	ej := exportEntryJSON{
		ID: e.ID, FullID: x.FullID, Type: e.Type, Kind: e.Kind, Layer: e.Layer,
		Confidence: e.Confidence, Intent: e.Intent, Participants: e.Participants,
		Time: e.Time.Format(wallClockLayout), Status: x.Status.Kind, StatusBy: x.Status.By,
		ClosedBy: x.ClosedBy, SupersededBy: x.SupersededBy, Closes: e.Closes, Supersedes: e.Supersedes,
		Heat: x.Heat, InDegree: x.InDegree, Summary: e.Summary, Body: e.Content,
		Canonical: e.Canonical, Aliases: e.Aliases, Class: e.Class, Actor: e.Actor, Override: e.Override,
		FocusActors: e.FocusActors, FocusWhen: focusWhenJSONFrom(e.FocusWhen), Preflight: e.Preflight,
	}
	if p, err := model.ParseID(e.ID); err == nil {
		ej.ShortID = p.TypeCode + "-" + p.LayerCode + "-" + p.Suffix
	}
	for _, t := range x.Topics {
		ej.Topics = append(ej.Topics, t.String())
	}
	for _, ref := range e.Refs {
		ej.Refs = append(ej.Refs, refJSON(ref))
	}
	if e.Index != nil {
		ej.Index = &factIndexJSON{Title: e.Index.Title}
		if len(e.Index.Topic.Components) > 0 {
			ej.Index.Topic = e.Index.Topic.String()
		}
	}
	for _, t := range e.AnnotationTopics {
		ej.AnnotationTopics = append(ej.AnnotationTopics, annotationTopicJSON(t))
	}
	for _, inv := range e.Involvement {
		actors := e.ResolveActors(inv)
		if actors == nil {
			actors = []string{}
		}
		ej.Involvement = append(ej.Involvement, involvementJSON{Target: inv.Target, Actors: actors, When: focusWhenJSONFrom(e.ResolveWhen(inv))})
	}
	for _, w := range e.Warnings {
		ej.Warnings = append(ej.Warnings, warningJSON(w))
	}
	for _, a := range x.Attachments {
		ej.Attachments = append(ej.Attachments, exportAttachmentJSON(a))
	}
	return ej
}

func focusWhenJSONFrom(w *model.FocusWhen) *focusWhenJSON {
	if w == nil {
		return nil
	}
	return &focusWhenJSON{From: w.From, To: w.To}
}
