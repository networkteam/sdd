package viewlayout

import (
	"fmt"
	"slices"
	"strings"
)

// Vocabulary lists names accepted by the live view executors.
type Vocabulary struct {
	Functions  []string
	Renders    []string
	Algorithms []string
	Decays     []string
	Macros     []string
	// LayoutMacros are macros used alone as a whole layout (they expand into
	// several sections), not at section start.
	LayoutMacros []string
}

type referenceItem struct {
	category    string
	syntax      string
	description string
}

type referenceSection struct {
	title string
	items []referenceItem
}

type referenceModel struct {
	sections []referenceSection
	grammar  []string
	examples []string
}

var functionReference = map[string]referenceItem{
	"source":      {"Sources", "source(graph|wip)", "Select graph entries (default) or active WIP markers."},
	"active":      {"Filters", "active", "Keep the derived active/open population, including terminal done signals; exclude closed, superseded, settled, cascade-closed, and orphaned entries."},
	"indexed":     {"Filters", "indexed", "Keep entries that carry fact-index metadata; lifecycle state is unchanged."},
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
	"skip":        {"Page", "skip(N)", "Drop the first N entries after filtering and ranking, before n(N) — pages through what a bounded list cut."},
	"group":       {"Aggregate", "group(by(<field>))", "Group by kind, layer, type, or participant; render with as-grouped."},
	"expand":      {"Transform", "expand(involvement|refs)", "Expand focus involvement or outgoing references; refs(inactive) narrows ref rows."},
	"name":        {"Output", "name(\"title\")", "Set the final section heading; the last call wins."},
	"name-prefix": {"Output", "name-prefix(\"title\")", "Set a heading prefix that composes with the rank suffix."},
	"stalled":     {"Output", "stalled(value)", "Set the focus-block stalled threshold (default 1.0)."},
	"brief":       {"Output", "brief", "Render compact entry lines containing only identity and first summary sentence."},
}

var renderReference = map[string]string{
	"as-bodies":             "Full entry bodies with an identity header, demoted beneath the section heading; pair with narrow filters.",
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
	"top":          "active entries ranked by heat",
	"topic":        "one topic neighborhood ranked by heat",
	"focus":        "active focuses with involvement state",
	"decisions":    "active decisions grouped by kind",
	"signals":      "active gaps and questions grouped by kind",
	"insights":     "recent active insights by date",
	"done":         "recent done signals by date",
	"aspirations":  "active aspirations",
	"contracts":    "active contracts",
	"participants": "active actors with bound roles",
	"wip":          "active WIP markers",
	"readiness":    "capped bootstrap grounding: participants, aspirations, strategic and conceptual guiding",
}

var macroSyntax = map[string]string{
	"top": "top(N)", "topic": "topic(L)", "focus": "focus", "decisions": "decisions",
	"signals": "signals", "insights": "insights", "done": "done", "aspirations": "aspirations",
	"contracts": "contracts", "participants": "participants", "wip": "wip", "readiness": "readiness",
}

var categoryOrder = []string{"Sources", "Filters", "Rank", "Page", "Aggregate", "Transform", "Output", "Other primitives"}

var referenceGrammar = []string{
	`layout  := section ("," section)*`,
	`section := function (":" function)*`,
	`function := name ("(" arguments? ")")?`,
}

var referenceExamples = []string{
	"top(20)",
	"active:indexed:as-list",
	"active:kind(plan):rank(heat(exp-14d)):n(10):as-list",
	`topic("infrastructure/cli"):rank(by(date)):n(20):as-list`,
	`active:participant("Jonathan Philipp"):as-list`,
	"decisions,signals,participants",
	"top(20):not(kind(contract,aspiration))",
	"wip",
}

// ExampleSpecs returns the shared reference examples.
func ExampleSpecs() []string {
	return slices.Clone(referenceExamples)
}

func buildReference(v Vocabulary) referenceModel {
	renders := make(map[string]struct{}, len(v.Renders))
	for _, name := range v.Renders {
		renders[name] = struct{}{}
	}
	byCategory := make(map[string][]referenceItem)
	for _, name := range v.Functions {
		if _, isRender := renders[name]; isRender {
			continue
		}
		item, ok := functionReference[name]
		if !ok {
			item = referenceItem{category: "Other primitives", syntax: name, description: missingMetadataError(name, "functionReference")}
		}
		byCategory[item.category] = append(byCategory[item.category], item)
	}

	model := referenceModel{grammar: slices.Clone(referenceGrammar), examples: slices.Clone(referenceExamples)}
	for _, category := range categoryOrder {
		if items := byCategory[category]; len(items) > 0 {
			model.sections = append(model.sections, referenceSection{title: category, items: items})
		}
	}
	model.sections = append(model.sections,
		referenceSection{title: "Render (required terminator)", items: namedItems(v.Renders, renderReference, "Executor-defined render shape.")},
		referenceSection{title: "Rank algorithms", items: namedItems(v.Algorithms, algorithmReference, "Executor-defined ranking algorithm.")},
		referenceSection{title: "Decays", items: namedItems(v.Decays, decayReference, "Model-defined decay function.")},
		referenceSection{title: "Macros (recognized at section start; later modifiers override defaults)", items: macroItems(v.Macros)},
	)
	if len(v.LayoutMacros) > 0 {
		model.sections = append(model.sections,
			referenceSection{title: "Layout macros (used alone as the whole layout, not at section start)", items: macroItems(v.LayoutMacros)},
		)
	}
	return model
}

