package query

import "github.com/networkteam/sdd/internal/model"

// WritingGuideQuery captures intent to check a draft entry against the
// writing guide — the isolation-scoped LLM check of the capture-guide
// architecture (d-cpt-20r). Pure read intent; the runner dependency injected
// into the finder handles the LLM call, and the draft is judged without any
// graph or dialogue context.
type WritingGuideQuery struct {
	Entry *model.Entry
}

// GuideSeverity weighs a writing-guide finding. Nothing gates on it — the
// guide's findings are drafting input, never a block: substantive means the
// drafting dialogue should take the finding up, minor means folding it in or
// ignoring it is the agent's call.
type GuideSeverity string

const (
	GuideSubstantive GuideSeverity = "substantive"
	GuideMinor       GuideSeverity = "minor"
)

// GuideFinding is a single writing-guide observation. Reasoning leads — it is
// the finding's work, carried to the drafting agent so a redraft can act on
// the why rather than obey a verdict; axis, repair, and severity are its
// conclusions. JSON tags define the document form the engine store and serves
// carry.
type GuideFinding struct {
	Reasoning string        `json:"reasoning"`
	Axis      string        `json:"axis"`
	Quote     string        `json:"quote"`
	Repair    string        `json:"repair"`
	Severity  GuideSeverity `json:"severity"`
}

// WritingGuideResult holds all findings from a writing-guide run. An empty
// Findings slice means the draft passed clean.
type WritingGuideResult struct {
	Findings []GuideFinding
}
