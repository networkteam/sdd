package finders

import (
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/viewlayout"
)

// LiveViewVocabulary assembles the executor's live layout vocabulary from the
// packages that own each part of the language.
func LiveViewVocabulary() viewlayout.Vocabulary {
	return viewlayout.Vocabulary{
		Functions:    ViewFunctionNames(),
		Renders:      ViewRenderNames(),
		Algorithms:   ViewRankAlgorithmNames(),
		Decays:       model.DecayNames(),
		Macros:       query.MacroNames(),
		LayoutMacros: query.LayoutMacroNames(),
	}
}
