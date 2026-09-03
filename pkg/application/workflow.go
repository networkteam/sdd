package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/basefacts"
	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/textpatch"
	"github.com/networkteam/sdd/internal/truncate"
	"github.com/networkteam/sdd/pkg/application/types"
)

const (
	WorkflowEventCode      = "workflow_event"
	workflowStagedBlobCode = "workflow_staged_blob"
	// workflowInstanceProjectCode records the project an instance targets when
	// it is not the home project — an application-owned fact the engine never
	// sees, keyed by instance like the branch binding is keyed by session
	// (d-cpt-yjc).
	workflowInstanceProjectCode = "workflow_instance_project"
	BranchBoundEventCode        = "branchBound"
	BranchClearedEventCode      = "branchCleared"
	DefaultShellCanonical       = "user-dialogue"
	WorkflowMaxLabelLength      = 120
	ExecutionForkPreferred      = "fork-preferred"
)

type WorkflowOpenRequest struct {
	ClientName    string
	ClientVersion string
	Shell         string
	Label         string
}

// WorkflowResumeRequest names the session to load by its ID and the client
// loading it. Possession of the ID within the principal's scope is the whole
// authorization (d-cpt-aen).
type WorkflowResumeRequest struct {
	SessionID     SessionID
	ClientName    string
	ClientVersion string
}

type WorkflowStartRequest struct {
	Canonical string
	Params    map[string]any
	Label     string
	Parent    string
	// Project pins the project the new instance targets. Empty leaves it to
	// the dispatching parent's project, else the home project.
	Project ProjectID
}

type WorkflowAdvanceRequest struct {
	Instance string
	Report   map[string]any
	Label    string
}

type WorkflowChooserOption struct {
	Choice  string
	Collect []string
}

type WorkflowChooser struct {
	Chooser string
	Kind    ChooserKind
	Options []WorkflowChooserOption
}

type WorkflowServe struct {
	Session SessionID
	// Project is the project the served instance targets — the home project
	// unless the move was started in a dependency (d-cpt-yjc).
	Project        ProjectID
	Branch         string
	Instance       string
	Procedure      string
	Status         string
	Step           string
	Goal           string
	Instructions   string
	Missing        []string
	ReportSchema   map[string]any
	PendingChooser *WorkflowChooser
	Execution      string
	Produced       map[string]any
	Diagnostics    []string
	// InstructionLanes are the unit's rendered lanes in order — what the MCP
	// layer dedups independently; Instructions is their join plus diagnostics.
	InstructionLanes []types.ServeLane
	// Sizes is the engine's per-part byte accounting for this serve, read by
	// the serve-budget measurement (d-tac-qwc).
	Sizes []types.PartSize
	Base  *WorkflowServe
	// Collected is the instance's already-gathered param and state values,
	// projected only onto resume serves so a newly attached or reoriented
	// agent sees what this instance holds — the anchor, chosen scope, and
	// reported judgments that persist across a handover (d-cpt-0tm). Empty on
	// door, next, and base-junction serves, which stay unchanged.
	Collected map[string]any
}

// ReminderInstructions composes the short reminder used when a host has
// already served this instruction unit, while retaining any gate diagnostics.
// The stub assumes the caller still holds the earlier full text; if a context
// compaction dropped it, the breadcrumb names the one-shot escape so an
// amnesiac agent is not left following instructions it no longer has.
func (s *WorkflowServe) ReminderInstructions() string {
	if s == nil {
		return ""
	}
	reminder := fmt.Sprintf("(step %s instructions were served earlier this session — follow them; goal: %s. Lost them to a context compaction? resume_session with this session's handle re-serves this position in full.)", s.Step, s.Goal)
	return engine.ComposeInstructions(reminder, s.Diagnostics)
}

// ComposeInstructions joins host-recomposed unit text (e.g. the deduped lane
// subset) with this serve's diagnostics — the engine's one composition rule
// applied host-side.
func (s *WorkflowServe) ComposeInstructions(unitText string) string {
	return engine.ComposeInstructions(unitText, s.Diagnostics)
}

type WorkflowInstanceSummary struct {
	Instance  string
	Procedure string
	Step      string
}

type WorkflowSessionSummary struct {
	Session      SessionID
	Label        string
	Participant  string
	Branch       string
	Anchor       string
	Open         []WorkflowInstanceSummary
	LastActivity time.Time
	Attachment   *Attachment
	Active       bool
}

type WorkflowResumeResult struct {
	Session      SessionID
	Participant  string
	Label        string
	Branch       string
	Open         []WorkflowServe
	Instructions string
}

type WorkflowAbandonResult struct {
	Abandoned   bool
	Session     SessionID
	Label       string
	Discarded   []WorkflowInstanceSummary
	HeldMarkers []string
	Base        *WorkflowServe
}

type WorkflowParkResult struct {
	Instance  string
	Procedure string
	Step      string
	Base      *WorkflowServe
}

// WorkflowSession is a protocol-neutral, durable engine session. It stores
// no authorization proof: every operation receives the current request
// identity and resolves access again before touching project state.
type WorkflowSession struct {
	app      *Application
	project  ProjectID
	identity RequestIdentity
	ctx      context.Context
	binding  SessionBinding
	branch   string
	engine   *engine.Engine
	session  *engine.Session
	graphs   *workflowGraphs
	sink     *workflowSink
	shell    string
	staged   map[string]string
	// instanceProjects holds the recorded per-instance targets; an instance
	// absent here derives its project from its parent, then the home.
	instanceProjects map[string]ProjectID
	// startProject is the project the instance being started targets, which
	// the sink records in the same append as the engine's start event.
	startProject ProjectID
}

func (a *Application) OpenWorkflow(ctx context.Context, identity RequestIdentity, project ProjectID, request WorkflowOpenRequest) (*WorkflowSession, *WorkflowServe, error) {
	info, err := a.Info(ctx, identity, project, InfoRequest{})
	if err != nil {
		return nil, nil, err
	}
	if info.Participant == "" {
		return nil, nil, fmt.Errorf("sdd: resolved participant is required to open a workflow")
	}
	principal, runtime, err := a.resolve(ctx, identity, info.Project.ID, AccessRead)
	if err != nil {
		return nil, nil, err
	}
	id := newWorkflowSessionID(time.Now())
	now := a.now().UTC().Round(0)
	metadata := SessionMetadata{
		ID: id, Subject: principal.Subject,
		Project: runtime.options.Project.ID, Participant: info.Participant, UpdatedAt: now,
		Attachment: newAttachment(principal.Subject, request.ClientName, request.ClientVersion, now),
	}
	created, err := a.sessions.Create(ctx, metadata)
	if err != nil {
		return nil, nil, err
	}
	w, err := a.newWorkflow(ctx, identity, info.Project.ID, sessionBindingFrom(created), created.Metadata.Branch)
	if err != nil {
		return nil, nil, err
	}
	w.session = w.engine.NewSession(string(id), info.Participant, w.sink)
	canonical := strings.TrimSpace(request.Shell)
	if canonical == "" {
		canonical = DefaultShellCanonical
	}
	spec, err := w.loadProcedure(canonical)
	if err != nil {
		return nil, nil, err
	}
	if spec.Class != model.ProcedureClassShell {
		return nil, nil, fmt.Errorf("%q is a move, not a session shell", canonical)
	}
	serve, err := w.session.Start(spec, nil, "")
	if err != nil {
		return nil, nil, err
	}
	w.shell = serve.Instance
	if err := w.setLabel(request.Label); err != nil {
		return nil, nil, err
	}
	return w, w.publicServe(serve), nil
}

