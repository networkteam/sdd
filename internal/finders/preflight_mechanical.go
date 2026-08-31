package finders

import (
	"fmt"
	"strings"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/serveview"
)

// mechanicalPreflight runs Go-side structural checks against a proposed
// entry before the LLM-backed pre-flight. Findings use the same severity
// vocabulary as LLM findings so the handler can merge without conversion.
//
// Current checks (see plan d-cpt-d34):
//
//   - participant-coverage (AC 6): every name in Participants must match
//     the canonical of an active actor signal. Self-transitioning grace —
//     skipped when the graph has zero active actor signals.
//   - focus-actor-drift: on a kind: focus decision, every focus-level actor
//     and every per-involvement actor override must match an active actor
//     canonical — participant coverage applied to the focus actor fields,
//     sharing the same canonical set and grace mode.
//   - actor-canonical-reused (AC 5): for a new kind: actor signal, the
//     canonical must not appear in any actor-identity chain other than
//     the chain the new entry extends.
//   - role-canonical-mismatch + role-refs-missing-head (AC 7): for a new
//     kind: role decision, the Actor value must match the current head
//     canonical of an active chain AND Refs must include that head entry's
//     ID.
//   - procedure-canonical-reused: for a new kind: procedure decision, the
//     canonical must not appear in any procedure chain other than the chain
//     the new entry extends — the actor write-once rule applied to the
//     procedure canonical namespace.
//   - ref-kind-missing + ref-kind-invalid (d-tac-cs0 AC 8): every ref on a
//     new entry must carry a kind from the closed set, rejecting the legacy
//     `unknown` sentinel which is reserved for read-side bare-string round-
//     trip and not authorable.
//   - ref-kind-inapplicable (d-tac-tph AC 4): a ref kind whose precondition
//     is violated by the target's kind and derived status, per the
//     applicability matrix in the model layer. Deterministic — the LLM
//     advisory no longer scores applicability at all.
//   - supersede-non-head (d-cpt-rgx): a new entry's supersedes must target the
//     live head of a chain, not an entry that already has an active successor
//     (which would create a fork). Settled branches — the existing successor
//     has closed — are exempt.
//   - cross-repo-dep-undeclared (d-cpt-6cq): every cross-repo ref — forward
//     and backward class alike — requires its target repo_id to be a
//     declared dependency of this graph (committed .sdd/config.yaml), which
//     makes the one-way reference direction structural: a graph that does
//     not declare a repository cannot capture refs into it.
//   - cross-repo-ref-unresolved (d-cpt-uh0): a backward-class cross-repo ref
//     must resolve in the target repo's cached graph or capture blocks;
//     forward-class kinds (surfaces, required-by) are exempt. Local ref
//     resolution is enforced even earlier — a dangling local ref hard-blocks
//     at write-time validation before pre-flight runs.
//
// Severity is binary for structural checks: SeverityHigh or absent — partial
// coverage is never "kind-of an actor". The one medium is the serve-budget
// finding on procedure specs, advisory by design: overshoot is a risk, not a
// defect, and the spec still runs (d-tac-rzi).
func mechanicalPreflight(entry *model.Entry, graph *model.Graph, declaredDeps []string, resolver engine.QueryResolver) []query.Finding {
	if entry == nil || graph == nil {
		return nil
	}
	var findings []query.Finding

	findings = append(findings, participantCoverageFindings(entry, graph)...)
	findings = append(findings, focusActorCoverageFindings(entry, graph)...)
	findings = append(findings, refKindFindings(entry)...)
	findings = append(findings, refKindApplicabilityFindings(entry, graph)...)
	findings = append(findings, supersedeForkFindings(entry, graph)...)
	findings = append(findings, crossRepoResolutionFindings(entry, graphCrossRepoResolver(graph), declaredDeps)...)

	if entry.IsActor() {
		findings = append(findings, actorWriteOnceFindings(entry, graph)...)
	}
	if entry.IsRole() {
		findings = append(findings, roleMechanicalFindings(entry, graph)...)
	}
	if entry.IsProcedure() {
		findings = append(findings, procedureWriteOnceFindings(entry, graph)...)
		findings = append(findings, serveBudgetFindings(entry, resolver)...)
	}

	return findings
}

