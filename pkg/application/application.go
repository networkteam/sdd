package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/networkteam/sdd/internal/bundledskills"
	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/pkg/application/types"
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
	principal, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return InfoResult{}, err
	}
	search := "text"
	if runtime.options.Embeddings != nil && runtime.options.SearchIndex != nil {
		search = "vector,text"
	}
	recoveries, err := listRecoveriesRuntime(ctx, runtime, false)
	if err != nil {
		return InfoResult{}, err
	}
	return InfoResult{
		Project: runtime.options.Project, Participant: principal.Participant, Language: runtime.options.Language,
		Search: search, Recovery: renderRecoveryNotices(recoveries.Items),
	}, nil
}

// Projects lists the projects the request's principal can reach, with
// per-project access. It resolves no project: it is the read a shell needs
// before a project is chosen (d-tac-1z6).
func (a *Application) Projects(ctx context.Context, identity RequestIdentity) (ProjectList, error) {
	principal, err := a.resolvePrincipal(ctx, identity)
	if err != nil {
		return ProjectList{}, err
	}
	return a.access.ListProjects(ctx, principal)
}

func (a *Application) Vocabulary(ctx context.Context, identity RequestIdentity, project ProjectID) (string, error) {
	info, err := a.Info(ctx, identity, project, InfoRequest{})
	if err != nil {
		return "", err
	}
	if info.Language == "" {
		return "", nil
	}
	locale := strings.ToLower(info.Language)
	base := locale
	if index := strings.IndexAny(base, "-_"); index >= 0 {
		base = base[:index]
	}
	if base == "en" {
		return "", nil
	}
	for _, candidate := range []string{locale, base} {
		body, readErr := bundledskills.ReadReference("sdd", "references/vocabulary-"+candidate+".md")
		if readErr == nil {
			return strings.TrimSpace(string(body)), nil
		}
		if candidate == base {
			break
		}
	}
	return fmt.Sprintf("(configured graph language %q has no bundled vocabulary reference — render user-facing terms in English canonical form; adding references/vocabulary-%s.md is a framework-level contribution)", info.Language, base), nil
}

// Lint runs the categorized lint providers over the project's graph — the
// composition-root lint query (d-cpt-xc3): shells render the findings by
// category and derive their exit state from LintResult.Errors alone. Index
// findings need shell-resolved inputs (IndexLintQuery) and stay a shell
// concern.
func (a *Application) Lint(ctx context.Context, identity RequestIdentity, project ProjectID, request types.LintQuery) (result *types.LintResult, err error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return nil, err
	}
	selected, err := acquireSnapshotForReadBranch(ctx, runtime, "")
	if err != nil {
		return nil, err
	}
	defer selected.releaseInto(&err)
	registry, err := ProcedureRegistry()
	if err != nil {
		return nil, err
	}
	finder := finders.New(finders.Options{
		Config:            &model.PerRepoConfig{Dependencies: runtime.options.Dependencies},
		ProcedureRegistry: registry,
	})
	return finder.OnGraph(selected.snapshot.graph).Lint(request)
}