// LoadWorkflow loads an existing session by ID: the session's own record
// names its home project, the composition's continuation policy and current
// membership in that project gate the load, and an ended session is refused.
// Possession of the ID is the authorization — no consent, no takeover
// (d-cpt-aen). Loading stamps which client attached and when, the record
// staleness is derived from.
func (a *Application) LoadWorkflow(ctx context.Context, identity RequestIdentity, request WorkflowResumeRequest) (*WorkflowSession, error) {
	if request.SessionID == "" {
		return nil, fmt.Errorf("sdd: session ID is required")
	}
	principal, runtime, stored, err := a.resolveSession(ctx, identity, request.SessionID, AccessRead)
	if err != nil {
		return nil, err
	}
	// An ended dialogue is kept to be read, never carried on: loading would
	// replay it and auto-start a fresh shell, the revival d-tac-k4q refuses.
	if end := stored.Metadata.Ended; end != nil {
		return nil, endedSessionError(stored.Metadata, *end)
	}
	stored, err = a.stampAttachment(ctx, principal, request, stored)
	if err != nil {
		return nil, err
	}
	w, err := a.newWorkflow(ctx, identity, runtime.options.Project.ID, sessionBindingFrom(stored), stored.Metadata.Branch)
	if err != nil {
		return nil, err
	}
	if err := w.restoreStagedBlobs(stored.Events); err != nil {
		return nil, err
	}
	if err := w.restoreInstanceProjects(stored.Events); err != nil {
		return nil, err
	}
	events, err := decodeWorkflowEvents(stored.Events)
	if err != nil {
		return nil, err
	}
	resolve := func(canonical string) (*engine.Spec, error) { return w.loadProcedure(canonical) }
	w.session, err = w.engine.ReplaySession(string(request.SessionID), stored.Metadata.Participant, events, resolve, w.sink)
	if err != nil {
		return nil, fmt.Errorf("replaying session %s: %w", request.SessionID, err)
	}
	if err := w.ensureShell(); err != nil {
		return nil, err
	}
	return w, nil
}

// ResumeWorkflow loads a session and serves its current position: every
// running instance at its step with the schema to continue it.
func (a *Application) ResumeWorkflow(ctx context.Context, identity RequestIdentity, request WorkflowResumeRequest) (*WorkflowSession, WorkflowResumeResult, error) {
	w, err := a.LoadWorkflow(ctx, identity, request)
	if err != nil {
		return nil, WorkflowResumeResult{}, err
	}
	result, err := w.resumeResult()
	return w, result, err
}

// stampAttachment records the loading client as the session's last attachment.
// A lost CAS race against a concurrent writer reloads and retries once; the
// stamp is a record, so a second loss surfaces as the conflict it is.
func (a *Application) stampAttachment(ctx context.Context, principal Principal, request WorkflowResumeRequest, stored StoredSession) (StoredSession, error) {
	for attempt := 0; ; attempt++ {
		now := a.now().UTC().Round(0)
		metadata := stored.Metadata
		metadata.Attachment = newAttachment(principal.Subject, request.ClientName, request.ClientVersion, now)
		metadata.UpdatedAt = now
		version, err := a.sessions.Append(ctx, request.SessionID, stored.Version, SessionAppend{Metadata: &metadata})
		if err == nil {
			stored.Metadata = metadata
			stored.Version = version
			return stored, nil
		}
		var appErr *ApplicationError
		if attempt > 0 || !errors.As(err, &appErr) || appErr.Code != ErrorSessionConflict {
			return StoredSession{}, err
		}
		if stored, err = a.sessions.Load(ctx, request.SessionID); err != nil {
			return StoredSession{}, err
		}
		if err := validateStoredSession(stored); err != nil {
			return StoredSession{}, err
		}
		if end := stored.Metadata.Ended; end != nil {
			return StoredSession{}, endedSessionError(stored.Metadata, *end)
		}
	}
}

func newAttachment(subject, clientName, clientVersion string, now time.Time) *Attachment {
	return &Attachment{Subject: subject, ClientName: clientName, ClientVersion: clientVersion, LastActivity: now}
}

func (a *Application) newWorkflow(ctx context.Context, identity RequestIdentity, project ProjectID, binding SessionBinding, branch string) (*WorkflowSession, error) {
	w := &WorkflowSession{
		app: a, project: project, identity: identity, ctx: ctx, binding: binding, branch: branch,
		staged: map[string]string{}, instanceProjects: map[string]ProjectID{},
	}
	w.graphs = &workflowGraphs{workflow: w}
	w.sink = &workflowSink{workflow: w}
	registry, err := w.buildRegistry()
	if err != nil {
		return nil, err
	}
	w.engine = engine.New(registry, w.graphs, engine.WithTemplateValues(map[string]any{
		"kindAuthoringFactIDs": basefacts.AuthoringFactIDs(),
	}))
	return w, nil
}

func (w *WorkflowSession) ID() SessionID { return w.binding.SessionID }

// ServedBefore reports whether the session's consumer already holds a block
// with this content hash, derived from the session ledger (d-cpt-aen).
func (w *WorkflowSession) ServedBefore(hash string) bool {
	return w.session.ServedBefore(hash)
}

// RecordServed logs the content hashes of blocks just served in full. A
// finished session records nothing: its last serve is the one that ended it.
func (w *WorkflowSession) RecordServed(ctx context.Context, identity RequestIdentity, hashes []string) error {
	if len(hashes) == 0 || w.Finished() {
		return nil
	}
	w.setOperation(ctx, identity)
	w.session.RecordServed(hashes)
	return w.session.SinkErr()
}

// Reorient records the consumer's request for the session's position and
// resets the served set, so the position re-serves in full.
func (w *WorkflowSession) Reorient(ctx context.Context, identity RequestIdentity) error {
	if w.Finished() {
		return nil
	}
	w.setOperation(ctx, identity)
	w.session.Reorient()
	return w.session.SinkErr()
}

func (w *WorkflowSession) Project() ProjectID { return w.project }

