package application_test

import (
	"context"
	"errors"
	"fmt"

	sdd "github.com/networkteam/sdd/pkg/application"
)

// Existing branch fixtures share their adapter bookkeeping between read and
// write ports. Read-only authorization is exercised separately.
type branchReadFixture struct {
	sdd.GraphStore
	targets sdd.TargetAcquirer
	project sdd.ProjectID
}

func (s branchReadFixture) AcquireSnapshot(ctx context.Context, q sdd.SnapshotReadQuery) (*sdd.AcquiredSnapshot, error) {
	if q.Branch == "" {
		if reader, ok := s.GraphStore.(sdd.SnapshotReader); ok {
			return reader.AcquireSnapshot(ctx, q)
		}
		snapshot, err := s.Current(ctx)
		return &sdd.AcquiredSnapshot{Snapshot: snapshot, Attachments: s.GraphStore, Release: func() error { return nil }}, err
	}
	target, err := s.targets.Acquire(ctx, sdd.MutationTarget{Project: s.project, Branch: q.Branch})
	if err != nil {
		return nil, err
	}
	if target == nil || target.Graph == nil || target.Release == nil {
		return nil, fmt.Errorf("incomplete fixture source")
	}
	// The fixture selected the branch above, independently of the scoped store.
	q.Branch = ""
	if reader, ok := target.Graph.(sdd.SnapshotReader); ok {
		source, err := reader.AcquireSnapshot(ctx, q)
		if err != nil {
			return nil, errors.Join(err, target.Release())
		}
		release := source.Release
		source.Release = func() error { return errors.Join(release(), target.Release()) }
		return source, nil
	}
	snapshot, err := target.Graph.Current(ctx)
	return &sdd.AcquiredSnapshot{Snapshot: snapshot, Attachments: target.Graph, Release: target.Release}, err
}
