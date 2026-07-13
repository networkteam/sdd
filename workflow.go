package sdd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
)

const (
	WorkflowEventCode      = "workflow_event"
	workflowStagedBlobCode = "workflow_staged_blob"
	DefaultShellCanonical  = "user-dialogue"
	WorkflowMaxLabelLength = 120
	ExecutionForkPreferred = "fork-preferred"
	workflowFramingLayout  = `aspirations:rank(heat(exp-14d)):n(8):brief,kind(directive):intent(guiding):active:rank(heat(exp-14d)):n(10):name("Guiding directives"):brief:as-list,focus:brief,participants:brief`
)

type WorkflowOpenRequest struct {
	MCPSessionID  string
	ClientName    string
	ClientVersion string
	Shell         string
	Label         string
}

type WorkflowResumeRequest struct {
	SessionID     SessionID
	MCPSessionID  string
	ClientName    string
	ClientVersion string
	Takeover      bool
}

type WorkflowStartRequest struct {
	Canonical string
	Params    map[string]any
	Label     string
	Parent    string
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
	Session         SessionID
	Instance        string
	Procedure       string
	Status          string
	Step            string
	Goal            string
	Instructions    string
	Missing         []string
	ReportSchema    map[string]any
	PendingChooser  *WorkflowChooser
	Execution       string
	Produced        map[string]any
	Diagnostics     []string
	InstructionUnit string
	Base            *WorkflowServe
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
	Anchor       string
	Open         []WorkflowInstanceSummary
	LastActivity time.Time
	Holder       *SessionHolder
	HolderLive   bool
}

type WorkflowResumeResult struct {
	Session      SessionID
	Participant  string
	Label        string
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
	client   BindSessionRequest
	engine   *engine.Engine
	session  *engine.Session
	graphs   *workflowGraphs
	sink     *workflowSink
	shell    string
	staged   map[string]string
}

func (a *Application) OpenWorkflow(ctx context.Context, identity RequestIdentity, project ProjectID, request WorkflowOpenRequest) (*WorkflowSession, *WorkflowServe, error) {
	if request.MCPSessionID == "" {
		return nil, nil, fmt.Errorf("sdd: MCP session ID is required")
	}
	info, err := a.Info(ctx, identity, project, InfoRequest{})
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(info.Participant) == "" {
		return nil, nil, fmt.Errorf("sdd: resolved principal participant is required to open a workflow")
	}
	id := newWorkflowSessionID(time.Now())
	bind := BindSessionRequest{
		SessionID: id, MCPSessionID: request.MCPSessionID, ClientName: request.ClientName,
		ClientVersion: request.ClientVersion, Chooser: ChooserUser,
	}
	bound, err := a.BindSession(ctx, identity, info.Project.ID, bind)
	if err != nil {
		return nil, nil, err
	}
	w, err := a.newWorkflow(ctx, identity, info.Project.ID, bind, bound.Binding)
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

func (a *Application) ResumeWorkflow(ctx context.Context, identity RequestIdentity, project ProjectID, request WorkflowResumeRequest) (*WorkflowSession, WorkflowResumeResult, error) {
	if request.SessionID == "" || request.MCPSessionID == "" {
		return nil, WorkflowResumeResult{}, fmt.Errorf("sdd: session ID and MCP session ID are required")
	}
	bind := BindSessionRequest{
		SessionID: request.SessionID, MCPSessionID: request.MCPSessionID, ClientName: request.ClientName,
		ClientVersion: request.ClientVersion, Chooser: ChooserUser, Takeover: request.Takeover,
	}
	_, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return nil, WorkflowResumeResult{}, err
	}
	if _, err := runtime.options.Sessions.Load(ctx, request.SessionID); err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil, WorkflowResumeResult{}, fmt.Errorf("unknown session %q", request.SessionID)
		}
		return nil, WorkflowResumeResult{}, err
	}
	bound, err := a.BindSession(ctx, identity, project, bind)
	if err != nil {
		return nil, WorkflowResumeResult{}, err
	}
	w, err := a.newWorkflow(ctx, identity, bound.Project.ID, bind, bound.Binding)
	if err != nil {
		return nil, WorkflowResumeResult{}, err
	}
	stored, err := w.loadStoredSession(ctx)
	if err != nil {
		return nil, WorkflowResumeResult{}, err
	}
	if err := w.restoreStagedBlobs(stored.Events); err != nil {
		return nil, WorkflowResumeResult{}, err
	}
	events, err := decodeWorkflowEvents(stored.Events)
	if err != nil {
		return nil, WorkflowResumeResult{}, err
	}
	resolve := func(canonical string) (*engine.Spec, error) { return w.loadProcedure(canonical) }
	w.session, err = w.engine.ReplaySession(string(request.SessionID), stored.Metadata.Participant, events, resolve, w.sink)
	if err != nil {
		return nil, WorkflowResumeResult{}, fmt.Errorf("replaying session %s: %w", request.SessionID, err)
	}
	if err := w.ensureShell(); err != nil {
		return nil, WorkflowResumeResult{}, err
	}
	result, err := w.resumeResult()
	return w, result, err
}

