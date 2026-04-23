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
//
// Severity is strictly binary: SeverityHigh or absent. Mechanical checks
// never emit medium or low; partial coverage is never "kind-of an actor".
func mechanicalPreflight(entry *model.Entry, graph *model.Graph) []query.Finding {
	if entry == nil || graph == nil {
		return nil
	}
	var findings []query.Finding

	findings = append(findings, participantCoverageFindings(entry, graph)...)

	if entry.IsActor() {
		findings = append(findings, actorWriteOnceFindings(entry, graph)...)
	}
	if entry.IsRole() {
		findings = append(findings, roleMechanicalFindings(entry, graph)...)
	}

	return findings
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
		if r == matchedHead.ID {
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
