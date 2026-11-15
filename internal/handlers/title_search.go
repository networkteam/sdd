package handlers

import (
	"context"
	"fmt"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/pkg/llm/embed"
)

// TitleSearchOpener opens the embedder and the search index a title call's
// similar titles are searched with. Only a title call opens it, so a command
// making none never reads the embedding config. A nil embedder or index means
// none is configured or built.
type TitleSearchOpener interface {
	OpenTitleSearch(ctx context.Context) (embed.Embedder, *index.Index, error)
}

// TitleSearchFunc adapts a function to TitleSearchOpener.
type TitleSearchFunc func(ctx context.Context) (embed.Embedder, *index.Index, error)

func (f TitleSearchFunc) OpenTitleSearch(ctx context.Context) (embed.Embedder, *index.Index, error) {
	return f(ctx)
}

// openTitleSearch opens the vector search a title call's similar titles come
// from. It returns a nil finder without an embedder or a built index.
func (h *Handler) openTitleSearch(ctx context.Context, graph *model.Graph) (*finders.SearchFinder, error) {
	if h.titleSearch == nil {
		return nil, nil
	}
	embedder, store, err := h.titleSearch.OpenTitleSearch(ctx)
	if err != nil || embedder == nil || store == nil {
		return nil, err
	}
	return finders.NewSearchFinder(finders.SearchFinderOptions{
		Graph:      graph,
		GraphDir:   h.graphDir,
		Embedder:   embedder,
		IndexStore: store,
	}), nil
}

// embedSimilarTitles embeds the search for the titled entries nearest entry's
// body: nil from a nil finder or for an entry without a body. It reads no
// graph and only entry fields no summarize worker writes, so it runs outside
// the graph lock.
func embedSimilarTitles(ctx context.Context, finder *finders.SearchFinder, entry *model.Entry) (*query.SearchQuery, error) {
	if finder == nil {
		return nil, nil
	}
	q, ok := query.SimilarTitles(entry)
	if !ok {
		return nil, nil
	}
	q, err := finder.EmbedPhrase(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("searching titles similar to %s: %w", entry.ID, err)
	}
	return &q, nil
}

// similarTitles runs an embedded search against the finder's graph, returning
// the titles it finds; none for a nil search.
func similarTitles(ctx context.Context, finder *finders.SearchFinder, id string, q *query.SearchQuery) ([]string, error) {
	if q == nil {
		return nil, nil
	}
	result, err := finder.Search(ctx, *q)
	if err != nil {
		return nil, fmt.Errorf("searching titles similar to %s: %w", id, err)
	}
	return result.Titles(), nil
}