func (a *Application) View(ctx context.Context, identity RequestIdentity, project ProjectID, request ViewRequest) (result ViewResult, err error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return ViewResult{}, err
	}
	selected, err := acquireSnapshotForReadBranch(ctx, runtime, request.Branch)
	if err != nil {
		return ViewResult{}, withSessionBindingTargetError(request.Branch, request.BranchFromSession, err)
	}
	defer selected.releaseInto(&err)
	snapshot := selected.snapshot
	layout, err := query.ParseLayout(request.Layout)
	if err != nil {
		return ViewResult{}, err
	}
	layout, err = query.ExpandMacros(layout)
	if err != nil {
		return ViewResult{}, err
	}
	viewResult, err := snapshot.finder.View(query.ViewQuery{Layout: layout, Budget: request.Budget})
	if err != nil {
		return ViewResult{}, err
	}
	var rendered bytes.Buffer
	presenters.RenderView(&rendered, viewResult)
	// Matched count is a read-side fact owned by the finder; sum it across the
	// local graph and every queried dependency so "empty" means nothing
	// rendered anywhere, not just an empty rendered string (which is also false
	// once a repo header or recovery notice prints).
	matched := viewResult.MatchedCount()
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
		memberResult, err := member.finder.View(query.ViewQuery{Layout: layout})
		if err != nil {
			return ViewResult{}, err
		}
		matched += memberResult.MatchedCount()
		fmt.Fprintf(&rendered, "\n── repo: %s ──\n", repoID)
		presenters.RenderView(&rendered, memberResult)
	}
	if !request.OmitRecovery {
		recoveries, err := listRecoveriesRuntime(ctx, runtime, false)
		if err != nil {
			return ViewResult{}, err
		}
		if notices := renderRecoveryNotices(recoveries.Items); notices != "" {
			fmt.Fprintf(&rendered, "\n%s\n", notices)
		}
	}
	// When a participant filter matched nothing, name the participants the
	// local graph knows: participant() is an exact canonical match, so an
	// empty result usually means a wrong spelling rather than genuinely no
	// work. Only the local graph's names are offered (the useful hint).
	var knownParticipants []string
	if matched == 0 && layout.UsesFunction("participant") {
		knownParticipants = snapshot.graph.AllParticipants()
	}
	return ViewResult{
		Project:           runtime.options.Project,
		Sections:          strings.TrimRight(rendered.String(), "\n"),
		MatchedCount:      matched,
		KnownParticipants: knownParticipants,
	}, nil
}

func (a *Application) Show(ctx context.Context, identity RequestIdentity, project ProjectID, request ShowRequest) (result ShowResult, err error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return ShowResult{}, err
	}
	if len(request.IDs) == 0 {
		return ShowResult{}, fmt.Errorf("sdd: at least one entry ID is required")
	}
	selected, err := acquireSnapshotForReadBranch(ctx, runtime, request.Branch)
	if err != nil {
		return ShowResult{}, withSessionBindingTargetError(request.Branch, request.BranchFromSession, err)
	}
	defer selected.releaseInto(&err)
	local := selected.snapshot
	snapshot, err := a.snapshotWithDependenciesFrom(ctx, identity, runtime, local)
	if err != nil {
		return ShowResult{}, err
	}
	up, down := request.UpDepth, request.DownDepth
	if up < 0 || down < 0 {
		return ShowResult{}, fmt.Errorf("sdd: show depths cannot be negative")
	}
	showResult, err := snapshot.finder.Show(query.ShowQuery{IDs: request.IDs, UpDepth: up, DownDepth: down, Budget: request.Budget})
	if err != nil {
		return ShowResult{}, err
	}
	var rendered bytes.Buffer
	presenters.RenderShow(&rendered, showResult, presenters.ShowOptions{})
	show := ShowResult{Project: runtime.options.Project, Entries: strings.TrimRight(rendered.String(), "\n")}
	for _, group := range showResult.Groups {
		if group.Primary != nil {
			if group.PrimaryID != "" {
				show.FullIDs = append(show.FullIDs, group.PrimaryID)
			} else {
				show.FullIDs = append(show.FullIDs, group.Primary.ID)
			}
		}
		for _, items := range [][]model.ShowTreeItem{group.Upstream, group.Downstream} {
			for _, item := range items {
				if item.Entry != nil {
					show.SummaryIDs = append(show.SummaryIDs, item.NodeID())
				}
			}
		}
	}
	return show, nil
}

