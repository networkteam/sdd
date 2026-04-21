package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
)

// errSchemaIncompatible is returned by the write gate when the binary cannot
// safely mutate the graph. The underlying reason is already printed to
// stderr via the presenter, so this value exists only to signal a non-zero
// exit up through urfave/cli's Run loop.
var errSchemaIncompatible = errors.New("graph schema incompatible with this binary")

// withWriteGate wraps an action function with a schema-compatibility check.
// Applied to every write command (sdd new, sdd wip start|done, sdd
// summarize); reads bypass the check entirely.
//
// When .sdd/meta.json is absent (pre-init graphs) or the binary is a dev
// build, the gate passes through without consulting the finder so local
// development isn't blocked by a released graph's pins.
func withWriteGate(action cli.ActionFunc) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		sddDir, err := resolveSDDDir()
		if err != nil {
			// No .sdd/ — the inner action will fail naturally with the
			// same message. Don't second-guess discovery here.
			return action(ctx, cmd)
		}

		f := newReadFinder()
		res, err := f.SchemaStatus(ctx, query.SchemaStatusQuery{
			SDDDir:              sddDir,
			BinaryVersion:       version,
			BinarySchemaVersion: model.CurrentGraphSchemaVersion,
		})
		if err != nil {
			return fmt.Errorf("schema check: %w", err)
		}
		if !res.Compatibility.Compatible {
			presenters.RenderSchemaError(os.Stderr, res.Compatibility)
			return errSchemaIncompatible
		}
		return action(ctx, cmd)
	}
}