func (a *Application) newWorkflow(ctx context.Context, identity RequestIdentity, project ProjectID, request BindSessionRequest, binding SessionBinding) (*WorkflowSession, error) {
	w := &WorkflowSession{app: a, project: project, identity: identity, ctx: ctx, binding: binding, client: request, staged: map[string]string{}}
	w.graphs = &workflowGraphs{workflow: w}
	w.sink = &workflowSink{workflow: w}
	registry, err := w.buildRegistry()
	if err != nil {
		return nil, err
	}
	w.engine = engine.New(registry, w.graphs)
	return w, nil
}

func (w *WorkflowSession) ID() SessionID { return w.binding.SessionID }

func (w *WorkflowSession) Project() ProjectID { return w.project }

func (w *WorkflowSession) Binding() SessionBinding { return w.binding }

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

func (w *WorkflowSession) Reopen(ctx context.Context, identity RequestIdentity, label string) (*WorkflowServe, error) {
	if err := w.begin(ctx, identity, ChooserUser); err != nil {
		return nil, err
	}
	if err := w.setLabel(label); err != nil {
		return nil, err
	}
	serve, err := w.session.Serve(w.shell)
	if err != nil {
		return nil, err
	}
	return w.publicServe(serve), nil
}

func (w *WorkflowSession) Start(ctx context.Context, identity RequestIdentity, request WorkflowStartRequest) (*WorkflowServe, error) {
	if strings.TrimSpace(request.Canonical) == "" {
		return nil, fmt.Errorf("canonical is required")
	}
	if err := w.begin(ctx, identity, ChooserAgent); err != nil {
		return nil, err
	}
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
	serve, err := w.session.Start(spec, request.Params, parent)
	if err != nil {
		return nil, err
	}
	return w.publicServe(serve), nil
}

func (w *WorkflowSession) Advance(ctx context.Context, identity RequestIdentity, request WorkflowAdvanceRequest) (*WorkflowServe, error) {
	if request.Instance == "" || len(request.Report) == 0 {
		return nil, fmt.Errorf("instance and report are required")
	}
	if err := w.begin(ctx, identity, w.currentChooser(request.Instance)); err != nil {
		return nil, err
	}
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
		fields, _ := request.Report["fields"].(map[string]any)
		words, _ := request.Report["userWords"].(string)
		serve, err = w.session.Answer(request.Instance, chooser, choice, fields, words)
	} else {
		serve, err = w.session.Report(request.Instance, request.Report)
	}
	if err != nil {
		return nil, err
	}
	result := w.publicServe(serve)
	if serve.Status != engine.StatusRunning && serve.Instance != w.shell {
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
	if err := w.begin(ctx, identity, ChooserUser); err != nil {
		return nil, err
	}
	serve, err := w.session.Serve(w.shell)
	if err != nil {
		return nil, err
	}
	return w.publicServe(serve), nil
}

func (w *WorkflowSession) ServeAll(ctx context.Context, identity RequestIdentity) (WorkflowResumeResult, error) {
	if err := w.begin(ctx, identity, ChooserUser); err != nil {
		return WorkflowResumeResult{}, err
	}
	return w.resumeResult()
}

func (w *WorkflowSession) resumeResult() (WorkflowResumeResult, error) {
	result := WorkflowResumeResult{Session: w.ID(), Participant: w.session.Participant, Label: w.session.Label}
	for _, inst := range w.session.Instances() {
		if inst.Status != engine.StatusRunning {
			continue
		}
		serve, err := w.session.Serve(inst.ID)
		if err != nil {
			return WorkflowResumeResult{}, err
		}
		result.Open = append(result.Open, *w.publicServe(serve))
	}
	return result, nil
}

func (w *WorkflowSession) Abandon(ctx context.Context, identity RequestIdentity, instance, reason string) (WorkflowAbandonResult, error) {
	if err := w.begin(ctx, identity, ChooserAgent); err != nil {
		return WorkflowAbandonResult{}, err
	}
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
	base, err := w.ServeShell(ctx, identity)
	if err != nil {
		return result, fmt.Errorf("serving session shell after abandoning %s: %w", instance, err)
	}
	result.Base = base
	return result, nil
}