func (a *Application) Search(ctx context.Context, identity RequestIdentity, project ProjectID, request SearchRequest) (result SearchResult, err error) {
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return SearchResult{}, err
	}
	selected, err := acquireSnapshotForReadBranch(ctx, runtime, request.Branch)
	if err != nil {
		return SearchResult{}, withSessionBindingTargetError(request.Branch, request.BranchFromSession, err)
	}
	defer selected.releaseInto(&err)
	snapshot := selected.snapshot
	filter, err := publicGraphFilter(request)
	if err != nil {
		return SearchResult{}, err
	}
	q := query.SearchQuery{
		Terms: request.Terms, Phrase: request.Phrase, Filter: filter,
		IncludeSuperseded: request.IncludeSuperseded, Limit: request.Limit, MaxCitationsPerEntry: request.MaxCitations,
	}
	searchResult, err := runtime.searchSnapshot(ctx, snapshot, selected.store, q)
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
		memberResult, err := dependency.searchSnapshot(ctx, member, dependency.options.Graph, q)
		if err != nil {
			return SearchResult{}, err
		}
		for i := range memberResult.Entries {
			memberResult.Entries[i].RepoID = repoID
		}
		searchResult.Entries = append(searchResult.Entries, memberResult.Entries...)
	}
	sort.SliceStable(searchResult.Entries, func(i, j int) bool { return searchResult.Entries[i].Score > searchResult.Entries[j].Score })
	if limit := q.EffectiveLimit(); len(searchResult.Entries) > limit {
		searchResult.Entries = searchResult.Entries[:limit]
	}
	var rendered bytes.Buffer
	presenters.RenderSearch(&rendered, searchResult, snapshot.graph)
	search := SearchResult{Project: runtime.options.Project, Results: strings.TrimRight(rendered.String(), "\n")}
	for _, entry := range searchResult.Entries {
		if entry.Entry != nil {
			search.EntryIDs = append(search.EntryIDs, entry.DisplayID())
		}
	}
	return search, nil
}

type readSnapshotSelection struct {
	snapshot *Snapshot
	store    GraphStore
	release  func() error
	branch   string
}

// acquireSnapshotForReadBranch selects only the local project's read
// authority. Empty means the runtime's current graph, not DefaultBranch:
// DefaultBranch is a write-routing fallback and may intentionally point at a
// different checkout. A concrete branch stays acquired until the caller has
// finished reading both the snapshot and its attachments.
func acquireSnapshotForReadBranch(ctx context.Context, runtime *ProjectRuntime, branch string) (*readSnapshotSelection, error) {
	if branch == "" {
		snapshot, err := runtime.options.Graph.Current(ctx)
		if err != nil {
			return nil, err
		}
		return &readSnapshotSelection{snapshot: snapshot, store: runtime.options.Graph}, nil
	}
	target, err := resolveMutationTarget(runtime, MutationTarget{Project: runtime.options.Project.ID, Branch: branch})
	if err != nil {
		return nil, err
	}
	acquired, err := runtime.acquire(ctx, target)
	if err != nil {
		return nil, err
	}
	snapshot, err := acquired.Graph.Current(ctx)
	if err != nil {
		releaseErr := acquired.Release()
		if releaseErr != nil {
			releaseErr = fmt.Errorf("releasing read target %s after snapshot failure: %w", branch, releaseErr)
		}
		return nil, errors.Join(err, releaseErr)
	}
	return &readSnapshotSelection{snapshot: snapshot, store: acquired.Graph, release: acquired.Release, branch: branch}, nil
}

func (s *readSnapshotSelection) releaseInto(errp *error) {
	if s == nil || s.release == nil {
		return
	}
	if releaseErr := s.release(); releaseErr != nil {
		*errp = errors.Join(*errp, fmt.Errorf("releasing read target %s: %w", s.branch, releaseErr))
	}
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
	snapshot, err := store.Current(ctx)
	if err != nil {
		return ReadAttachmentResult{}, err
	}
	entry, ok := snapshot.graph.ByID[entryID]
	if !ok {
		return ReadAttachmentResult{}, fmt.Errorf("entry not found: %s", entryID)
	}
	page, err := store.ReadAttachmentPage(ctx, entryID, request.Filename, request.Offset, request.MaxBytes)
	if err != nil {
		return ReadAttachmentResult{}, err
	}
	available := make([]string, 0, len(entry.Attachments))
	for _, attachment := range entry.Attachments {
		available = append(available, filepath.Base(attachment))
	}
	return ReadAttachmentResult{Project: runtime.options.Project, Page: page, Available: available}, nil
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
		signature, err := publicProcedureSignature(head)
		if err != nil {
			// A broken spec is listed as broken, never silently signature-less
			// — the author's only load test is this surface and the start.
			lines = append(lines, fmt.Sprintf("- %s — spec fails to load: %s", head.Canonical, oneLineError(err)))
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s%s — %s", head.Canonical, signature, head.FirstSummarySentence()))
	}
	sort.Strings(lines)
	return ProcedureListResult{Project: runtime.options.Project, Procedures: strings.Join(lines, "\n")}, nil
}

