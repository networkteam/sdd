// Package viewlayout renders the user-facing reference for the composable
// view layout language. Callers supply the live executor vocabularies so CLI,
// MCP, and embedded reference surfaces share names instead of copying them.
package viewlayout

import (
	"fmt"
	"slices"
	"strings"
)

// Vocabulary is the live layout vocabulary exposed by the packages that
// execute each part of the language.
type Vocabulary struct {
	Functions  []string
	Renders    []string
	Algorithms []string
	Decays     []string
	Macros     []string
}

type item struct {
	category    string
	syntax      string
	description string
}

var functionReference = map[string]item{
	"source":      {"Sources", "source(graph|wip)", "Select graph entries (default) or active WIP markers."},
	"active":      {"Filters", "active", "Keep entries that are neither closed nor superseded."},
	"kind":        {"Filters", "kind(K[, K2, ...])", "Keep entries matching any listed kind; repeated calls intersect."},
	"intent":      {"Filters", "intent(I[, I2, ...])", "Keep directives with pending, guiding, or settled intent."},
	"type":        {"Filters", "type(T)", "Keep decisions/signals; accepts d, s, decision, or signal."},
	"layer":       {"Filters", "layer(L)", "Keep one thinking layer; accepts short or full layer names."},
	"since":       {"Filters", "since(\"spec\")", "Keep entries since an ISO date or duration such as \"7d\" or \"1m\"."},
	"topic":       {"Filters", "topic(L)", "Keep entries whose effective topics have the component-wise prefix L."},
	"participant": {"Filters", "participant(P[, P2, ...])", "Keep entries naming any canonical participant; quote names containing spaces."},
	"untagged":    {"Filters", "untagged", "Keep entries whose effective topic set is empty."},
	"id":          {"Filters", "id(ID[, ID2, ...])", "Keep listed entries; quote full IDs and use short IDs bare."},
	"not":         {"Filters", "not(<filter>)", "Exclude matches of kind, intent, layer, or topic filters."},
	"rank":        {"Rank", "rank(<algorithm>)", "Sort by a live ranking algorithm, descending unless the algorithm defines otherwise."},
	"n":           {"Page", "n(N)", "Take the first N entries after filtering and ranking."},
	"group":       {"Aggregate", "group(by(<field>))", "Group by kind, layer, type, or participant; render with as-grouped."},
	"expand":      {"Transform", "expand(involvement|refs)", "Expand focus involvement or outgoing references; refs(inactive) narrows ref rows."},
	"name":        {"Output", "name(\"title\")", "Set the final section heading; the last call wins."},
	"name-prefix": {"Output", "name-prefix(\"title\")", "Set a heading prefix that composes with the rank suffix."},
	"stalled":     {"Output", "stalled(value)", "Set the focus-block stalled threshold (default 1.0)."},
	"brief":       {"Output", "brief", "Render compact entry lines containing only identity and first summary sentence."},
}

var renderReference = map[string]string{
	"as-list":               "Flat entry list.",
	"as-grouped":            "Grouped entries; requires group(by(...)).",
	"as-counts":             "Per-topic count and heat rows over the filtered set.",
	"as-focus-block":        "Focus targets with pull-available, stalled, or driving state.",
	"as-participants-block": "Active actor heads with their bound active roles.",
	"as-wip-list":           "Active WIP marker rows; requires source(wip).",
}

var algorithmReference = map[string]string{
	"heat":      "incoming-reference heat; default decay exp-14d",
	"in-degree": "raw incoming-reference count",
	"mult":      "heat multiplied by in-degree",
	"add":       "heat plus in-degree",
	"log":       "heat multiplied by log(1 + in-degree)",
	"coldness":  "fresh, weakly referenced entries first; default decay exp-30d",
	"by(date)":  "entry creation time, newest first",
}

var decayReference = map[string]string{
	"exp-7d":     "exponential 7-day half-life",
	"exp-14d":    "exponential 14-day half-life",
	"exp-30d":    "exponential 30-day half-life",
	"linear-7d":  "linear decay to zero after 7 days",
	"linear-14d": "linear decay to zero after 14 days",
	"linear-30d": "linear decay to zero after 30 days",
	"none":       "no age effect",
}

var macroReference = map[string]string{
	"top":          "top(N) — active entries ranked by heat",
	"topic":        "topic(L) — one topic neighborhood ranked by heat",
	"focus":        "focus — active focuses with involvement state",
	"decisions":    "decisions — active decisions grouped by kind",
	"signals":      "signals — active gaps and questions grouped by kind",
	"insights":     "insights — recent active insights by date",
	"done":         "done — recent done signals by date",
	"aspirations":  "aspirations — active aspirations",
	"contracts":    "contracts — active contracts",
	"participants": "participants — active actors with bound roles",
	"wip":          "wip — active WIP markers",
	"readiness":    "readiness — capped bootstrap grounding: participants, aspirations, strategic and conceptual guiding",
}

