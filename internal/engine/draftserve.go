package engine

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/networkteam/sdd/internal/textdiff"
)

// Draft serving renders a step's serveDelta fields as an engine-owned block:
// whole on the first serve at the step, only what changed after — prose as
// content diffs anchored on what the engine last served this instance,
// lists item-level, scalars whole — and everything whole again on the
// rehydrate path (resume / whole-position replay), which also resets the
// delta base (20260826-120330-d-tac-8f8). The base is in-memory only: a
// process restart forgets it, so the next serve is whole by construction.

const (
	proseAckHead = 6
	proseAckTail = 3
)

// draftSnapshot captures the current serveDelta field values in canonical
// JSON form, so equality and diffs compare stable bytes.
func draftSnapshot(inst *Instance, fields []string) map[string]string {
	snap := make(map[string]string, len(fields))
	for _, name := range fields {
		v, ok := inst.Store.Get(name)
		if !ok {
			continue
		}
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		snap[name] = string(b)
	}
	return snap
}

// renderDraft renders the draft block. prev nil means first serve (or a full
// re-serve); full additionally serves prose whole instead of head-and-tail.
func renderDraft(spec *Spec, fields []string, prev, cur map[string]string, full bool) string {
	var sb strings.Builder
	if prev == nil {
		sb.WriteString("Draft as it stands (engine-rendered — what you confirm is this):\n")
		for _, name := range fields {
			raw, ok := cur[name]
			if !ok {
				continue
			}
			sb.WriteString(renderDraftField(spec, name, raw, full))
		}
		return strings.TrimSuffix(sb.String(), "\n")
	}

	var changed, cleared, unchanged []string
	for _, name := range fields {
		curRaw, curOK := cur[name]
		prevRaw, prevOK := prev[name]
		switch {
		case curOK && !prevOK:
			changed = append(changed, name)
		case !curOK && prevOK:
			cleared = append(cleared, name)
		case curOK && prevOK && curRaw != prevRaw:
			changed = append(changed, name)
		case curOK:
			unchanged = append(unchanged, name)
		}
	}
	if len(changed) == 0 && len(cleared) == 0 {
		return "Draft unchanged since last served — nothing new to re-read."
	}
	sb.WriteString("Draft delta since last served (engine-rendered):\n")
	for _, name := range fields {
		switch {
		case slices.Contains(cleared, name):
			sb.WriteString("- " + name + ": (cleared)\n")
		case slices.Contains(changed, name):
			if isProse(spec, name) {
				if diff := textdiff.Unified(jsonToText(prev[name]), jsonToText(cur[name])); diff != "" {
					sb.WriteString("- " + name + " (diff against what was last served):\n" + diff + "\n")
				}
				continue
			}
			if isList(spec, name) {
				sb.WriteString(renderListDelta(name, prev[name], cur[name]))
				continue
			}
			sb.WriteString("- " + name + ": " + jsonToText(cur[name]) + "\n")
		}
	}
	if len(unchanged) > 0 {
		sb.WriteString("(unchanged: " + strings.Join(unchanged, ", ") + ")\n")
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

func renderDraftField(spec *Spec, name, raw string, full bool) string {
	if isProse(spec, name) {
		text := jsonToText(raw)
		if !full {
			text = textdiff.HeadTail(text, proseAckHead, proseAckTail)
		}
		return "- " + name + ":\n" + text + "\n"
	}
	if isList(spec, name) {
		var items []any
		if err := json.Unmarshal([]byte(raw), &items); err == nil {
			var parts []string
			for _, item := range items {
				parts = append(parts, compactJSON(item))
			}
			return "- " + name + ": " + strings.Join(parts, " · ") + "\n"
		}
	}
	return "- " + name + ": " + jsonToText(raw) + "\n"
}

// renderListDelta renders an item-level list change: items are compared by
// canonical bytes, additions marked +, removals -.
func renderListDelta(name, prevRaw, curRaw string) string {
	prevItems := listItems(prevRaw)
	curItems := listItems(curRaw)
	prevSet := make(map[string]int, len(prevItems))
	for _, item := range prevItems {
		prevSet[item]++
	}
	curSet := make(map[string]int, len(curItems))
	for _, item := range curItems {
		curSet[item]++
	}
	var sb strings.Builder
	sb.WriteString("- " + name + " (item delta):\n")
	for _, item := range curItems {
		if curSet[item] > prevSet[item] {
			sb.WriteString("  + " + item + "\n")
			curSet[item]--
		}
	}
	for _, item := range prevItems {
		if prevSet[item] > curSet[item] {
			sb.WriteString("  - " + item + "\n")
			prevSet[item]--
		}
	}
	return sb.String()
}

func listItems(raw string) []string {
	var items []any
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []string{raw}
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, compactJSON(item))
	}
	return out
}

func isProse(spec *Spec, name string) bool {
	decl, ok := spec.State[name]
	return ok && !decl.Type.List && decl.Type.Base == TypeProse
}

func isList(spec *Spec, name string) bool {
	decl, ok := spec.State[name]
	return ok && decl.Type.List
}

// jsonToText renders a canonical JSON value for the block: strings unquoted,
// everything else as its JSON form.
func jsonToText(raw string) string {
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err == nil {
		return s
	}
	return raw
}

func compactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
