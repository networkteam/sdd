package application_test

import (
	"context"

	sdd "github.com/networkteam/sdd/pkg/application"
)

func ExampleSearchTarget_local() {
	// The composition root supplies runtimes for the projects it authorizes.
	var runtimes map[sdd.ProjectID]*sdd.ProjectRuntime
	options := sdd.ApplicationOptions{
		PrepareSearch: func(ctx context.Context, target sdd.SearchTarget) error {
			for requirement, err := range target.Entries(ctx) {
				if err != nil {
					return err
				}
				if requirement.Published {
					continue
				}
				runtime := runtimes[requirement.Entry.Version.Namespace.Project]
				if err := runtime.IndexSearchEntry(ctx, sdd.IndexSearchEntryCmd{Entry: requirement.Entry}); err != nil {
					return err
				}
			}
			return nil
		},
	}
	_ = options // Pass to NewApplication with the other required capabilities.
}

func ExampleSearchTarget_externalConsumer() {
	// These functions belong to the consumer's durable scheduling protocol.
	var enqueue func(context.Context, sdd.SearchEntryDescriptor, sdd.SearchDiscoveryCursor) error
	var waitWithinBudget func(context.Context, []sdd.SearchTargetProject) error
	options := sdd.ApplicationOptions{
		PrepareSearch: func(ctx context.Context, target sdd.SearchTarget) error {
			for requirement, err := range target.Entries(ctx) {
				if err != nil {
					return err
				}
				if requirement.Published {
					continue
				}
				if err := enqueue(ctx, requirement.Entry, requirement.Cursor); err != nil {
					return err
				}
			}
			// Ordinary budget expiry returns nil. Parent cancellation and preparation
			// failures return errors. SDD determines coverage after this returns.
			return waitWithinBudget(ctx, target.Projects())
		},
	}
	_ = options
}