func (a *Application) resolvePrincipal(ctx context.Context, identity RequestIdentity) (Principal, error) {
	principal, err := a.access.ResolvePrincipal(ctx, identity)
	if err != nil {
		return Principal{}, err
	}
	if principal.Subject == "" || principal.Subject != identity.Subject {
		return Principal{}, &ApplicationError{Code: ErrorAuthenticationRequired, Message: "resolved principal does not match current request"}
	}
	return principal, nil
}

func (a *Application) resolve(ctx context.Context, identity RequestIdentity, project ProjectID, required Access) (Principal, *ProjectRuntime, error) {
	principal, err := a.resolvePrincipal(ctx, identity)
	if err != nil {
		return Principal{}, nil, err
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

func (r *ProjectRuntime) searchSnapshot(ctx context.Context, snapshot *Snapshot, attachments GraphStore, request query.SearchQuery) (*query.SearchResult, error) {
	if request.Phrase == "" {
		return finders.NewSearchFinder(finders.SearchFinderOptions{Graph: snapshot.graph}).Search(ctx, request)
	}
	return r.vectorSearch(ctx, snapshot, attachments, request)
}

func (a *Application) snapshotWithDependencies(ctx context.Context, identity RequestIdentity, runtime *ProjectRuntime) (*Snapshot, error) {
	base, err := runtime.options.Graph.Current(ctx)
	if err != nil {
		return nil, err
	}
	return a.snapshotWithDependenciesFrom(ctx, identity, runtime, base)
}

func (a *Application) snapshotWithDependenciesFrom(ctx context.Context, identity RequestIdentity, runtime *ProjectRuntime, base *Snapshot) (*Snapshot, error) {
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
	// The finder is the read authority, so rewrap it around the cross-graph
	// assembled graph — otherwise reads through clone.finder would see the
	// pre-assembly base graph. The base snapshot's WIP markers carry over.
	clone.finder = finders.New(finders.Options{}).OnGraph(local).WithWIP(base.wip)
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

// publicGraphFilter normalizes the request's type/layer/kind into a
// GraphFilter. The MCP tool schema documents these as abbreviations ("s"/"d",
// "tac", …) and agents pass them, so an unnormalized cast — matching the
// canonical full names only — makes every filtered search silently return
// nothing. Each field is normalized (abbrev → canonical) and validated;
// an unrecognized value fails loud rather than building a filter that matches
// no entry.
func publicGraphFilter(request SearchRequest) (model.GraphFilter, error) {
	filter := model.GraphFilter{}
	if request.Type != "" {
		t, ok := model.ParseTypeFilter(request.Type)
		if !ok {
			return model.GraphFilter{}, &ApplicationError{Code: ErrorInvalidArgument, Message: fmt.Sprintf("sdd: unknown type %q (want s, d, signal, or decision)", request.Type)}
		}
		filter.Type = t
	}
	if request.Layer != "" {
		l, ok := model.ParseLayerFilter(request.Layer)
		if !ok {
			return model.GraphFilter{}, &ApplicationError{Code: ErrorInvalidArgument, Message: fmt.Sprintf("sdd: unknown layer %q (want stg, cpt, tac, ops, prc, or the full name)", request.Layer)}
		}
		filter.Layer = l
	}
	if request.Kind != "" {
		k := model.Kind(request.Kind)
		if !model.IsKnownKind(k) {
			return model.GraphFilter{}, &ApplicationError{Code: ErrorInvalidArgument, Message: fmt.Sprintf("sdd: unknown kind %q", request.Kind)}
		}
		filter.Kind = k
	}
	return filter, nil
}

// oneLineError compacts a (possibly multi-line, joined) spec error into one
// served list line.
func oneLineError(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
}

func publicProcedureSignature(head *model.Entry) (string, error) {
	spec, err := engine.ParseSpec(head)
	if err != nil {
		return "", err
	}
	if len(spec.Params) == 0 {
		return "", nil
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
	return "(" + strings.Join(parts, ", ") + ")", nil
}

var errVectorUnavailable = errors.New("vector search requires EmbeddingExecutor and SearchIndexStore")
