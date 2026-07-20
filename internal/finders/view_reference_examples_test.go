package finders

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/viewlayout"
)

func TestViewReferenceExamplesExecute(t *testing.T) {
	graph := model.NewGraph(nil)
	for _, example := range viewlayout.ExampleSpecs() {
		t.Run(example, func(t *testing.T) {
			layout, err := query.ParseLayout(example)
			if err != nil {
				t.Fatal(err)
			}
			layout, err = query.ExpandMacros(layout)
			if err != nil {
				t.Fatal(err)
			}
			_, err = New(Options{}).View(query.ViewQuery{Graph: graph, Layout: layout, WIPMarkers: []*model.WIPMarker{}})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMarkdownReferenceCoversLiveViewVocabulary(t *testing.T) {
	vocabulary := LiveViewVocabulary()
	markdown := viewlayout.Markdown(vocabulary)
	for _, names := range [][]string{vocabulary.Functions, vocabulary.Algorithms, vocabulary.Decays, vocabulary.Macros} {
		for _, name := range names {
			if !strings.Contains(markdown, name) {
				t.Errorf("Markdown reference omits live name %q", name)
			}
		}
	}
}