func (w *WorkflowSession) Park(ctx context.Context, identity RequestIdentity, instance, note string) (WorkflowParkResult, error) {
	if err := w.begin(ctx, identity, ChooserAgent); err != nil {
		return WorkflowParkResult{}, err
	}
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
	base, err := w.ServeShell(ctx, identity)
	if err != nil {
		return WorkflowParkResult{Instance: inst.ID, Procedure: inst.Spec.Canonical, Step: inst.Step}, fmt.Errorf("serving session shell after parking %s: %w", instance, err)
	}
	return WorkflowParkResult{Instance: inst.ID, Procedure: inst.Spec.Canonical, Step: inst.Step, Base: base}, nil
}

func (w *WorkflowSession) StageAttachment(ctx context.Context, identity RequestIdentity, filename string, content []byte) (string, error) {
	if err := w.begin(ctx, identity, ChooserAgent); err != nil {
		return "", err
	}
	blob, err := w.app.StageBlob(ctx, identity, w.project, BlobOwner{Subject: w.binding.Subject, Session: w.binding.SessionID}, filename, content)
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

func (w *WorkflowSession) LogRead(ctx context.Context, identity RequestIdentity, tool string, full, summary []string) error {
	if err := w.begin(ctx, identity, ChooserAgent); err != nil {
		return err
	}
	w.session.LogRead(tool, full, summary)
	return nil
}

func (w *WorkflowSession) Framing(ctx context.Context, identity RequestIdentity) (string, error) {
	info, err := w.app.Info(ctx, identity, w.project, InfoRequest{})
	if err != nil {
		return "", err
	}
	view, err := w.app.View(ctx, identity, w.project, ViewRequest{Layout: workflowFramingLayout})
	if err != nil {
		return "", err
	}
	var out strings.Builder
	fmt.Fprintf(&out, "Local participant: %s\n", info.Participant)
	if info.Language != "" {
		fmt.Fprintf(&out, "Language: %s\n", info.Language)
	}
	fmt.Fprintf(&out, "Search: %s\n\n%s", info.Search, view.Sections)
	return out.String(), nil
}

func (w *WorkflowSession) Release(ctx context.Context, identity RequestIdentity) error {
	w.setOperation(ctx, identity)
	return w.app.ReleaseSession(ctx, identity, w.project, w.binding)
}

// Leave releases the durable holder. A session with no open moves is also
// auto-concluded so it does not remain as an empty parked dialogue.
func (w *WorkflowSession) Leave(ctx context.Context, identity RequestIdentity) error {
	if err := w.begin(ctx, identity, ChooserUser); err != nil {
		return err
	}
	if len(w.OpenInstances()) == 0 && w.shell != "" {
		if inst, ok := w.session.Instance(w.shell); ok && inst.Status == engine.StatusRunning {
			if err := w.session.Abandon(w.shell, "auto-concluded: session left with no open work"); err != nil {
				return err
			}
		}
	}
	return w.Release(ctx, identity)
}

func (a *Application) ListWorkflowSessions(ctx context.Context, identity RequestIdentity, project ProjectID) ([]WorkflowSessionSummary, error) {
	principal, runtime, err := a.resolve(ctx, identity, project, AccessRead)
	if err != nil {
		return nil, err
	}
	stored, err := runtime.options.Sessions.List(ctx, SessionFilter{Subject: principal.Subject, Project: runtime.options.Project.ID})
	if err != nil {
		return nil, err
	}
	now := runtime.options.Now().UTC()
	result := make([]WorkflowSessionSummary, 0, len(stored))
	for _, item := range stored {
		if err := validateStoredSession(item); err != nil {
			return nil, err
		}
		events, err := decodeWorkflowEvents(item.Events)
		if err != nil {
			return nil, err
		}
		summary := deriveWorkflowSummary(item.Metadata.ID, events)
		summary.Label = item.Metadata.Label
		if summary.Label == "" {
			summary.Label = workflowLabel(events)
		}
		if summary.Label == "" {
			summary.Label = workflowBodyLabel(events)
		}
		summary.Participant = item.Metadata.Participant
		if item.Metadata.UpdatedAt.After(summary.LastActivity) {
			summary.LastActivity = item.Metadata.UpdatedAt
		}
		if item.Metadata.Holder != nil {
			holder := *item.Metadata.Holder
			summary.Holder = &holder
			summary.HolderLive = holder.ExpiresAt.After(now)
		}
		result = append(result, summary)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].LastActivity.After(result[j].LastActivity) })
	return result, nil
}

