package sdd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
)

// Application resolves current access and dispatches protocol-neutral SDD
// operations. Every method resolves identity and project afresh.
type Application struct {
	access AccessResolver
}

func NewApplication(access AccessResolver) (*Application, error) {
	if access == nil {
		return nil, fmt.Errorf("sdd: AccessResolver is required")
	}
	return &Application{access: access}, nil
}

func (a *Application) Info(ctx context.Context, identity RequestIdentity, project ProjectID, _ InfoRequest) (InfoResult, error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return InfoResult{}, err
	}
	search := "text"
	if runtime.options.Embeddings != nil && runtime.options.SearchIndex != nil {
		search = "vector,text"
	}
	return InfoResult{Project: runtime.options.Project, Participant: runtime.options.Participant, Language: runtime.options.Language, Search: search}, nil
}

func (a *Application) View(ctx context.Context, identity RequestIdentity, project ProjectID, request ViewRequest) (ViewResult, error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return ViewResult{}, err
	}
	snapshot, err := runtime.options.Graph.Current(ctx)
	if err != nil {
		return ViewResult{}, err
	}
	layout, err := query.ParseLayout(request.Layout)
	if err != nil {
		return ViewResult{}, err
	}
	layout, err = query.ExpandMacros(layout)
	if err != nil {
		return ViewResult{}, err
	}
	result, err := finders.New(finders.Options{}).View(query.ViewQuery{Graph: snapshot.graph, Layout: layout, WIPMarkers: snapshot.wip})
	if err != nil {
		return ViewResult{}, err
	}
	var rendered bytes.Buffer
	presenters.RenderView(&rendered, result)
	repos, err := a.selectedDependencies(request.Repos, request.AllRepos, runtime.options.Dependencies)
	if err != nil {
		return ViewResult{}, err
	}
	for _, repoID := range repos {
		dependency, err := a.dependency(ctx, identity, runtime, repoID)
		if err != nil {
			return ViewResult{}, err
		}
		member, err := dependency.options.Graph.Current(ctx)
		if err != nil {
			return ViewResult{}, dependencyUnavailable()
		}
		memberResult, err := finders.New(finders.Options{}).View(query.ViewQuery{Graph: member.graph, Layout: layout, WIPMarkers: member.wip})
		if err != nil {
			return ViewResult{}, err
		}
		fmt.Fprintf(&rendered, "\n── repo: %s ──\n", repoID)
		presenters.RenderView(&rendered, memberResult)
	}
	return ViewResult{Project: runtime.options.Project, Sections: strings.TrimRight(rendered.String(), "\n")}, nil
}

func (a *Application) Show(ctx context.Context, identity RequestIdentity, project ProjectID, request ShowRequest) (ShowResult, error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return ShowResult{}, err
	}
	if len(request.IDs) == 0 {
		return ShowResult{}, fmt.Errorf("sdd: at least one entry ID is required")
	}
	snapshot, err := a.snapshotWithDependencies(ctx, identity, runtime)
	if err != nil {
		return ShowResult{}, err
	}
	up, down := request.UpDepth, request.DownDepth
	if up < 0 || down < 0 {
		return ShowResult{}, fmt.Errorf("sdd: show depths cannot be negative")
	}
	result, err := finders.New(finders.Options{}).Show(query.ShowQuery{Graph: snapshot.graph, IDs: request.IDs, UpDepth: up, DownDepth: down})
	if err != nil {
		return ShowResult{}, err
	}
	var rendered bytes.Buffer
	presenters.RenderShow(&rendered, result, presenters.ShowOptions{})
	return ShowResult{Project: runtime.options.Project, Entries: strings.TrimRight(rendered.String(), "\n")}, nil
}