// serveBudgetFindings runs the advisory authoring arithmetic (d-tac-rzi) on a
// procedure entry: each step whose worst-case serve exceeds the effective
// total is one medium finding. A spec that does not parse is skipped — spec
// validation reports that on its own — and a nil resolver skips the check
// (lint catches the spec on every later sweep).
func serveBudgetFindings(entry *model.Entry, resolver engine.QueryResolver) []query.Finding {
	if resolver == nil {
		return nil
	}
	spec, err := engine.ParseSpec(entry)
	if err != nil {
		return nil
	}
	budget := serveview.Default()
	var findings []query.Finding
	for _, size := range spec.OverBudget(budget, resolver) {
		findings = append(findings, query.Finding{
			Severity: query.SeverityMedium,
			Category: "serve-budget",
			Observation: fmt.Sprintf("step %q sizes to a worst-case %d bytes against the %d-byte serve budget — tighten caps, or declare `serveBudget: %d` on the spec to record the trade",
				size.Step, size.Bytes, spec.EffectiveTotal(budget), size.Bytes),
		})
	}
	return findings
}

// refKindFindings enforces d-tac-cs0 AC 8 mechanical pre-flight: every ref
// on a new entry must carry a kind from the capturable closed set. Missing,
// empty, the legacy `unknown` sentinel, and unknown vocabulary values all
// produce a single high-severity finding per offending ref. The model layer
// rejects malformed object form at unmarshal — this check covers in-memory
// entries that bypass the YAML round-trip (CLI captures, programmatic flows).
func refKindFindings(entry *model.Entry) []query.Finding {
	var findings []query.Finding
	for i, r := range entry.Refs {
		switch {
		case r.Kind == "":
			findings = append(findings, query.Finding{
				Severity:    query.SeverityHigh,
				Category:    "ref-kind-missing",
				Observation: fmt.Sprintf("refs[%d] (%s): missing required `kind` (one of: %s)", i, r.ID, capturableRefKindList()),
			})
		case !model.IsCapturableRefKind(r.Kind):
			findings = append(findings, query.Finding{
				Severity:    query.SeverityHigh,
				Category:    "ref-kind-invalid",
				Observation: fmt.Sprintf("refs[%d] (%s): kind %q is not allowed for new entries (expected one of: %s)", i, r.ID, r.Kind, capturableRefKindList()),
			})
		}
	}
	return findings
}

// refKindApplicabilityFindings enforces the ref-kind applicability matrix
// (plan d-tac-tph AC 4): a ref whose kind's precondition is violated by the
// target's kind and derived status is a deterministic high. This is the
// lookup the LLM advisory kept getting wrong in both directions — blocking
// applicable kinds (s-prc-pex, s-prc-lbv, s-prc-2lm) — so it runs in code
// and the LLM rubric no longer scores applicability at all. Dangling targets
// are skipped (reported by ref resolution); kinds outside the capturable set
// are covered by refKindFindings.
func refKindApplicabilityFindings(entry *model.Entry, graph *model.Graph) []query.Finding {
	var findings []query.Finding
	for i, r := range entry.Refs {
		// Cross-repo targets resolve through the cached member graph, whose
		// derivation classifies them; unresolvable targets are skipped
		// (owned by ref resolution / resolve-or-block).
		target, owner, ok := graph.ResolveAcross(r.ID)
		if !ok {
			continue
		}
		status := owner.DerivedStatus(target)
		class := model.ClassifyRefTarget(target, status)
		cell, ok := model.RefKindApplicability(r.Kind, class)
		if !ok || cell.Applicable {
			continue
		}
		obs := fmt.Sprintf("refs[%d] (%s): kind %q does not apply to this target (%s) — %s. Admissible kinds here: %s",
			i, r.ID, r.Kind, class, cell.Note, refKindNames(model.AdmissibleRefKinds(class)))
		// A refines ref stranded on a superseded target usually means the
		// author meant the live head — point there rather than only naming
		// alternative kinds.
		if status.Kind == model.StatusSupersededBy {
			obs += fmt.Sprintf(". The target is superseded by %s — if the body sharpens the live head in place, re-point the ref at it", status.By)
		}
		findings = append(findings, query.Finding{
			Severity:    query.SeverityHigh,
			Category:    "ref-kind-inapplicable",
			Observation: obs,
		})
	}
	return findings
}