func (w *WorkflowSession) Binding() SessionBinding { return w.binding }

func (w *WorkflowSession) Branch() string { return w.branch }

func (w *WorkflowSession) setBranch(branch string) {
	if w.branch != branch && w.graphs != nil {
		w.graphs.Invalidate()
	}
	w.branch = branch
}

func (w *WorkflowSession) OpenInstances() []WorkflowInstanceSummary {
	var result []WorkflowInstanceSummary
	for _, instance := range w.session.Instances() {
		if instance.Status == engine.StatusRunning && instance.ID != w.shell {
			result = append(result, WorkflowInstanceSummary{Instance: instance.ID, Procedure: instance.Spec.Canonical, Step: instance.Step})
		}
	}
	return result
}

func (w *WorkflowSession) IsShell(instance string) bool { return instance != "" && instance == w.shell }

// Finished reports whether this dialogue is over: the shell has left running,
// which is the act that wrote the terminal record. A finished session is spent —
// the door opens a new one rather than re-serving it, and no move may carry it on.
func (w *WorkflowSession) Finished() bool {
	inst, ok := w.session.Instance(w.shell)
	return ok && inst.Status != engine.StatusRunning
}

// finishedError refuses a move against a spent dialogue, carrying the way on
// instead of reviving the session (d-tac-k4q).
func (w *WorkflowSession) finishedError() error {
	return &ApplicationError{Code: ErrorSessionEnded, Message: fmt.Sprintf("session %s has ended. %s", w.ID(), NewSessionNote)}
}

func (w *WorkflowSession) Start(ctx context.Context, identity RequestIdentity, request WorkflowStartRequest) (*WorkflowServe, error) {
	if strings.TrimSpace(request.Canonical) == "" {
		return nil, fmt.Errorf("canonical is required")
	}
	if w.Finished() {
		return nil, w.finishedError()
	}
	w.setOperation(ctx, identity)
	w.graphs.Invalidate()
	spec, err := w.loadProcedure(request.Canonical)
	if err != nil {
		return nil, err
	}
	if spec.Class == model.ProcedureClassShell {
		return nil, fmt.Errorf("%q is a session shell — sessions open through start_session, not start_procedure", request.Canonical)
	}
	if err := w.setLabel(request.Label); err != nil {
		return nil, err
	}
	parent := request.Parent
	if parent == "" {
		parent = w.shell
	}
	// The target is settled before the instance exists, so a project the
	// session may not work in refuses the start rather than leaving an
	// instance behind that derives the wrong one.
	project, err := w.startProjectFor(request.Project)
	if err != nil {
		return nil, err
	}
	w.startProject = project
	serve, err := w.session.Start(spec, request.Params, parent)
	w.startProject = ""
	if err != nil {
		return nil, err
	}
	if err := w.session.SinkErr(); err != nil {
		return nil, err
	}
	return w.publicServe(serve), nil
}

func (w *WorkflowSession) Advance(ctx context.Context, identity RequestIdentity, request WorkflowAdvanceRequest) (*WorkflowServe, error) {
	if request.Instance == "" || len(request.Report) == 0 {
		return nil, fmt.Errorf("instance and report are required")
	}
	if w.Finished() {
		return nil, w.finishedError()
	}
	w.setOperation(ctx, identity)
	w.graphs.Invalidate()
	if err := w.setLabel(request.Label); err != nil {
		return nil, err
	}
	var (
		serve *engine.Serve
		err   error
	)
	chooser, hasChooser := request.Report["chooser"].(string)
	choice, hasChoice := request.Report["choice"].(string)
	if hasChooser && hasChoice && chooser != "" && choice != "" {
		// A chooser answer's envelope is closed: anything else at the top
		// level would be dropped without reaching the engine, leaving the
		// sender unable to tell a landed value from a lost one
		// (20260811-233331-s-tac-bjn). Refuse by name instead.
		for _, key := range slices.Sorted(maps.Keys(request.Report)) {
			switch key {
			case "chooser", "choice", "fields", "userWords":
			default:
				return nil, fmt.Errorf("field %q cannot ride a chooser answer at the top level and was not applied — state a chooser answer collects goes inside its fields object", key)
			}
		}
		fields, _ := request.Report["fields"].(map[string]any)
		words, _ := request.Report["userWords"].(string)
		serve, err = w.session.Answer(request.Instance, chooser, choice, fields, words)
	} else {
		serve, err = w.session.Report(request.Instance, request.Report)
	}
	if err != nil {
		return nil, err
	}
	// A displaced binding fails its append inside the engine, which stashes the
	// typed error rather than returning it; surface it here so the first write
	// fails typed instead of a phantom success.
	if err := w.session.SinkErr(); err != nil {
		return nil, err
	}
	result := w.publicServe(serve)
	if serve.Status != engine.StatusRunning {
		// The shell ending is the dialogue ending: its serve carries the way on
		// rather than a spent position with nothing to do. Any other instance
		// ending lands the dialogue back on the shell.
		if serve.Instance == w.shell {
			result.Instructions = NewSessionNote
			return result, nil
		}
		result.Base, err = w.ServeShell(ctx, identity)
		if err != nil {
			return result, fmt.Errorf("serving session shell after advancing %s: %w", request.Instance, err)
		}
	}
	return result, nil
}

func (w *WorkflowSession) ServeShell(ctx context.Context, identity RequestIdentity) (*WorkflowServe, error) {
	if w.shell == "" {
		return nil, nil
	}
	w.setOperation(ctx, identity)
	serve, err := w.session.Serve(w.shell)
	if err != nil {
		return nil, err
	}
	return w.publicServe(serve), nil
}

func (w *WorkflowSession) ServeAll(ctx context.Context, identity RequestIdentity) (WorkflowResumeResult, error) {
	w.setOperation(ctx, identity)
	return w.resumeResult()
}

func (w *WorkflowSession) resumeResult() (WorkflowResumeResult, error) {
	result := WorkflowResumeResult{Session: w.ID(), Participant: w.session.Participant, Label: w.session.Label, Branch: w.branch}
	for _, inst := range w.session.Instances() {
		if inst.Status != engine.StatusRunning {
			continue
		}
		serve, err := w.session.Serve(inst.ID)
		if err != nil {
			return WorkflowResumeResult{}, err
		}
		// The collected projection is the honest-surfacing half of the
		// re-entry contract: a resuming agent sees the values this instance
		// already holds. It rides the resume path alone — publicServe stays
		// state-free so door, next, and base-junction serves are unchanged.
		ws := w.publicServe(serve)
		ws.Collected = inst.Store.Collected()
		result.Open = append(result.Open, *ws)
	}
	return result, nil
}

