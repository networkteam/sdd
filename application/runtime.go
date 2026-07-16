package application

import (
	"fmt"
	"time"
)

// ProjectRuntime owns the immutable ports and project configuration resolved
// for one application operation.
type ProjectRuntime struct {
	options ProjectRuntimeOptions
}

type ProjectRuntimeOptions struct {
	Project      ProjectRef
	Language     string
	Dependencies []string
	Graph        GraphStore
	Sessions     SessionStore
	StagedBlobs  StagedBlobStore
	Embeddings   EmbeddingExecutor
	SearchIndex  SearchIndexStore
	LLM          LLMExecutor
	Finalizers   []MutationFinalizer
	Now          func() time.Time
	// ExcludeEmbeddedFromIndex mirrors the CLI's excludeEmbedded semantics for
	// the vector index: connected-repo runtimes set it so binary-shipped base
	// entries embed once per machine (in the base store) rather than once per
	// connected repo. The base runtime leaves it false — its store includes
	// embedded entries. The rule is applied identically at index and read time.
	ExcludeEmbeddedFromIndex bool
}

func NewProjectRuntime(options ProjectRuntimeOptions) (*ProjectRuntime, error) {
	if options.Project.ID == "" {
		return nil, fmt.Errorf("sdd: project ID is required")
	}
	if options.Graph == nil {
		return nil, fmt.Errorf("sdd: GraphStore is required")
	}
	if options.Sessions == nil {
		return nil, fmt.Errorf("sdd: SessionStore is required")
	}
	if options.StagedBlobs == nil {
		return nil, fmt.Errorf("sdd: StagedBlobStore is required")
	}
	if (options.Embeddings == nil) != (options.SearchIndex == nil) {
		return nil, fmt.Errorf("sdd: EmbeddingExecutor and SearchIndexStore must be configured together")
	}
	if options.LLM == nil {
		return nil, fmt.Errorf("sdd: LLMExecutor is required")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &ProjectRuntime{options: options}, nil
}

func (r *ProjectRuntime) Project() ProjectRef {
	if r == nil {
		return ProjectRef{}
	}
	return r.options.Project
}