// crossRepoRefResolution reports how a cross-repo ref target resolved
// against the connected-repos machinery.
type crossRepoRefResolution int

const (
	// crossRepoRepoUnavailable: the target repo is not connected or its
	// cache cannot be read — resolution is impossible.
	crossRepoRepoUnavailable crossRepoRefResolution = iota
	// crossRepoEntryMissing: the repo's cached graph is available (fetched
	// fresh on miss) but does not contain the entry on its default branch.
	crossRepoEntryMissing
	// crossRepoEntryResolved: the target entry exists in the cached graph.
	crossRepoEntryResolved
)

// crossRepoRefResolver resolves a cross-repo target against the cached
// remote graph. nil means no repo is resolvable.
type crossRepoRefResolver func(repoID, entryID string) crossRepoRefResolution

// graphCrossRepoResolver resolves through the graph's cross-graph assembly
// (the MultiGraph the GraphSource attached): member graphs load lazily from
// the connected-repos caches. Fetch-on-miss is the write handler's job —
// it refreshes caches for referenced repos before pre-flight runs, so this
// resolver reads the live cache state.
func graphCrossRepoResolver(graph *model.Graph) crossRepoRefResolver {
	return func(repoID, entryID string) crossRepoRefResolution {
		member, err := graph.MemberGraph(repoID)
		if err != nil || member == nil {
			return crossRepoRepoUnavailable
		}
		if _, ok := member.ByID[entryID]; !ok {
			return crossRepoEntryMissing
		}
		return crossRepoEntryResolved
	}
}

// crossRepoResolutionFindings enforces the two cross-repo capture
// preconditions. First, the declared-dependency rule (d-cpt-6cq): every
// cross-repo ref — forward and backward class alike — requires its target
// repo_id to be declared in this graph's committed dependencies, so the
// one-way direction holds by construction; an undeclared repo blocks
// without further resolution (declaring it is the fix, and the dependency
// finding already carries it). Second, resolve-or-block (d-cpt-uh0): a
// backward-class ref into a declared repo must resolve in its cached graph
// or capture blocks at high severity. Forward-class kinds (surfaces,
// required-by) are exempt from resolution — their target may legitimately
// be absent. Local refs are not checked here: a dangling local ref already
// hard-blocks at write-time validation, before pre-flight runs.
func crossRepoResolutionFindings(entry *model.Entry, resolve crossRepoRefResolver, declaredDeps []string) []query.Finding {
	declared := make(map[string]bool, len(declaredDeps))
	for _, d := range declaredDeps {
		declared[d] = true
	}
	var findings []query.Finding
	for i, r := range entry.Refs {
		repoID, entryID, ok := model.SplitCrossRepoID(r.ID)
		if !ok {
			continue
		}
		if !declared[repoID] {
			findings = append(findings, query.Finding{
				Severity:    query.SeverityHigh,
				Category:    "cross-repo-dep-undeclared",
				Observation: fmt.Sprintf("refs[%d] (%s): repo %q is not a declared dependency of this graph — declare it with `sdd repo add <clone-url>` (conventionally https://%s), which records the committed dependency and the per-user connection", i, r.ID, repoID, repoID),
			})
			continue
		}
		if model.IsForwardClassRefKind(r.Kind) {
			continue
		}
		if resolve == nil {
			findings = append(findings, query.Finding{
				Severity:    query.SeverityHigh,
				Category:    "cross-repo-ref-unresolved",
				Observation: fmt.Sprintf("refs[%d] (%s): cannot resolve cross-repo ref — repo %q is declared but not connected on this machine; run `sdd repo add <clone-url>` (conventionally https://%s)", i, r.ID, repoID, repoID),
			})
			continue
		}
		switch resolve(repoID, entryID) {
		case crossRepoRepoUnavailable:
			findings = append(findings, query.Finding{
				Severity:    query.SeverityHigh,
				Category:    "cross-repo-ref-unresolved",
				Observation: fmt.Sprintf("refs[%d] (%s): cannot resolve cross-repo ref — repo %q is declared but not connected on this machine; run `sdd repo add <clone-url>` (conventionally https://%s)", i, r.ID, repoID, repoID),
			})
		case crossRepoEntryMissing:
			findings = append(findings, query.Finding{
				Severity:    query.SeverityHigh,
				Category:    "cross-repo-ref-unresolved",
				Observation: fmt.Sprintf("refs[%d] (%s): entry %s is absent from repo %q's default branch — a backward-class ref must target an entry that exists", i, r.ID, entryID, repoID),
			})
		case crossRepoEntryResolved:
			// resolves — nothing to report
		}
	}
	return findings
}