func (w *WorkflowSession) Abandon(ctx context.Context, identity RequestIdentity, instance, reason string) (WorkflowAbandonResult, error) {
	w.setOperation(ctx, identity)
	if instance == w.shell {
		return WorkflowAbandonResult{}, fmt.Errorf("the session shell concludes through its own junction")
	}
	result := WorkflowAbandonResult{Abandoned: true}
	if inst, ok := w.session.Instance(instance); ok {
		if marker, present := workflowStoreString(inst.Store, "wipMarker"); present {
			result.HeldMarkers = append(result.HeldMarkers, marker)
		}
	}
	if err := w.session.Abandon(instance, reason); err != nil {
		return WorkflowAbandonResult{}, err
	}
	if err := w.session.SinkErr(); err != nil {
		return WorkflowAbandonResult{}, err
	}
	base, err := w.ServeShell(ctx, identity)
	if err != nil {
		return result, fmt.Errorf("serving session shell after abandoning %s: %w", instance, err)
	}
	result.Base = base
	return result, nil
}

func (w *WorkflowSession) Park(ctx context.Context, identity RequestIdentity, instance, note string) (WorkflowParkResult, error) {
	w.setOperation(ctx, identity)
	if instance == w.shell {
		return WorkflowParkResult{}, fmt.Errorf("park is for moves; the session shell is the junction they park back to")
	}
	inst, ok := w.session.Instance(instance)
	if !ok {
		return WorkflowParkResult{}, fmt.Errorf("instance %q not found in session", instance)
	}
	if err := w.session.Park(instance, note); err != nil {
		return WorkflowParkResult{}, err
	}
	if err := w.session.SinkErr(); err != nil {
		return WorkflowParkResult{}, err
	}
	base, err := w.ServeShell(ctx, identity)
	if err != nil {
		return WorkflowParkResult{Instance: inst.ID, Procedure: inst.Spec.Canonical, Step: inst.Step}, fmt.Errorf("serving session shell after parking %s: %w", instance, err)
	}
	return WorkflowParkResult{Instance: inst.ID, Procedure: inst.Spec.Canonical, Step: inst.Step, Base: base}, nil
}

func (w *WorkflowSession) StageAttachment(ctx context.Context, identity RequestIdentity, filename string, content []byte) (string, error) {
	w.setOperation(ctx, identity)
	blob, err := w.app.StageBlob(ctx, identity, w.project, SessionRef{Subject: w.binding.Subject, Session: w.binding.SessionID}, filename, content)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]string{"handle": filename, "blob_id": blob.ID})
	if err != nil {
		return "", err
	}
	if err := w.appendStoredEvent(workflowStagedBlobCode, payload); err != nil {
		return "", err
	}
	w.staged[filename] = blob.ID
	return filename, nil
}

// EditStagedAttachment applies ordered exact search-replace pairs to a staged
// file addressed by its handle and stages the result under the same handle,
// so a small correction costs neither a full re-stage nor a full re-read
// (20260826-120330-d-tac-8f8). Atomic: a failing pair names itself and the
// staged file stays unchanged.
func (w *WorkflowSession) EditStagedAttachment(ctx context.Context, identity RequestIdentity, handle string, pairs []types.PatchPair) error {
	w.setOperation(ctx, identity)
	current, err := w.readStagedBlob(ctx, identity, handle)
	if err != nil {
		return err
	}
	patched, err := textpatch.Apply(string(current), pairs)
	if err != nil {
		return fmt.Errorf("editing staged %q: %w (nothing applied; the staged file is unchanged)", handle, err)
	}
	_, err = w.StageAttachment(ctx, identity, handle, []byte(patched))
	return err
}

// ReadStagedAttachment returns one bounded page of a staged file by handle,
// plus the session's staged handles as the discovery surface.
func (w *WorkflowSession) ReadStagedAttachment(ctx context.Context, identity RequestIdentity, handle string, offset int64, maxBytes int) (AttachmentPage, []string, error) {
	w.setOperation(ctx, identity)
	content, err := w.readStagedBlob(ctx, identity, handle)
	if err != nil {
		return AttachmentPage{}, w.StagedHandles(), err
	}
	total := int64(len(content))
	if offset < 0 || offset > total {
		return AttachmentPage{}, w.StagedHandles(), fmt.Errorf("offset %d out of range (staged %q holds %d bytes)", offset, handle, total)
	}
	end := offset + int64(maxBytes)
	if maxBytes <= 0 || end > total {
		end = total
	}
	return AttachmentPage{
		Filename: handle, Content: content[offset:end],
		Offset: offset, NextOffset: end, TotalSize: total, More: end < total,
	}, w.StagedHandles(), nil
}

// StagedHandles lists the session's staged file handles, sorted.
func (w *WorkflowSession) StagedHandles() []string {
	return slices.Sorted(maps.Keys(w.staged))
}

func (w *WorkflowSession) readStagedBlob(ctx context.Context, identity RequestIdentity, handle string) ([]byte, error) {
	blobID, ok := w.staged[handle]
	if !ok {
		return nil, fmt.Errorf("no file is staged under handle %q in this session", handle)
	}
	ref := SessionRef{Subject: w.binding.Subject, Session: w.binding.SessionID}
	rc, err := w.app.OpenStagedBlob(ctx, identity, w.project, ref, blobID)
	if err != nil {
		return nil, err
	}
	content, err := io.ReadAll(rc)
	if closeErr := rc.Close(); err == nil {
		err = closeErr
	}
	return content, err
}

type branchBoundEvent struct {
	Branch string `json:"branch"`
}

type branchClearedEvent struct {
	Branch string `json:"branch"`
}

// BindBranch changes the durable session-level branch declaration. Setting a
// binding resolves the branch against the runtime's live branch capability
// before the CAS append; clearing is store-only and works without that
// capability.
func (w *WorkflowSession) BindBranch(ctx context.Context, identity RequestIdentity, branch string, clear bool) error {
	w.setOperation(ctx, identity)
	if branch != strings.TrimSpace(branch) {
		return &ApplicationError{Code: ErrorInvalidArgument, Message: "branch must not have leading or trailing whitespace"}
	}
	if (branch != "") == clear {
		return &ApplicationError{Code: ErrorInvalidArgument, Message: "pass exactly one of a nonblank branch or clear=true"}
	}
	err := w.bindBranchOnce(branch, clear)
	var appErr *ApplicationError
	if !errors.As(err, &appErr) || appErr.Code != ErrorSessionConflict {
		return err
	}
	if err := w.resyncBindingVersion(); err != nil {
		return err
	}
	err = w.bindBranchOnce(branch, clear)
	if errors.As(err, &appErr) && appErr.Code == ErrorSessionConflict {
		return &ApplicationError{Code: ErrorSessionConflict, Message: appErr.Message + " — " + reorientSuffix}
	}
	return err
}

