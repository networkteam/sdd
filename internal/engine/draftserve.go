package engine

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/networkteam/sdd/internal/textdiff"
	"github.com/networkteam/sdd/internal/truncate"
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
// maxPieceBytes bounds each scaling piece — the adjust-round diff and the two
// list renders — with the cut routed out of band; whole prose (rehydrate) and
// the head/tail acknowledgment stay unbounded (d-tac-qwc).
func renderDraft(spec *Spec, fields []string, prev, cur map[string]string, full bool, maxPieceBytes int) (string, []truncate.Cut) {
	var sb strings.Builder
	var cuts []truncate.Cut
	if prev == nil {
		sb.WriteString("Draft as it stands (engine-rendered — what you confirm is this):\n")
		for _, name := range fields {
			raw, ok := cur[name]
			if !ok {
				continue
			}
			piece, cut := renderDraftField(spec, name, raw, full, maxPieceBytes)
			sb.WriteString(piece)
			if cut != nil {
				cuts = append(cuts, *cut)
			}
		}
		return strings.TrimSuffix(sb.String(), "\n"), cuts
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
		return "Draft unchanged since last served — nothing new to re-read.", nil
	}
	sb.WriteString("Draft delta since last served (engine-rendered):\n")
	for _, name := range fields {
		switch {
		case slices.Contains(cleared, name):
			sb.WriteString("- " + name + ": (cleared)\n")
		case slices.Contains(changed, name):
			if isProse(spec, name) {
				if diff := textdiff.Unified(jsonToText(prev[name]), jsonToText(cur[name])); diff != "" {
					bounded := truncate.Bytes(diff, maxPieceBytes, "")
					sb.WriteString("- " + name + " (diff against what was last served):\n" + bounded.Text + "\n")
					if cut := draftCut(bounded.Cut, name); cut != nil {
						cuts = append(cuts, *cut)
					}
				}
				continue
			}
			if isList(spec, name) {
				piece, cut := renderListDelta(name, prev[name], cur[name], maxPieceBytes)
				sb.WriteString(piece)
				if cut != nil {
					cuts = append(cuts, *cut)
				}
				continue
			}
			sb.WriteString("- " + name + ": " + jsonToText(cur[name]) + "\n")
		}
	}
	if len(unchanged) > 0 {
		sb.WriteString("(unchanged: " + strings.Join(unchanged, ", ") + ")\n")
	}
	return strings.TrimSuffix(sb.String(), "\n"), cuts
}

// draftCut stamps a non-clean cut with its draft part name; nil when clean.
func draftCut(cut truncate.Cut, field string) *truncate.Cut {
	if cut.Clean() {
		return nil
	}
	cut.Part = "draft:" + field
	return &cut
}

func renderDraftField(spec *Spec, name, raw string, full bool, maxPieceBytes int) (string, *truncate.Cut) {
	if isProse(spec, name) {
		text := jsonToText(raw)
		if !full {
			text = textdiff.HeadTail(text, proseAckHead, proseAckTail)
		}
		return "- " + name + ":\n" + text + "\n", nil
	}
	if isList(spec, name) {
		var items []any
		if err := json.Unmarshal([]byte(raw), &items); err == nil {
			var parts []string
			for _, item := range items {
				parts = append(parts, compactJSON(item))
			}
			bounded := truncate.Items(parts, func(s string) int { return len(s) + len(" · ") }, maxPieceBytes, "")
			return "- " + name + ": " + strings.Join(bounded.Items, " · ") + "\n", draftCut(bounded.Cut, name)
		}
	}
	return "- " + name + ": " + jsonToText(raw) + "\n", nil
}

// renderListDelta renders an item-level list change: items are compared by
// canonical bytes, additions marked +, removals -.
func renderListDelta(name, prevRaw, curRaw string, maxPieceBytes int) (string, *truncate.Cut) {
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
	var lines []string
	for _, item := range curItems {
		if curSet[item] > prevSet[item] {
			lines = append(lines, "  + "+item+"\n")
			curSet[item]--
		}
	}
	for _, item := range prevItems {
		if prevSet[item] > curSet[item] {
			lines = append(lines, "  - "+item+"\n")
			prevSet[item]--
		}
	}
	bounded := truncate.Items(lines, func(s string) int { return len(s) }, maxPieceBytes, "")
	return "- " + name + " (item delta):\n" + strings.Join(bounded.Items, ""), draftCut(bounded.Cut, name)
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