func refKindNames(kinds []model.RefKind) string {
	parts := make([]string, len(kinds))
	for i, k := range kinds {
		parts[i] = string(k)
	}
	return strings.Join(parts, ", ")
}

// supersedeForkFindings enforces capture-time fork prevention (d-cpt-rgx): a
// new entry must supersede the live head of a chain, not an entry that already
// has an active successor. Superseding a non-head leaves two active successors
// competing for head resolution — a fork. Mechanical and binary: high when a
// supersedes target resolves to a different, still-active head; the author
// should supersede that head instead. A target whose existing successor has
// since closed is a settled branch, not a fork, so reviving it is allowed.
func supersedeForkFindings(entry *model.Entry, graph *model.Graph) []query.Finding {
	var findings []query.Finding
	for _, sid := range entry.Supersedes {
		if _, ok := graph.ByID[sid]; !ok {
			continue // dangling supersedes is reported by ref resolution
		}
		head := graph.ResolveRef(sid).Head()
		if head == sid {
			continue // sid is already a live head — linear supersession
		}
		headEntry := graph.ByID[head]
		if headEntry == nil || isInactiveStatus(graph.DerivedStatus(headEntry).Kind) {
			continue // existing branch settled (head closed) — not a live fork
		}
		findings = append(findings, query.Finding{
			Severity:    query.SeverityHigh,
			Category:    "supersede-non-head",
			Observation: fmt.Sprintf("supersedes %s, which is already superseded by the active entry %s — supersede %s (the live head) instead to keep the chain linear and avoid a fork", sid, head, head),
		})
	}
	return findings
}

func capturableRefKindList() string {
	values := model.RefKindValues()
	parts := make([]string, 0, len(values))
	for _, k := range values {
		parts = append(parts, string(k))
	}
	return strings.Join(parts, ", ")
}

// participantCoverageFindings enforces AC 6: every name listed in the
// proposed entry's Participants must match the canonical of an active
// actor signal. Grace mode (no active actors yet) skips the check so
// fresh graphs aren't blocked before the first actor is captured.
func participantCoverageFindings(entry *model.Entry, graph *model.Graph) []query.Finding {
	canonicals, active := activeActorCanonicals(graph)
	if !active {
		return nil // grace mode
	}
	var findings []query.Finding
	for _, p := range entry.Participants {
		if p == "" {
			continue
		}
		if _, ok := canonicals[p]; ok {
			continue
		}
		findings = append(findings, query.Finding{
			Severity:    query.SeverityHigh,
			Category:    "participant-drift",
			Observation: fmt.Sprintf("participant %q does not match any active actor canonical", p),
		})
	}
	return findings
}