func (w *WorkflowSession) bindBranchOnce(branch string, clear bool) error {
	principal, runtime, stored, err := w.app.resolveSession(w.ctx, w.identity, w.ID(), AccessRead)
	if err != nil {
		return err
	}
	if err := verifyBinding(stored, w.binding); err != nil {
		return err
	}
	if !clear {
		if runtime.options.Branches == nil {
			return &ApplicationError{Code: ErrorBranchUnavailable, Message: "this deployment has no branch concept"}
		}
		target := MutationTarget{Project: runtime.options.Project.ID, Branch: branch}
		if err := runtime.options.Branches.ValidateBranch(w.ctx, target); err != nil {
			return err
		}
	}

	metadata := stored.Metadata
	eventCode := BranchBoundEventCode
	eventBranch := branch
	if clear {
		eventCode = BranchClearedEventCode
		eventBranch = metadata.Branch
		metadata.Branch = ""
	} else {
		metadata.Branch = branch
	}
	now := w.app.now().UTC().Round(0)
	metadata.UpdatedAt = now
	if metadata.Attachment != nil {
		attachment := *metadata.Attachment
		attachment.LastActivity = now
		metadata.Attachment = &attachment
	}
	if metadata.Subject != principal.Subject {
		return &ApplicationError{Code: ErrorSessionOwnership, Message: "session subject changed"}
	}
	var event any = branchBoundEvent{Branch: eventBranch}
	if clear {
		event = branchClearedEvent{Branch: eventBranch}
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	next, err := w.app.sessions.Append(w.ctx, w.ID(), stored.Version, SessionAppend{
		Metadata: &metadata,
		Events:   []StoredEvent{{CodecVersion: SessionCodecVersion, Code: eventCode, Payload: payload}},
	})
	if err != nil {
		return err
	}
	w.binding.Version = next
	w.setBranch(metadata.Branch)
	return nil
}

func (w *WorkflowSession) LogRead(ctx context.Context, identity RequestIdentity, tool string, full, summary []string) error {
	w.setOperation(ctx, identity)
	w.session.LogRead(tool, full, summary)
	return w.session.SinkErr()
}

// Framing composes the session framing as ordered, independently dedupable
// blocks: the engine-supplied info block (participant, language, search modes)
// first, then one block per declared shell lane, rendered through the injection
// mechanism. Returning the lanes as separate blocks — not one joined string —
// lets the host dedup each on its own, so a graph write that changes only the
// recent-moves lane re-serves that lane alone, never the stable aspirations or
// directives (I6, A1). A shell with no declared lanes yields the info block
// alone; there is no Go-constant fallback.
func (w *WorkflowSession) Framing(ctx context.Context, identity RequestIdentity) ([]string, error) {
	w.setOperation(ctx, identity)
	info, err := w.app.Info(ctx, identity, w.project, InfoRequest{})
	if err != nil {
		return nil, err
	}
	var infoBlock strings.Builder
	fmt.Fprintf(&infoBlock, "Local participant: %s\n", info.Participant)
	if info.Language != "" {
		fmt.Fprintf(&infoBlock, "Language: %s\n", info.Language)
	}
	fmt.Fprintf(&infoBlock, "Search: %s", info.Search)
	if w.branch != "" {
		fmt.Fprintf(&infoBlock, "\nBranch binding: %s", w.branch)
	}
	lanes, err := w.framingLanes()
	if err != nil {
		return nil, err
	}
	blocks := append([]string{infoBlock.String()}, lanes...)
	health, err := w.graphHealthBlock()
	if err != nil {
		return nil, err
	}
	if health != "" {
		blocks = append(blocks, health)
	}
	return blocks, nil
}

// graphHealthBlock renders a compact framing notice of graph-integrity
// problems — entry warnings and unreadable (parse-failed) entries — so an
// agent opening a session notices that the graph carries warnings. It is
// self-contained: it presents the problem lines themselves and never tells the
// agent to run a CLI command. Empty when the graph is clean; the list is
// capped so a badly degraded graph cannot flood the framing.
func (w *WorkflowSession) graphHealthBlock() (string, error) {
	var (
		graph *model.Graph
		err   error
	)
	if shell, ok := w.session.Instance(w.shell); ok {
		graph, err = w.graphs.CurrentFor(shell.Store)
	} else {
		graph, err = w.graphs.Current()
	}
	if err != nil {
		return "", err
	}
	health := graph.Health()
	if health.Clean() {
		return "", nil
	}
	const maxLines = 5
	var b strings.Builder
	fmt.Fprintf(&b, "Graph health: %d warning(s), %d unreadable entry/entries.\n", health.Warnings, health.LoadErrors)
	shown := health.Issues
	if len(shown) > maxLines {
		shown = shown[:maxLines]
	}
	for _, issue := range shown {
		fmt.Fprintf(&b, "%s: %s\n", issue.Ref, issue.Message)
	}
	if extra := len(health.Issues) - len(shown); extra > 0 {
		fmt.Fprintf(&b, "(… and %d more)", extra)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// framingLanes renders the shell procedure's declared framing lanes through the
// engine's inject query mechanism, one block per lane (empty results omitted).
// A lane whose query returns a non-string fails loud — a framing lane must
// render to text, never be silently dropped.
func (w *WorkflowSession) framingLanes() ([]string, error) {
	if w.shell == "" {
		return nil, nil
	}
	inst, ok := w.session.Instance(w.shell)
	if !ok {
		return nil, nil
	}
	var lanes []string
	for _, call := range inst.Spec.Framing {
		result, cut, err := w.session.Inject(w.shell, call)
		if err != nil {
			return nil, fmt.Errorf("framing lane %s: %w", call.Fn, err)
		}
		text, ok := result.(string)
		if !ok {
			return nil, fmt.Errorf("framing lane %s: query returned %T, want string", call.Fn, result)
		}
		if cut != nil {
			text += "\n" + framingCutNotice(*cut)
		}
		if text = strings.TrimSpace(text); text != "" {
			lanes = append(lanes, text)
		}
	}
	return lanes, nil
}

// framingCutNotice renders one framing lane's cut in this surface's register,
// appended below the lane's own content — framing blocks are engine-rendered,
// never authored text, so the notice rides inside the block it describes.
func framingCutNotice(cut truncate.Cut) string {
	var b strings.Builder
	if cut.Dropped > 0 {
		fmt.Fprintf(&b, "(cut for size: %d of %d items dropped", cut.Dropped, cut.Total)
	} else {
		fmt.Fprintf(&b, "(cut for size: kept %d of %d bytes", cut.KeptBytes, cut.TotalBytes)
	}
	if cut.Pull != "" {
		fmt.Fprintf(&b, " — pull the rest: %s", cut.Pull)
	}
	b.WriteString(")")
	return b.String()
}

func (a *Application) ListWorkflowSessions(ctx context.Context, identity RequestIdentity, project ProjectID) ([]WorkflowSessionSummary, error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return nil, err
	}
	page, err := a.sessions.List(ctx, SessionFilter{Subject: principal.Subject, Project: runtime.options.Project.ID})
	if err != nil {
		return nil, err
	}
	now := a.now().UTC()
	result := make([]WorkflowSessionSummary, 0, len(page.Sessions))
	log := slogutils.FromContext(ctx)
	for _, item := range page.Sessions {
		// A session this binary cannot read may have been written by a newer
		// one. Skipping it keeps every other session listed and resumable
		// rather than making one log take the whole listing down.
		if err := validateStoredSession(item); err != nil {
			log.Warn("skipping unreadable session", "session", item.Metadata.ID, "err", err)
			continue
		}
		events, err := decodeWorkflowEvents(item.Events)
		if err != nil {
			log.Warn("skipping unreadable session", "session", item.Metadata.ID, "err", err)
			continue
		}
		summary := deriveWorkflowSummary(item.Metadata.ID, events)
		summary.Label = item.Metadata.Label
		if summary.Label == "" {
			summary.Label = workflowLabel(events)
		}
		if summary.Label == "" {
			summary.Label = workflowBodyLabel(events)
		}
		// An ended dialogue holds no open work however its instances were left
		// standing: conclude drops its threads and no served path resumes them, so
		// listing them would offer work that cannot be reached (d-tac-k4q). The log
		// stays readable until retention expires; it just stops being open work.
		if item.Metadata.Ended != nil {
			summary.Open = nil
		}
		summary.Participant = item.Metadata.Participant
		summary.Branch = item.Metadata.Branch
		if item.Metadata.UpdatedAt.After(summary.LastActivity) {
			summary.LastActivity = item.Metadata.UpdatedAt
		}
		if item.Metadata.Attachment != nil {
			attachment := *item.Metadata.Attachment
			summary.Attachment = &attachment
			summary.Active = attachmentActive(&attachment, now)
		}
		result = append(result, summary)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].LastActivity.After(result[j].LastActivity) })
	return result, nil
}

