package application_test

import (
	"context"
	"fmt"

	sdd "github.com/networkteam/sdd/pkg/application"
)

type indexingDiscoveryFinalizer struct {
	retainFinalizedSource func(context.Context, sdd.AppliedMutation) (string, error)
	enqueueDiscovery      func(context.Context, sdd.ProjectID, string, []string) error
}

func (indexingDiscoveryFinalizer) Name() string { return "index-discovery" }
func (f indexingDiscoveryFinalizer) Finalize(ctx context.Context, mutation sdd.AppliedMutation) error {
	ids, err := mutation.AffectedEntryIDs()
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	revision, err := f.retainFinalizedSource(ctx, mutation)
	if err != nil {
		return err
	}
	if revision == "" {
		return fmt.Errorf("finalized source revision is required")
	}
	return f.enqueueDiscovery(ctx, mutation.Project, revision, ids)
}

func ExampleAppliedMutation_AffectedEntryIDs() {
	mutation := sdd.AppliedMutation{Project: "example", Revision: "workspace-revision", Batch: sdd.MutationBatch{
		Attachments: []sdd.AttachmentMaterialization{{LogicalPath: "2026/01/01-100000-s-tac-aaa/evidence.md"}},
	}}
	// The consumer's durable write/recovery protocol makes these effects
	// idempotent and retries a crash between source finalization and enqueueing.
	finalizer := indexingDiscoveryFinalizer{
		retainFinalizedSource: func(context.Context, sdd.AppliedMutation) (string, error) { return "retained-git-revision", nil },
		enqueueDiscovery: func(_ context.Context, project sdd.ProjectID, revision string, ids []string) error {
			fmt.Println(project, revision, ids)
			return nil
		},
	}
	if err := finalizer.Finalize(context.Background(), mutation); err != nil {
		panic(err)
	}
	// Output: example retained-git-revision [20260101-100000-s-tac-aaa]
}