// activeActorCanonicals returns the canonical set of every active actor head.
// The bool is false in grace mode (no active actor signals yet) so coverage
// checks skip a fresh graph rather than blocking before the first actor lands.
func activeActorCanonicals(graph *model.Graph) (map[string]struct{}, bool) {
	active := graph.ActiveActorHeads()
	if len(active) == 0 {
		return nil, false
	}
	canonicals := make(map[string]struct{}, len(active))
	for _, a := range active {
		if a.Canonical != "" {
			canonicals[a.Canonical] = struct{}{}
		}
	}
	return canonicals, true
}

// focusActorCoverageFindings applies participant coverage to the actor fields
// of a kind: focus decision: every focus-level actor and every per-involvement
// actor override must match an active actor canonical. Shares the canonical set
// and grace mode with participantCoverageFindings; non-focus entries pass.
func focusActorCoverageFindings(entry *model.Entry, graph *model.Graph) []query.Finding {
	if !entry.IsFocus() {
		return nil
	}
	canonicals, active := activeActorCanonicals(graph)
	if !active {
		return nil // grace mode
	}
	var findings []query.Finding
	for i, name := range entry.FocusActors {
		if name == "" {
			continue
		}
		if _, ok := canonicals[name]; ok {
			continue
		}
		findings = append(findings, query.Finding{
			Severity:    query.SeverityHigh,
			Category:    "focus-actor-drift",
			Observation: fmt.Sprintf("actors[%d] %q does not match any active actor canonical", i, name),
		})
	}
	for i, inv := range entry.Involvement {
		for j, name := range inv.Actors {
			if name == "" {
				continue
			}
			if _, ok := canonicals[name]; ok {
				continue
			}
			findings = append(findings, query.Finding{
				Severity:    query.SeverityHigh,
				Category:    "focus-actor-drift",
				Observation: fmt.Sprintf("involvement[%d].actors[%d] %q does not match any active actor canonical", i, j, name),
			})
		}
	}
	return findings
}

// actorWriteOnceFindings enforces AC 5: a new actor signal's canonical must
// not appear in any actor-identity chain other than the chain the new entry
// extends. Within-chain reuse (same canonical repeated or changed across
// supersessions) is fine. Closed chains do not free their canonicals.
func actorWriteOnceFindings(entry *model.Entry, graph *model.Graph) []query.Finding {
	canonical := strings.TrimSpace(entry.Canonical)
	if canonical == "" {
		return nil // missing canonical reported by frontmatter validator
	}

	parentChain := parentActorChain(entry, graph)

	for _, chain := range graph.ActorChains() {
		if !chain.HasCanonical(canonical) {
			continue
		}
		if parentChain != nil && sameChain(chain, parentChain) {
			continue
		}
		headID := "<unknown>"
		if chain.Head != nil {
			headID = chain.Head.ID
		}
		return []query.Finding{{
			Severity:    query.SeverityHigh,
			Category:    "actor-canonical-reused",
			Observation: fmt.Sprintf("canonical %q is already used by actor-identity chain with head %s (write-once across chains)", canonical, headID),
		}}
	}
	return nil
}