// AbandonWorkflowSession tears down a session by handle: it replays into a
// buffering sink (no stamp), abandons the instances, then records the terminal
// abandon in one final append. A mid-teardown failure returns before that
// append, so the session stays as it was — honest state, no phantom teardown.
// Ending a session is a participant act, and holding the handle is what
// authorizes it (d-cpt-aen).
func (a *Application) AbandonWorkflowSession(ctx context.Context, identity RequestIdentity, request WorkflowResumeRequest, reason string) (WorkflowAbandonResult, error) {
	if request.SessionID == "" {
		return WorkflowAbandonResult{}, fmt.Errorf("sdd: session ID is required")
	}
	_, runtime, stored, err := a.resolveSession(ctx, identity, request.SessionID, AccessRead)
	if err != nil {
		return WorkflowAbandonResult{}, err
	}
	if end := stored.Metadata.Ended; end != nil {
		return WorkflowAbandonResult{}, endedSessionError(stored.Metadata, *end)
	}
	w, err := a.newWorkflow(ctx, identity, runtime.options.Project.ID, sessionBindingFrom(stored), stored.Metadata.Branch)
	if err != nil {
		return WorkflowAbandonResult{}, err
	}
	events, err := decodeWorkflowEvents(stored.Events)
	if err != nil {
		return WorkflowAbandonResult{}, err
	}
	sink := &bufferSink{}
	resolve := func(canonical string) (*engine.Spec, error) { return w.loadProcedure(canonical) }
	w.session, err = w.engine.ReplaySession(string(request.SessionID), stored.Metadata.Participant, events, resolve, sink)
	if err != nil {
		return WorkflowAbandonResult{}, fmt.Errorf("replaying session %s: %w", request.SessionID, err)
	}
	result := WorkflowAbandonResult{Abandoned: true, Session: stored.Metadata.ID, Label: w.session.Label}
	for _, inst := range w.session.Instances() {
		if inst.Status != engine.StatusRunning {
			continue
		}
		if inst.Spec.Class != model.ProcedureClassShell {
			result.Discarded = append(result.Discarded, WorkflowInstanceSummary{Instance: inst.ID, Procedure: inst.Spec.Canonical, Step: inst.Step})
			if marker, ok := workflowStoreString(inst.Store, "wipMarker"); ok {
				result.HeldMarkers = append(result.HeldMarkers, marker)
			}
		}
		if err := w.session.Abandon(inst.ID, reason); err != nil {
			return WorkflowAbandonResult{}, err
		}
	}
	now := a.now().UTC().Round(0)
	metadata := stored.Metadata
	metadata.UpdatedAt = now
	metadata.Ended = &SessionEnd{Act: SessionAbandoned, EndedAt: now, Reason: strings.TrimSpace(reason)}
	appendData := SessionAppend{Metadata: &metadata}
	for _, event := range sink.events {
		payload, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			return WorkflowAbandonResult{}, marshalErr
		}
		appendData.Events = append(appendData.Events, StoredEvent{CodecVersion: SessionCodecVersion, Code: WorkflowEventCode, Payload: payload})
	}
	if _, err := a.sessions.Append(ctx, request.SessionID, stored.Version, appendData); err != nil {
		return WorkflowAbandonResult{}, err
	}
	return result, nil
}

// bufferSink collects engine events instead of persisting each, so a teardown
// can abandon instances in memory and write the abandon events with the final
// attachment-ending metadata in one atomic append (never per-instance, never
// under a claimed attachment).
type bufferSink struct{ events []engine.Event }

func (b *bufferSink) Append(event engine.Event) error {
	if event.Event == engine.EventServed {
		return nil
	}
	b.events = append(b.events, event)
	return nil
}

func (w *WorkflowSession) setOperation(ctx context.Context, identity RequestIdentity) {
	w.ctx = ctx
	w.identity = identity
}

func (w *WorkflowSession) setLabel(label string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil
	}
	if strings.ContainsAny(label, "\n\r") {
		return fmt.Errorf("label must be a single line")
	}
	if utf8.RuneCountInString(label) > WorkflowMaxLabelLength {
		return fmt.Errorf("label exceeds %d characters", WorkflowMaxLabelLength)
	}
	w.session.SetLabel(label)
	return nil
}

func (w *WorkflowSession) ensureShell() error {
	for _, inst := range w.session.Instances() {
		if inst.Status == engine.StatusRunning && inst.Spec.Class == model.ProcedureClassShell {
			w.shell = inst.ID
			return nil
		}
	}
	spec, err := w.loadProcedure(DefaultShellCanonical)
	if err != nil {
		return fmt.Errorf("auto-starting session shell: %w", err)
	}
	serve, err := w.session.Start(spec, nil, "")
	if err != nil {
		return fmt.Errorf("auto-starting session shell: %w", err)
	}
	w.shell = serve.Instance
	return nil
}

