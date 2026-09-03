package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/llm/embed"
)

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
	// Embedder and LLM are the two model dependencies, each a pkg/llm port
	// injected as an instance that arrives already composed — observed,
	// bounded, and rate-limited by the host's decorators. Routing, deadlines,
	// and recording are the host's composition duty (20260830-234501-d-cpt-q6n,
	// 20260902-114838-d-tac-cov); application contributes only the facts it
	// alone holds: the Purpose and the prompts, or the texts.
	Embedder    embed.Embedder
	SearchIndex SearchIndexStore
	LLM         llm.Runner
	Finalizers  []MutationFinalizer
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
	if (options.Embedder == nil) != (options.SearchIndex == nil) {
		return nil, fmt.Errorf("sdd: Embedder and SearchIndexStore must be configured together")
	}
	if options.LLM == nil {
		return nil, fmt.Errorf("sdd: LLM runner is required")
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
		return MutationTarget{}, fmt.Errorf("sdd: project %s has no concrete default mutation branch configured: %w", r.options.Project.ID, err)
	}
	return target, nil
}

func (r *ProjectRuntime) acquire(ctx context.Context, target MutationTarget) (*AcquiredTarget, error) {
	if err := target.Validate(r.options.Project.ID); err != nil {
		return nil, markTargetAcquisitionError(target, err)
	}
	if r.options.Targets == nil {
		return nil, markTargetAcquisitionError(target, &ApplicationError{Code: ErrorWriteDenied, Message: fmt.Sprintf("project %s has no mutation target acquirer for branch %q", target.Project, target.Branch)})
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