// roleMechanicalFindings enforces AC 7: role's Actor must match the current
// head canonical of an active actor-identity chain, AND Refs must include
// that head entry's ID. Both checks are capture-time; derivation-time
// resolution walks full canonical history (see Graph.DerivedStatus).
func roleMechanicalFindings(entry *model.Entry, graph *model.Graph) []query.Finding {
	actor := strings.TrimSpace(entry.Actor)
	if actor == "" {
		return nil // missing actor reported by frontmatter validator
	}

	var matchedHead *model.Entry
	for _, head := range graph.ActiveActorHeads() {
		if head.Canonical == actor {
			matchedHead = head
			break
		}
	}

	var findings []query.Finding
	if matchedHead == nil {
		findings = append(findings, query.Finding{
			Severity:    query.SeverityHigh,
			Category:    "role-canonical-mismatch",
			Observation: fmt.Sprintf("role actor %q does not match the current head canonical of any active actor-identity chain", actor),
		})
		return findings
	}

	hasHeadRef := false
	for _, r := range entry.Refs {
		if r.ID == matchedHead.ID {
			hasHeadRef = true
			break
		}
	}
	if !hasHeadRef {
		findings = append(findings, query.Finding{
			Severity:    query.SeverityHigh,
			Category:    "role-refs-missing-head",
			Observation: fmt.Sprintf("role refs must include %s (the head actor signal for canonical %q)", matchedHead.ID, actor),
		})
	}
	return findings
}

// procedureWriteOnceFindings enforces write-once for procedure canonicals: a
// new procedure decision's canonical must not appear in any procedure chain
// other than the chain the new entry extends via its supersedes links.
// Within-chain reuse is fine; closed chains do not free their canonicals.
// Procedure canonicals are their own namespace — actor canonicals are not
// consulted.
func procedureWriteOnceFindings(entry *model.Entry, graph *model.Graph) []query.Finding {
	canonical := strings.TrimSpace(entry.Canonical)
	if canonical == "" {
		return nil // missing canonical reported by frontmatter validator
	}

	parentChain := parentProcedureChain(entry, graph)

	for _, chain := range graph.ProcedureChains() {
		if !chain.HasCanonical(canonical) {
			continue
		}
		if parentChain != nil && sameProcedureChain(chain, parentChain) {
			continue
		}
		headID := "<unknown>"
		if chain.Head != nil {
			headID = chain.Head.ID
		}
		return []query.Finding{{
			Severity:    query.SeverityHigh,
			Category:    "procedure-canonical-reused",
			Observation: fmt.Sprintf("canonical %q is already used by the procedure chain with head %s (write-once across chains) — supersede that chain's head to revise the move instead", canonical, headID),
		}}
	}
	return nil
}

// parentProcedureChain returns the procedure chain a new procedure entry
// extends via its Supersedes links. Returns nil when the entry does not
// supersede any existing procedure (starts a new chain).
func parentProcedureChain(entry *model.Entry, graph *model.Graph) *model.ProcedureChain {
	for _, sid := range entry.Supersedes {
		parent, ok := graph.ByID[sid]
		if !ok || !parent.IsProcedure() {
			continue
		}
		for _, chain := range graph.ProcedureChains() {
			for _, e := range chain.Entries {
				if e.ID == parent.ID {
					return chain
				}
			}
		}
	}
	return nil
}

// sameProcedureChain reports whether two chain pointers refer to the same
// procedure chain, resolved via head ID like sameChain for actors.
func sameProcedureChain(a, b *model.ProcedureChain) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Head == nil || b.Head == nil {
		return false
	}
	return a.Head.ID == b.Head.ID
}

// parentActorChain returns the actor chain a new actor entry extends via
// its Supersedes links. Returns nil when the entry does not supersede any
// existing actor signal (starts a new chain).
func parentActorChain(entry *model.Entry, graph *model.Graph) *model.ActorChain {
	for _, sid := range entry.Supersedes {
		parent, ok := graph.ByID[sid]
		if !ok || !parent.IsActor() {
			continue
		}
		for _, chain := range graph.ActorChains() {
			for _, e := range chain.Entries {
				if e.ID == parent.ID {
					return chain
				}
			}
		}
	}
	return nil
}

// sameChain reports whether two chain pointers refer to the same actor
// chain. Identity is resolved via head ID since ActorChains builds fresh
// slices each call; pointer equality would fail across separate invocations.
func sameChain(a, b *model.ActorChain) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Head == nil || b.Head == nil {
		return false
	}
	return a.Head.ID == b.Head.ID
}