func (w *WorkflowSession) loadProcedure(canonical string) (*engine.Spec, error) {
	graph, err := w.graphs.Current()
	if err != nil {
		return nil, fmt.Errorf("loading graph: %w", err)
	}
	entry := graph.ResolveProcedure(canonical)
	if entry == nil {
		return nil, fmt.Errorf("no procedure %q", canonical)
	}
	return engine.LoadSpec(entry, w.engine.Registry)
}

func (w *WorkflowSession) publicServe(serve *engine.Serve) *WorkflowServe {
	result := &WorkflowServe{
		Session: w.ID(), Project: w.instanceProject(serve.Instance), Branch: w.branch,
		Instance: serve.Instance, Procedure: serve.Procedure, Status: string(serve.Status),
		Step: serve.Step, Goal: serve.Goal, Instructions: serve.Instructions, Missing: serve.Missing,
		ReportSchema: serve.ReportSchema, Produced: serve.Produced, Diagnostics: append([]string(nil), serve.Diagnostics...),
		InstructionLanes: append([]types.ServeLane(nil), serve.Lanes...),
		Sizes:            append([]types.PartSize(nil), serve.Sizes...),
	}
	if serve.Chooser != nil {
		chooser := &WorkflowChooser{Chooser: serve.Chooser.Chooser, Kind: ChooserKind(serve.Chooser.Kind)}
		for _, option := range serve.Chooser.Options {
			out := WorkflowChooserOption{Choice: option.Choice}
			for _, field := range option.Collect {
				name := field.Name
				if field.Optional {
					name += "?"
				}
				out.Collect = append(out.Collect, name)
			}
			chooser.Options = append(chooser.Options, out)
		}
		result.PendingChooser = chooser
	}
	if inst, ok := w.session.Instance(serve.Instance); ok && inst.Spec.Class == model.ProcedureClassTask {
		result.Execution = ExecutionForkPreferred
	}
	return result
}

type workflowGraphs struct {
	workflow *WorkflowSession
	snapshot *Snapshot
	targets  map[MutationTarget]*Snapshot
}

func (g *workflowGraphs) Current() (*model.Graph, error) {
	if g.snapshot != nil {
		return g.snapshot.graph, nil
	}
	_, runtime, err := g.workflow.app.resolve(g.workflow.ctx, g.workflow.identity, g.workflow.project, AccessRead)
	if err != nil {
		return nil, err
	}
	snapshot, err := g.workflow.app.snapshotWithDependencies(g.workflow.ctx, g.workflow.identity, runtime)
	if err != nil {
		return nil, err
	}
	g.snapshot = snapshot
	return snapshot.graph, nil
}

// CurrentFor resolves the graph authority carried by a procedure instance: the
// project it targets, on the branch its state or the session binding names.
func (g *workflowGraphs) CurrentFor(store *engine.Store) (*model.Graph, error) {
	target, fromBinding := g.workflow.effectiveTarget(store)
	if target.Branch == "" && target.Project == g.workflow.project {
		return g.Current()
	}
	return g.currentTarget(target, fromBinding)
}

func (g *workflowGraphs) currentTarget(target MutationTarget, fromBinding bool) (*model.Graph, error) {
	runtime, err := g.workflow.targetRuntime(target.Project, AccessRead)
	if err != nil {
		return nil, err
	}
	if snapshot := g.targets[target]; snapshot != nil {
		return snapshot.graph, nil
	}
	var snapshot *Snapshot
	if target.Branch == "" {
		// Another project's view on its configured default: its graph as the
		// store serves it, no acquisition — reads in it need no write authority.
		snapshot, err = runtime.options.Graph.Current(g.workflow.ctx)
		if err != nil {
			return nil, err
		}
	} else {
		resolved, err := resolveMutationTarget(runtime, target)
		if err != nil {
			return nil, err
		}
		// A cache miss deliberately uses the same short-lived acquisition as a
		// write snapshot. Local acquisition is a checkout lookup; remote target
		// acquirers may clone, so remote compositions should accelerate this seam
		// with their read cache while preserving explicit target authority.
		snapshot, err = snapshotMutationTarget(g.workflow.ctx, runtime, resolved)
		if err != nil {
			return nil, withSessionBindingTargetError(g.workflow.branch, fromBinding, err)
		}
	}
	snapshot, err = g.workflow.app.snapshotWithDependenciesFrom(g.workflow.ctx, g.workflow.identity, runtime, snapshot)
	if err != nil {
		return nil, err
	}
	if g.targets == nil {
		g.targets = make(map[MutationTarget]*Snapshot)
	}
	g.targets[target] = snapshot
	return snapshot.graph, nil
}

func (g *workflowGraphs) Invalidate() {
	g.snapshot = nil
	g.targets = nil
}

type workflowSink struct{ workflow *WorkflowSession }

