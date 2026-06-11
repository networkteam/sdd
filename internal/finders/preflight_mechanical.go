package finders

import (
	"fmt"
	"strings"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
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
//   - actor-canonical-reused (AC 5): for a new kind: actor signal, the
//     canonical must not appear in any actor-identity chain other than
//     the chain the new entry extends.
//   - role-canonical-mismatch + role-refs-missing-head (AC 7): for a new
//     kind: role decision, the Actor value must match the current head
//     canonical of an active chain AND Refs must include that head entry's
//     ID.
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
//
// Severity is strictly binary: SeverityHigh or absent. Mechanical checks
// never emit medium or low; partial coverage is never "kind-of an actor".
func mechanicalPreflight(entry *model.Entry, graph *model.Graph) []query.Finding {
	if entry == nil || graph == nil {
		return nil
	}
	var findings []query.Finding

	findings = append(findings, participantCoverageFindings(entry, graph)...)
	findings = append(findings, refKindFindings(entry)...)
	findings = append(findings, refKindApplicabilityFindings(entry, graph)...)
	findings = append(findings, supersedeForkFindings(entry, graph)...)

	if entry.IsActor() {
		findings = append(findings, actorWriteOnceFindings(entry, graph)...)
	}
	if entry.IsRole() {
		findings = append(findings, roleMechanicalFindings(entry, graph)...)
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
		target, ok := graph.ByID[r.ID]
		if !ok {
			continue
		}
		status := graph.DerivedStatus(target)
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
	active := graph.ActiveActorHeads()
	if len(active) == 0 {
		return nil // grace mode
	}
	canonicals := make(map[string]struct{}, len(active))
	for _, a := range active {
		if a.Canonical != "" {
			canonicals[a.Canonical] = struct{}{}
		}
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
