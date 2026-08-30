package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DefaultLLMTimeout caps an LLM call when the host configures no timeout.
const DefaultLLMTimeout = 2 * time.Minute

// ProjectRuntime owns the immutable ports and project configuration resolved
// for one application operation.
type ProjectRuntime struct {
	options ProjectRuntimeOptions
}

type ProjectRuntimeOptions struct {
	Project       ProjectRef
	DefaultBranch string
	Language      string
	Dependencies  []string
	Graph         GraphStore
	Targets       TargetAcquirer
	Branches      BranchValidator
	Recovery      RecoveryAuthorizer
	Sessions      SessionStore
	StagedBlobs   StagedBlobStore
	Embeddings    EmbeddingExecutor
	SearchIndex   SearchIndexStore
	LLM           LLMExecutor
	// LLMTimeout caps each individual LLM call. Zero means DefaultLLMTimeout.
	// One value covers every purpose; a slow provider that needs longer for the
	// writing guide needs it for pre-flight too.
	LLMTimeout time.Duration
	Finalizers []MutationFinalizer
	Now        func() time.Time
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
	options.Sessions = legacyEndStore{options.Sessions}
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
	if options.LLMTimeout <= 0 {
		options.LLMTimeout = DefaultLLMTimeout
	}
	if options.Targets == nil && options.DefaultBranch != "" {
		options.Targets = FixedTargetAcquirer{
			Target: MutationTarget{Project: options.Project.ID, Branch: options.DefaultBranch},
			Graph:  options.Graph, Finalizers: options.Finalizers,
		}
	}
	return &ProjectRuntime{options: options}, nil
}

func (r *ProjectRuntime) Project() ProjectRef {
	if r == nil {
		return ProjectRef{}
	}
	return r.options.Project
}

func (r *ProjectRuntime) defaultMutationTarget() (MutationTarget, error) {
	target := MutationTarget{Project: r.options.Project.ID, Branch: strings.TrimSpace(r.options.DefaultBranch)}
	if err := target.Validate(r.options.Project.ID); err != nil {
		return MutationTarget{}, fmt.Errorf("sdd: no concrete default mutation branch is configured: %w", err)
	}
	return target, nil
}

func (r *ProjectRuntime) acquire(ctx context.Context, target MutationTarget) (*AcquiredTarget, error) {
	if err := target.Validate(r.options.Project.ID); err != nil {
		return nil, markTargetAcquisitionError(target, err)
	}
	if r.options.Targets == nil {
		return nil, markTargetAcquisitionError(target, &ApplicationError{Code: ErrorWriteDenied, Message: "project has no mutation target acquirer"})
	}
	acquired, err := r.options.Targets.Acquire(ctx, target)
	if err != nil {
		return nil, markTargetAcquisitionError(target, err)
	}
	if err := acquired.validate(target); err != nil {
		if acquired != nil && acquired.Release != nil {
			return nil, markTargetAcquisitionError(target, errors.Join(err, acquired.Release()))
		}
		return nil, markTargetAcquisitionError(target, err)
	}
	return acquired, nil
}