func (a *Application) AbandonWorkflowSession(ctx context.Context, identity RequestIdentity, project ProjectID, request WorkflowResumeRequest, reason string) (WorkflowAbandonResult, error) {
	w, _, err := a.ResumeWorkflow(ctx, identity, project, request)
	if err != nil {
		return WorkflowAbandonResult{}, err
	}
	result := WorkflowAbandonResult{Abandoned: true, Session: w.ID(), Label: w.session.Label}
	for _, inst := range w.session.Instances() {
		if inst.Status != engine.StatusRunning {
			continue
		}
		if inst.ID != w.shell {
			result.Discarded = append(result.Discarded, WorkflowInstanceSummary{Instance: inst.ID, Procedure: inst.Spec.Canonical, Step: inst.Step})
			if marker, ok := workflowStoreString(inst.Store, "wipMarker"); ok {
				result.HeldMarkers = append(result.HeldMarkers, marker)
			}
		}
		if err := w.session.Abandon(inst.ID, reason); err != nil {
			return WorkflowAbandonResult{}, err
		}
	}
	if err := w.Release(ctx, identity); err != nil {
		return WorkflowAbandonResult{}, err
	}
	return result, nil
}

func (w *WorkflowSession) begin(ctx context.Context, identity RequestIdentity, chooser ChooserKind) error {
	w.setOperation(ctx, identity)
	req := w.client
	req.SessionID = w.ID()
	req.Chooser = chooser
	req.Takeover = false
	bound, err := w.app.BindSession(ctx, identity, w.project, req)
	if err != nil {
		return err
	}
	w.binding = bound.Binding
	return nil
}

func (w *WorkflowSession) setOperation(ctx context.Context, identity RequestIdentity) {
	w.ctx = ctx
	w.identity = identity
}

func (w *WorkflowSession) currentChooser(instance string) ChooserKind {
	inst, ok := w.session.Instance(instance)
	if !ok || inst.Status != engine.StatusRunning {
		return ChooserAgent
	}
	step := inst.Spec.StepByID[inst.Step]
	if step == nil {
		return ChooserAgent
	}
	switch step.Chooser {
	case engine.ChooserUser:
		return ChooserUser
	case engine.ChooserAgent:
		return ChooserAgent
	default:
		return ChooserGate
	}
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
		Session: w.ID(), Instance: serve.Instance, Procedure: serve.Procedure, Status: string(serve.Status),
		Step: serve.Step, Goal: serve.Goal, Instructions: serve.Instructions, Missing: serve.Missing,
		ReportSchema: serve.ReportSchema, Produced: serve.Produced, Diagnostics: append([]string(nil), serve.Diagnostics...), InstructionUnit: serve.UnitText,
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

func (g *workflowGraphs) Invalidate() { g.snapshot = nil }

type workflowSink struct{ workflow *WorkflowSession }

func (s *workflowSink) Append(event engine.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return s.workflow.appendStoredEvent(WorkflowEventCode, payload)
}

func (w *WorkflowSession) appendStoredEvent(code string, payload json.RawMessage) error {
	principal, runtime, err := w.app.resolve(w.ctx, w.identity, w.project, AccessRead)
	if err != nil {
		return err
	}
	stored, err := runtime.options.Sessions.Load(w.ctx, w.ID())
	if err != nil {
		return err
	}
	if err := verifyBinding(stored, w.binding); err != nil {
		return err
	}
	metadata := stored.Metadata
	metadata.UpdatedAt = runtime.options.Now().UTC().Round(0)
	if code == WorkflowEventCode {
		var event engine.Event
		if err := json.Unmarshal(payload, &event); err != nil {
			return err
		}
		if event.Event == engine.EventLabeled {
			metadata.Label, _ = event.Data["label"].(string)
		}
	}
	if metadata.Subject != principal.Subject {
		return &ApplicationError{Code: ErrorSessionOwnership, Message: "session subject changed"}
	}
	next, err := runtime.options.Sessions.Append(w.ctx, w.ID(), stored.Version, SessionAppend{
		Metadata: &metadata,
		Events:   []StoredEvent{{CodecVersion: SessionCodecVersion, Code: code, Payload: payload}},
	})
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
		if stored.CodecVersion != SessionCodecVersion {
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

func (w *WorkflowSession) loadStoredSession(ctx context.Context) (StoredSession, error) {
	_, runtime, err := w.app.resolve(ctx, w.identity, w.project, AccessRead)
	if err != nil {
		return StoredSession{}, err
	}
	stored, err := runtime.options.Sessions.Load(ctx, w.ID())
	if err != nil {
		return StoredSession{}, err
	}
	if err := verifyBinding(stored, w.binding); err != nil {
		return StoredSession{}, err
	}
	return stored, nil
}

func decodeWorkflowEvents(stored []StoredEvent) ([]engine.Event, error) {
	var events []engine.Event
	for _, item := range stored {
		if item.Code != WorkflowEventCode {
			continue
		}
		if item.CodecVersion != SessionCodecVersion {
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
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("sdd: reading session randomness: %v", err))
	}
	return SessionID("s_" + now.Format("20060102-150405") + "-" + hex.EncodeToString(buf))
}
