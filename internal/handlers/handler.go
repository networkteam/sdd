// Package handlers is the only place framework Go code is allowed to have
// side effects. Handlers are the write side of CQRS: they accept a domain
// command, do any validation against loaded state, perform IO (git, disk,
// pre-flight via a finder dependency), and return only errors. Results flow
// back through callbacks defined on the command struct.
package handlers

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// Reader is the handler-side view of the finder. It bundles every read
// operation handlers need (graph loading, pre-flight, WIP markers, install
// state). Defined here (rather than imported as the concrete *finders.Finder)
// so consumers can substitute fakes in tests — standard "accept interfaces,
// return structs" Go pattern. *finders.Finder satisfies this interface.
type Reader interface {
	// CurrentGraph returns the current graph through the read-side GraphSource
	// seam (a fresh source per call — per-command lifetime, no retained cache).
	// Handlers route through it rather than loading directly so cross-repo
	// assembly reaches the write path for free at the one seam.
	CurrentGraph(dir string) (*model.Graph, error)
	LoadWIPMarkers(graphDir string) ([]*model.WIPMarker, error)
	Preflight(ctx context.Context, q query.PreflightQuery) (*query.PreflightResult, error)
	SkillStatus(ctx context.Context, q query.SkillStatusQuery) (*query.SkillStatusResult, error)
}

// Committer performs a git commit of the given paths with the given message.
// Injected so tests can record or no-op commits.
type Committer interface {
	Commit(message string, paths ...string) error
}

// Brancher performs git branch operations. Injected so tests can fake
// checkout/branch deletion without touching real git.
type Brancher interface {
	Checkout(branch string, create bool) error
	BranchMerged(branch string) bool
	DeleteBranch(branch string, force bool) error
}

// Mover renames a path in the working tree and the git index as one operation
// — the semantics of `git mv`. Injected so tests can fake moves without
// shelling out. Used by the rewrite handler when the target entry changes
// type and its file needs a new on-disk location.
type Mover interface {
	Move(src, dst string) error
}

// Puller is the git surface for `sdd sync --pull`: a merge-only pull that
// never rewrites the shared graph's history. Injected so tests can fake the
// working-tree state and the pull without shelling out to git.
type Puller interface {
	// IsClean reports whether the working tree has no uncommitted changes
	// (a `git status --porcelain` with empty output).
	IsClean(ctx context.Context) (bool, error)
	// MergePull runs a merge-only pull (`git pull --no-rebase`) and returns
	// git's combined output. A non-nil error means the pull failed.
	MergePull(ctx context.Context) (string, error)
}

// Handler holds injected dependencies shared across command methods.
// Each public method corresponds to one command and lives in its own file
// (handler_new_entry.go, etc.).
type Handler struct {
	graphDir  string
	sddDir    string // path to .sdd/ directory; required for commands that write tmp files
	reader    Reader
	llmRunner llm.Runner
	committer Committer
	brancher  Brancher
	mover     Mover
	puller    Puller
	stderr    io.Writer
	now       func() time.Time
}

// Options configures a new Handler. Zero-valued fields get sensible defaults.
type Options struct {
	GraphDir  string
	SDDDir    string // path to .sdd/ directory; required for commands that write tmp files
	Reader    Reader
	LLMRunner llm.Runner
	Committer Committer
	Brancher  Brancher
	Mover     Mover
	Puller    Puller
	Stderr    io.Writer
	Now       func() time.Time
}

// New constructs a Handler with the given options.
func New(opts Options) *Handler {
	h := &Handler{
		graphDir:  opts.GraphDir,
		sddDir:    opts.SDDDir,
		reader:    opts.Reader,
		llmRunner: opts.LLMRunner,
		committer: opts.Committer,
		brancher:  opts.Brancher,
		mover:     opts.Mover,
		puller:    opts.Puller,
		stderr:    opts.Stderr,
		now:       opts.Now,
	}
	if h.stderr == nil {
		h.stderr = os.Stderr
	}
	if h.now == nil {
		h.now = time.Now
	}
	return h
}
