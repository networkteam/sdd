package query

import (
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/application/types"
)

// The lint vocabulary is defined in pkg/application/types — the exported
// surface names it, so the definitions live in the cycle-free public leaf
// (s-tac-ah2). IndexLintQuery stays here: it carries model config and is a
// shell concern, never part of the exported surface.
type (
	LintQuery    = types.LintQuery
	LintSeverity = types.LintSeverity
	LintFinding  = types.LintFinding
	LintResult   = types.LintResult
)

const (
	LintError    = types.LintError
	LintAdvisory = types.LintAdvisory
)

// IndexLintQuery captures intent to surface search-index health: the
// resolved embedding config (flag/config merging is the shell's job) and
// the local index location. Processed by Finder.IndexLint into index
// findings on a LintResult.
type IndexLintQuery struct {
	Embedding model.EmbeddingConfig
	IndexDir  string
}