func macroItems(names []string) []referenceItem {
	items := make([]referenceItem, 0, len(names))
	for _, name := range names {
		syntax := macroSyntax[name]
		if syntax == "" {
			syntax = name
		}
		description, ok := macroReference[name]
		if !ok {
			description = missingMetadataError(name, "macroReference")
		}
		items = append(items, referenceItem{syntax: syntax, description: description})
	}
	return items
}

func namedItems(names []string, descriptions map[string]string, fallback string) []referenceItem {
	items := make([]referenceItem, 0, len(names))
	for _, name := range names {
		description := descriptions[name]
		if description == "" {
			description = fallback
		}
		items = append(items, referenceItem{syntax: name, description: description})
	}
	return items
}

// Reference renders the terminal layout reference.
func Reference(v Vocabulary) string {
	model := buildReference(v)
	var b strings.Builder
	b.WriteString("Usage: sdd view --layout=<spec>\n\n")
	b.WriteString(referenceBody(model))
	b.WriteString("\nExamples:\n")
	for _, example := range model.examples {
		fmt.Fprintf(&b, "  sdd view --layout='%s'\n", example)
	}
	return b.String()
}

func referenceBody(model referenceModel) string {
	var b strings.Builder
	b.WriteString("Compose colon-chained functions into a section; separate multiple sections with commas.\n")
	b.WriteString("Every section ends in a render function. Filters intersect; rank, page, name, and\n")
	b.WriteString("render modifiers use the last call of their kind. No whitespace is allowed outside\n")
	b.WriteString("quoted strings. Quote multi-word names, dates, durations, and topic paths.\n\n")
	b.WriteString("Grammar:\n")
	for _, line := range model.grammar {
		fmt.Fprintf(&b, "  %s\n", line)
	}
	b.WriteString("\nImplemented pipeline vocabulary:\n")
	for _, section := range model.sections {
		if len(section.items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n  %s:\n", section.title)
		for _, item := range section.items {
			if strings.HasPrefix(section.title, "Macros ") || strings.HasPrefix(section.title, "Layout macros") {
				fmt.Fprintf(&b, "    %s — %s\n", item.syntax, item.description)
			} else {
				fmt.Fprintf(&b, "    %-30s %s\n", item.syntax, item.description)
			}
		}
	}
	return b.String()
}

// Markdown renders the host-neutral layout reference.
func Markdown(v Vocabulary) string {
	model := buildReference(v)
	var b strings.Builder
	b.WriteString("# How to compose graph views\n\n")
	b.WriteString("Compose colon-chained functions into a section and separate multiple sections with commas. Every section ends in a render function. Filters intersect; rank, page, name, and render modifiers use the last call of their kind. No whitespace is allowed outside quoted strings. Quote multi-word names, dates, durations, and topic paths.\n\n")
	b.WriteString("## Grammar\n\n```text\n")
	for _, line := range model.grammar {
		b.WriteString(line + "\n")
	}
	b.WriteString("```\n\n## Vocabulary\n\n| Category | Syntax | Meaning |\n| --- | --- | --- |\n")
	for _, section := range model.sections {
		if len(section.items) == 0 {
			continue
		}
		for _, item := range section.items {
			fmt.Fprintf(&b, "| %s | `%s` | %s |\n", escapeTable(section.title), escapeTable(item.syntax), escapeTable(item.description))
		}
	}
	b.WriteString("\n## Example layout specifications\n\n```text\n")
	for _, example := range model.examples {
		b.WriteString(example + "\n")
	}
	b.WriteString("```\n")
	return b.String()
}

// ReferenceBody renders the host-neutral body (grammar plus pipeline
// vocabulary) without the terminal Usage/Examples framing — the shared surface
// behind both Reference and the view-grammar base fact.
func ReferenceBody(v Vocabulary) string {
	return referenceBody(buildReference(v))
}

func escapeTable(value string) string {
	return strings.ReplaceAll(value, "|", `\|`)
}

// missingMetadataError is the loud placeholder rendered when a live vocabulary
// name has no descriptive entry in its metadata map. It renders into
// `sdd view --help` and the auto-rendered view-grammar base fact, so a missing
// registration is immediately visible instead of silently blank.
func missingMetadataError(name, mapName string) string {
	return fmt.Sprintf("ERROR: missing reference metadata for %q — register it in %s", name, mapName)
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
	for _, name := range v.LayoutMacros {
		if _, ok := macroReference[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}
