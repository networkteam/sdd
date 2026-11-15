package handlers

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// SearchIndexOpener opens a read view of the search index for one command. A
// nil index with a nil error means no index is built.
type SearchIndexOpener interface {
	OpenSearchIndex(ctx context.Context) (*index.Index, error)
}

// SearchIndexDir opens the search index stored in the directory it names.
type SearchIndexDir string

func (d SearchIndexDir) OpenSearchIndex(context.Context) (*index.Index, error) {
	_, err := os.Stat(string(d))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the search index: %w", err)
	}
	store, err := index.Open(string(d))
	if err != nil {
		return nil, fmt.Errorf("opening the search index: %w", err)
	}
	return store, nil
}

// openTitleSearch opens the vector search a title call's similar titles come
// from. It returns a nil finder without an embedder or a built index.
func (h *Handler) openTitleSearch(ctx context.Context, graph *model.Graph) (*finders.SearchFinder, error) {
	if h.embedder == nil || h.index == nil {
		return nil, nil
	}
	store, err := h.index.OpenSearchIndex(ctx)
	if err != nil || store == nil {
		return nil, err
	}
	return finders.NewSearchFinder(finders.SearchFinderOptions{
		Graph:      graph,
		GraphDir:   h.graphDir,
		Embedder:   h.embedder,
		IndexStore: store,
	}), nil
}

// similarTitles returns the titles of the titled entries nearest entry's body,
// none from a nil finder.
func similarTitles(ctx context.Context, finder *finders.SearchFinder, entry *model.Entry) ([]string, error) {
	if finder == nil {
		return nil, nil
	}
	q, ok := query.SimilarTitles(entry)
	if !ok {
		return nil, nil
	}
	result, err := finder.Search(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("searching titles similar to %s: %w", entry.ID, err)
	}
	return result.Titles(), nil
}