func (a *Application) Search(ctx context.Context, identity RequestIdentity, project ProjectID, request SearchRequest) (SearchResult, error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return SearchResult{}, err
	}
	snapshot, err := runtime.options.Graph.Current(ctx)
	if err != nil {
		return SearchResult{}, err
	}
	filter, err := publicGraphFilter(request)
	if err != nil {
		return SearchResult{}, err
	}
	q := query.SearchQuery{
		Graph: snapshot.graph, Terms: request.Terms, Phrase: request.Phrase, Filter: filter,
		IncludeSuperseded: request.IncludeSuperseded, Limit: request.Limit, MaxCitationsPerEntry: request.MaxCitations,
	}
	result, err := runtime.searchSnapshot(ctx, snapshot, q)
	if err != nil {
		return SearchResult{}, err
	}
	repos, err := a.selectedDependencies(request.Repos, request.AllRepos, runtime.options.Dependencies)
	if err != nil {
		return SearchResult{}, err
	}
	for _, repoID := range repos {
		dependency, err := a.dependency(ctx, identity, runtime, repoID)
		if err != nil {
			return SearchResult{}, err
		}
		member, err := dependency.options.Graph.Current(ctx)
		if err != nil {
			return SearchResult{}, dependencyUnavailable()
		}
		memberQuery := q
		memberQuery.Graph = member.graph
		memberResult, err := dependency.searchSnapshot(ctx, member, memberQuery)
		if err != nil {
			return SearchResult{}, err
		}
		for i := range memberResult.Entries {
			memberResult.Entries[i].RepoID = repoID
		}
		result.Entries = append(result.Entries, memberResult.Entries...)
	}
	sort.SliceStable(result.Entries, func(i, j int) bool { return result.Entries[i].Score > result.Entries[j].Score })
	if limit := q.EffectiveLimit(); len(result.Entries) > limit {
		result.Entries = result.Entries[:limit]
	}
	var rendered bytes.Buffer
	presenters.RenderSearch(&rendered, result, snapshot.graph)
	return SearchResult{Project: runtime.options.Project, Results: strings.TrimRight(rendered.String(), "\n")}, nil
}

func (a *Application) ReadAttachment(ctx context.Context, identity RequestIdentity, project ProjectID, request ReadAttachmentRequest) (ReadAttachmentResult, error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return ReadAttachmentResult{}, err
	}
	entryID := request.EntryID
	store := runtime.options.Graph
	if repoID, memberID, qualified := model.SplitCrossRepoID(request.EntryID); qualified {
		dependency, depErr := a.dependency(ctx, identity, runtime, repoID)
		if depErr != nil {
			return ReadAttachmentResult{}, depErr
		}
		store = dependency.options.Graph
		entryID = memberID
	}
	page, err := store.ReadAttachmentPage(ctx, entryID, request.Filename, request.Offset, request.MaxBytes)
	if err != nil {
		return ReadAttachmentResult{}, err
	}
	return ReadAttachmentResult{Project: runtime.options.Project, Page: page}, nil
}

func (a *Application) Procedures(ctx context.Context, identity RequestIdentity, project ProjectID, _ ProcedureListRequest) (ProcedureListResult, error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return ProcedureListResult{}, err
	}
	snapshot, err := runtime.options.Graph.Current(ctx)
	if err != nil {
		return ProcedureListResult{}, err
	}
	var lines []string
	for _, chain := range snapshot.graph.ProcedureChains() {
		head := chain.Head
		if head == nil || head.Canonical == "" || head.IsShellProcedure() || head.IsTaskProcedure() || len(chain.LiveHeads) == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s%s — %s", head.Canonical, publicProcedureSignature(head), head.FirstSummarySentence()))
	}
	sort.Strings(lines)
	return ProcedureListResult{Project: runtime.options.Project, Procedures: strings.Join(lines, "\n")}, nil
}

func (a *Application) resolve(ctx context.Context, identity RequestIdentity, project ProjectID, required Access) (Principal, *ProjectRuntime, error) {
	principal, err := a.access.ResolvePrincipal(ctx, identity)
	if err != nil {
		return Principal{}, nil, err
	}
	if principal.Subject == "" || principal.Subject != identity.Subject {
		return Principal{}, nil, &ApplicationError{Code: ErrorAuthenticationRequired, Message: "resolved principal does not match current request"}
	}
	if project == "" {
		projects, err := a.access.ListProjects(ctx, principal)
		if err != nil {
			return Principal{}, nil, err
		}
		for _, candidate := range projects.Projects {
			allowed := candidate.CanRead
			if required == AccessWrite {
				allowed = candidate.CanWrite
			}
			if candidate.State == ProjectReady && allowed {
				if project != "" {
					return Principal{}, nil, &ApplicationError{Code: ErrorProjectRequired, Message: "more than one accessible project; select one explicitly"}
				}
				project = candidate.ID
			}
		}
		if project == "" {
			return Principal{}, nil, &ApplicationError{Code: ErrorProjectRequired, Message: "no accessible project could be inferred"}
		}
	}
	runtime, err := a.access.ResolveProject(ctx, principal, project, required)
	if err != nil {
		return Principal{}, nil, err
	}
	if runtime == nil || runtime.options.Project.ID != project {
		return Principal{}, nil, &ApplicationError{Code: ErrorProjectUnavailable, Message: "project resolver returned no matching runtime"}
	}
	return principal, runtime, nil
}