func (s *workflowSink) Append(event engine.Event) error {
	// A serve is a pure read: its forensic served marker never reaches the
	// durable log, so re-serving (ServeShell/ServeAll) writes nothing
	// and each engine operation appends only its own event with the stamp.
	if event.Event == engine.EventServed {
		return nil
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	events := []StoredEvent{{CodecVersion: SessionCodecVersion, Code: WorkflowEventCode, Payload: payload}}
	// The started instance's target lands in the same append as its start, so
	// no instance ever exists without the project it was started in.
	w := s.workflow
	if event.Event == engine.EventStarted && w.startProject != "" {
		project := w.startProject
		w.startProject = ""
		record, err := instanceProjectRecord(event.Instance, project)
		if err != nil {
			return err
		}
		events = append(events, record)
		if err := w.appendStoredEvents(events); err != nil {
			return err
		}
		w.instanceProjects[event.Instance] = project
		return nil
	}
	return w.appendStoredEvents(events)
}

// appendStoredEvent appends one of the session's own events with the activity
// stamp.
func (w *WorkflowSession) appendStoredEvent(code string, payload json.RawMessage) error {
	return w.appendStoredEvents([]StoredEvent{{CodecVersion: SessionCodecVersion, Code: code, Payload: payload}})
}

// appendStoredEvents appends the session's own events in one CAS append with
// the activity stamp. A version race (another consumer of the same handle
// appended under us) is absorbed once: resync the observed version and retry;
// a second conflict surfaces with the way on.
func (w *WorkflowSession) appendStoredEvents(events []StoredEvent) error {
	err := w.appendStoredEventsOnce(events)
	var appErr *ApplicationError
	if !errors.As(err, &appErr) || appErr.Code != ErrorSessionConflict {
		return err
	}
	if err := w.resyncBindingVersion(); err != nil {
		return err
	}
	err = w.appendStoredEventsOnce(events)
	if errors.As(err, &appErr) && appErr.Code == ErrorSessionConflict {
		// The retry lost too — surface the conflict with a next step rather than a
		// dead-end "version changed".
		return &ApplicationError{Code: ErrorSessionConflict, Message: appErr.Message + " — " + reorientSuffix}
	}
	return err
}

// resyncBindingVersion reloads the session and advances the observed version to
// the store's so a retried append passes the version CAS; an ended session
// surfaces typed instead of being written on.
func (w *WorkflowSession) resyncBindingVersion() error {
	_, _, stored, err := w.app.resolveSession(w.ctx, w.identity, w.ID(), AccessRead)
	if err != nil {
		return err
	}
	if err := verifyOwnership(stored, w.binding); err != nil {
		return err
	}
	w.binding.Version = stored.Version
	w.setBranch(stored.Metadata.Branch)
	return nil
}

func (w *WorkflowSession) appendStoredEventsOnce(events []StoredEvent) error {
	principal, _, stored, err := w.app.resolveSession(w.ctx, w.identity, w.ID(), AccessRead)
	if err != nil {
		return err
	}
	if err := verifyBinding(stored, w.binding); err != nil {
		return err
	}
	metadata := stored.Metadata
	now := w.app.now().UTC().Round(0)
	metadata.UpdatedAt = now
	if metadata.Attachment != nil {
		attachment := *metadata.Attachment
		attachment.LastActivity = now
		metadata.Attachment = &attachment
	}
	for _, stored := range events {
		if stored.Code != WorkflowEventCode {
			continue
		}
		var event engine.Event
		if err := json.Unmarshal(stored.Payload, &event); err != nil {
			return err
		}
		switch event.Event {
		case engine.EventLabeled:
			metadata.Label, _ = event.Data["label"].(string)
		case engine.EventCompleted, engine.EventAbandoned:
			// The shell leaving running is the participant act that ends the
			// dialogue — whether the user answered conclude or a quiescent session
			// auto-concluded on leave — so the terminal record lands in the same
			// append as the act it records (d-cpt-rw7). Written once, never revised.
			if event.Instance == w.shell && metadata.Ended == nil {
				metadata.Ended = &SessionEnd{Act: SessionConcluded, EndedAt: now}
			}
		}
	}
	if metadata.Subject != principal.Subject {
		return &ApplicationError{Code: ErrorSessionOwnership, Message: "session subject changed"}
	}
	next, err := w.app.sessions.Append(w.ctx, w.ID(), stored.Version, SessionAppend{Metadata: &metadata, Events: events})
	if err != nil {
		return err
	}
	w.binding.Version = next
	return nil
}

func (w *WorkflowSession) restoreStagedBlobs(events []StoredEvent) error {
	for _, stored := range events {
		if stored.Code != workflowStagedBlobCode {
			continue
		}
		if !SupportedSessionCodecVersion(stored.CodecVersion) {
			return &ApplicationError{Code: ErrorMigrationRequired, Message: "unsupported staged blob event codec", Version: stored.CodecVersion}
		}
		var item struct {
			Handle string `json:"handle"`
			BlobID string `json:"blob_id"`
		}
		if err := json.Unmarshal(stored.Payload, &item); err != nil {
			return fmt.Errorf("decoding staged blob event: %w", err)
		}
		if item.Handle == "" || item.BlobID == "" {
			return fmt.Errorf("decoding staged blob event: handle and blob_id are required")
		}
		w.staged[item.Handle] = item.BlobID
	}
	return nil
}

func decodeWorkflowEvents(stored []StoredEvent) ([]engine.Event, error) {
	var events []engine.Event
	for _, item := range stored {
		if item.Code != WorkflowEventCode {
			continue
		}
		if !SupportedSessionCodecVersion(item.CodecVersion) {
			return nil, &ApplicationError{Code: ErrorMigrationRequired, Message: "unsupported workflow event codec", Version: item.CodecVersion}
		}
		var event engine.Event
		if err := json.Unmarshal(item.Payload, &event); err != nil {
			return nil, fmt.Errorf("decoding workflow event: %w", err)
		}
		events = append(events, event)
	}
	return events, nil
}

func deriveWorkflowSummary(id SessionID, events []engine.Event) WorkflowSessionSummary {
	result := WorkflowSessionSummary{Session: id}
	type state struct {
		procedure string
		step      string
		running   bool
		shell     bool
	}
	states := map[string]*state{}
	var order []string
	for _, event := range events {
		if event.TS.After(result.LastActivity) {
			result.LastActivity = event.TS
		}
		switch event.Event {
		case engine.EventStarted:
			item := &state{running: true}
			item.procedure, _ = event.Data["procedure"].(string)
			item.step, _ = event.Data["step"].(string)
			class, _ := event.Data["class"].(string)
			item.shell = class == string(model.ProcedureClassShell)
			states[event.Instance] = item
			order = append(order, event.Instance)
			if params, ok := event.Data["params"].(map[string]any); ok && result.Anchor == "" {
				result.Anchor, _ = params["anchor"].(string)
			}
			// An anchor can arrive as a dispatch seed instead of a caller param
			// (engage seeding its capture) — either source names the session's anchor.
			if seed, ok := event.Data["seed"].(map[string]any); ok && result.Anchor == "" {
				result.Anchor, _ = seed["anchor"].(string)
			}
		case engine.EventTransition:
			if item := states[event.Instance]; item != nil {
				if to, ok := event.Data["to"].(string); ok && !engine.IsEndTarget(to) {
					item.step = to
				}
			}
		case engine.EventCompleted, engine.EventAbandoned:
			if item := states[event.Instance]; item != nil {
				item.running = false
			}
		}
	}
	for _, instance := range order {
		item := states[instance]
		if item == nil || !item.running || item.shell {
			continue
		}
		result.Open = append(result.Open, WorkflowInstanceSummary{Instance: instance, Procedure: item.procedure, Step: item.step})
	}
	return result
}

func workflowLabel(events []engine.Event) string {
	var label string
	for _, event := range events {
		if event.Event == engine.EventLabeled {
			label, _ = event.Data["label"].(string)
		}
	}
	return label
}

func workflowBodyLabel(events []engine.Event) string {
	var label string
	for _, event := range events {
		if event.Event != engine.EventReport {
			continue
		}
		fields, _ := event.Data["fields"].(map[string]any)
		body, _ := fields["body"].(string)
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		if newline := strings.IndexByte(body, '\n'); newline >= 0 {
			body = body[:newline]
		}
		runes := []rune(strings.TrimSpace(body))
		if len(runes) > WorkflowMaxLabelLength {
			runes = runes[:WorkflowMaxLabelLength]
		}
		label = string(runes)
	}
	return label
}

func newWorkflowSessionID(now time.Time) SessionID {
	// 128 bits: possession of a handle must imply issuance (d-cpt-aen).
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("sdd: reading session randomness: %v", err))
	}
	return SessionID("s_" + now.Format("20060102-150405") + "-" + hex.EncodeToString(buf))
}