var categoryOrder = []string{"Sources", "Filters", "Rank", "Page", "Aggregate", "Transform", "Output", "Other primitives"}

// Reference renders the complete CLI-shaped layout reference around the
// host-neutral vocabulary body.
func Reference(v Vocabulary) string {
	var b strings.Builder
	b.WriteString("Usage: sdd view --layout=<spec>\n\n")
	b.WriteString(ReferenceBody(v))
	b.WriteString("\nExamples:\n")
	b.WriteString("  sdd view --layout='top(20)'\n")
	b.WriteString("  sdd view --layout='active:kind(plan):rank(heat(exp-14d)):n(10):as-list'\n")
	b.WriteString("  sdd view --layout='topic(\"infrastructure/cli\"):rank(by(date)):n(20):as-list'\n")
	b.WriteString("  sdd view --layout='active:participant(\"Jonathan Philipp\"):as-list'\n")
	b.WriteString("  sdd view --layout='decisions,signals,participants'\n")
	b.WriteString("  sdd view --layout='top(20):not(kind(contract,aspiration))'\n")
	b.WriteString("  sdd view --layout='wip'\n")
	return b.String()
}

// ReferenceBody renders the host-neutral grammar and categorized vocabulary.
// Every live name is emitted even when descriptive metadata has not caught up,
// making a newly added primitive discoverable instead of silently absent.
func ReferenceBody(v Vocabulary) string {
	renders := make(map[string]struct{}, len(v.Renders))
	for _, name := range v.Renders {
		renders[name] = struct{}{}
	}

	byCategory := make(map[string][]string)
	for _, name := range v.Functions {
		if _, isRender := renders[name]; isRender {
			continue
		}
		ref, ok := functionReference[name]
		if !ok {
			ref = item{category: "Other primitives", syntax: name, description: "See executor validation for accepted arguments."}
		}
		byCategory[ref.category] = append(byCategory[ref.category], name)
	}

	var b strings.Builder
	b.WriteString("Compose colon-chained functions into a section; separate multiple sections with commas.\n")
	b.WriteString("Every section ends in a render function. Filters intersect; rank, page, name, and\n")
	b.WriteString("render modifiers use the last call of their kind. No whitespace is allowed outside\n")
	b.WriteString("quoted strings. Quote multi-word names, dates, durations, and topic paths.\n\n")
	b.WriteString("Grammar:\n")
	b.WriteString("  layout  := section (\",\" section)*\n")
	b.WriteString("  section := function (\":\" function)*\n")
	b.WriteString("  function := name (\"(\" arguments? \")\")?\n\n")
	b.WriteString("Implemented pipeline vocabulary:\n")

	for _, category := range categoryOrder {
		names := byCategory[category]
		if len(names) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n  %s:\n", category)
		for _, name := range names {
			ref := functionReference[name]
			if ref.syntax == "" {
				ref.syntax = name
			}
			fmt.Fprintf(&b, "    %-30s %s\n", ref.syntax, ref.description)
		}
	}

	b.WriteString("\n  Render (required terminator):\n")
	for _, name := range v.Renders {
		description := renderReference[name]
		if description == "" {
			description = "Executor-defined render shape."
		}
		fmt.Fprintf(&b, "    %-30s %s\n", name, description)
	}

	b.WriteString("\n  Rank algorithms:\n")
	for _, name := range v.Algorithms {
		description := algorithmReference[name]
		if description == "" {
			description = "Executor-defined ranking algorithm."
		}
		fmt.Fprintf(&b, "    %-30s %s\n", name, description)
	}

	b.WriteString("\n  Decays:\n")
	for _, name := range v.Decays {
		description := decayReference[name]
		if description == "" {
			description = "Model-defined decay function."
		}
		fmt.Fprintf(&b, "    %-30s %s\n", name, description)
	}

	b.WriteString("\n  Macros (recognized at section start; later modifiers override defaults):\n")
	for _, name := range v.Macros {
		description := macroReference[name]
		if description == "" {
			description = name + " — query-defined macro"
		}
		fmt.Fprintf(&b, "    %s\n", description)
	}

	return b.String()
}

// MissingReferenceNames returns live vocabulary names that lack descriptive
// metadata. Tests use it to keep the reference explanatory as well as
// mechanically complete.
func MissingReferenceNames(v Vocabulary) []string {
	var missing []string
	for _, name := range v.Functions {
		if slices.Contains(v.Renders, name) {
			if _, ok := renderReference[name]; !ok {
				missing = append(missing, name)
			}
			continue
		}
		if _, ok := functionReference[name]; !ok {
			missing = append(missing, name)
		}
	}
	for _, name := range v.Algorithms {
		if _, ok := algorithmReference[name]; !ok {
			missing = append(missing, name)
		}
	}
	for _, name := range v.Decays {
		if _, ok := decayReference[name]; !ok {
			missing = append(missing, name)
		}
	}
	for _, name := range v.Macros {
		if _, ok := macroReference[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}