func (r *ProjectRuntime) searchSnapshot(ctx context.Context, snapshot *Snapshot, request query.SearchQuery) (*query.SearchResult, error) {
	if request.Phrase == "" {
		return finders.NewSearchFinder(finders.SearchFinderOptions{}).Search(ctx, request)
	}
	return r.vectorSearch(ctx, snapshot, request)
}

func (a *Application) snapshotWithDependencies(ctx context.Context, identity RequestIdentity, runtime *ProjectRuntime) (*Snapshot, error) {
	base, err := runtime.options.Graph.Current(ctx)
	if err != nil {
		return nil, err
	}
	if len(runtime.options.Dependencies) == 0 {
		return base, nil
	}
	principal, err := a.access.ResolvePrincipal(ctx, identity)
	if err != nil {
		return nil, err
	}
	local := model.NewGraph(append([]*model.Entry(nil), base.graph.Entries...))
	model.NewMultiGraph(local, append([]string(nil), runtime.options.Dependencies...), func(repoID string) (*model.Graph, error) {
		if !slices.Contains(runtime.options.Dependencies, repoID) {
			return nil, dependencyUnavailable()
		}
		dependency, err := a.access.ResolveDependency(ctx, principal, runtime.options.Project.ID, repoID)
		if err != nil || dependency == nil {
			return nil, dependencyUnavailable()
		}
		snapshot, err := dependency.options.Graph.Current(ctx)
		if err != nil {
			return nil, dependencyUnavailable()
		}
		return model.NewGraph(append([]*model.Entry(nil), snapshot.graph.Entries...)), nil
	})
	clone := *base
	clone.graph = local
	return &clone, nil
}

func (a *Application) dependency(ctx context.Context, identity RequestIdentity, runtime *ProjectRuntime, repoID string) (*ProjectRuntime, error) {
	if !slices.Contains(runtime.options.Dependencies, repoID) {
		return nil, dependencyUnavailable()
	}
	principal, err := a.access.ResolvePrincipal(ctx, identity)
	if err != nil {
		return nil, err
	}
	dependency, err := a.access.ResolveDependency(ctx, principal, runtime.options.Project.ID, repoID)
	if err != nil || dependency == nil {
		return nil, dependencyUnavailable()
	}
	return dependency, nil
}

func (a *Application) selectedDependencies(named []string, all bool, declared []string) ([]string, error) {
	if all {
		return append([]string(nil), declared...), nil
	}
	seen := map[string]bool{}
	selected := make([]string, 0, len(named))
	for _, repoID := range named {
		if !slices.Contains(declared, repoID) {
			return nil, dependencyUnavailable()
		}
		if !seen[repoID] {
			seen[repoID] = true
			selected = append(selected, repoID)
		}
	}
	return selected, nil
}

func dependencyUnavailable() error {
	return &ApplicationError{Code: ErrorProjectUnavailable, Message: "dependency unavailable"}
}

func publicGraphFilter(request SearchRequest) (model.GraphFilter, error) {
	filter := model.GraphFilter{}
	if request.Type != "" {
		filter.Type = model.EntryType(request.Type)
	}
	if request.Layer != "" {
		layer := model.Layer(request.Layer)
		if expanded, ok := model.LayerFromAbbrev[request.Layer]; ok {
			layer = expanded
		}
		filter.Layer = layer
	}
	if request.Kind != "" {
		filter.Kind = model.Kind(request.Kind)
	}
	return filter, nil
}

func publicProcedureSignature(head *model.Entry) string {
	spec, err := engine.ParseSpec(head)
	if err != nil || len(spec.Params) == 0 {
		return ""
	}
	var names []string
	for name := range spec.Params {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		decl := spec.Params[name]
		optional := ""
		if decl.Optional {
			optional = "?"
		}
		parts = append(parts, fmt.Sprintf("%s%s: %s", name, optional, decl.Type))
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

var errVectorUnavailable = errors.New("vector search requires EmbeddingExecutor and SearchIndexStore")
