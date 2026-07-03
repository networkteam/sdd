package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
)

// framingLayout renders the session framing an /sdd skill session gets
// injected at start: aspirations, guiding directives, active focus, and
// participants.
const framingLayout = `aspirations,` +
	`kind(directive):intent(guiding):active:name("Guiding directives"):as-list,` +
	`focus,` +
	`participants`

// --- the loop -------------------------------------------------------------

type StartProcedureArgs struct {
	Canonical string         `json:"canonical" jsonschema:"the procedure to start, by its stable name (e.g. capture)"`
	Params    map[string]any `json:"params,omitempty" jsonschema:"typed start params per the procedure's declaration"`
	Label     string         `json:"label,omitempty" jsonschema:"short single-line subject label for the session (the dialogue); set it early, update when the subject sharpens"`
	Parent    string         `json:"parent,omitempty" jsonschema:"instance handle of the spawning instance, when this is a sub-move (e.g. a capture dispatched from an engage); records lineage in the session log"`
}

type NextArgs struct {
	Instance string `json:"instance" jsonschema:"instance handle from start_procedure"`
	// Report carries either state fields for the current step (per the served
	// report_schema) or a chooser answer {chooser, choice, userWords?, fields?}.
	Report map[string]any `json:"report" jsonschema:"state fields per the served report_schema, or a chooser answer object {chooser, choice, userWords?, fields?}"`
	Label  string         `json:"label,omitempty" jsonschema:"update the session's subject label when the dialogue's subject has sharpened"`
}

type AbandonArgs struct {
	Instance string `json:"instance" jsonschema:"instance handle to abandon"`
	Reason   string `json:"reason,omitempty" jsonschema:"why the instance is being abandoned"`
}

type AbandonResult struct {
	Abandoned    bool     `json:"abandoned"`
	HeldMarkers  []string `json:"held_markers,omitempty" jsonschema:"WIP markers the instance holds; left standing — resume later or close via groom"`
	Instructions string   `json:"instructions,omitempty"`
	OpenThreads  string   `json:"open_threads,omitempty" jsonschema:"parked work at this junction: this dialogue's other threads, then other open dialogues"`
}

type ChooserOptionResult struct {
	Choice  string   `json:"choice"`
	Collect []string `json:"collect,omitempty" jsonschema:"state fields this option carries (suffix ? = optional)"`
}

type ChooserResult struct {
	Kind    string                `json:"kind" jsonschema:"who answers: agent (advisory judgment) or user (relay their answer verbatim)"`
	Options []ChooserOptionResult `json:"options"`
}

// ServeResult is the loop's uniform response shape: where the instance
// stands, what advances it, and the material to work with.
type ServeResult struct {
	Session        string         `json:"session" jsonschema:"session handle; sessions survive restarts and resume via resume_session"`
	Instance       string         `json:"instance"`
	Procedure      string         `json:"procedure"`
	Status         string         `json:"status" jsonschema:"running, completed, or abandoned"`
	Step           string         `json:"step,omitempty"`
	Goal           string         `json:"goal" jsonschema:"one line: what advances the instance from here"`
	Instructions   string         `json:"instructions,omitempty"`
	Missing        []string       `json:"missing,omitempty" jsonschema:"required report fields not yet provided"`
	ReportSchema   map[string]any `json:"report_schema,omitempty" jsonschema:"JSON Schema for the current step's report"`
	PendingChooser *ChooserResult `json:"pending_chooser,omitempty"`
	Produced       map[string]any `json:"produced,omitempty" jsonschema:"engine-written results on completion (e.g. the created entry ID)"`
	Framing        string         `json:"framing,omitempty" jsonschema:"session framing (aspirations, directives, focus, participants); delivered once per agent session"`
	OpenThreads    string         `json:"open_threads,omitempty" jsonschema:"parked work, present at junctions only (session entry, completion, abandon): this dialogue's other threads, then other open dialogues"`
}

// --- sessions & staging ----------------------------------------------------

type ListSessionsArgs struct{}

type ListSessionsResult struct {
	Sessions []sessionDescriptor `json:"sessions"`
}

type ResumeSessionArgs struct {
	Session string `json:"session" jsonschema:"session handle from list_sessions"`
}

type ResumeSessionResult struct {
	Session      string        `json:"session"`
	Participant  string        `json:"participant,omitempty"`
	Label        string        `json:"label,omitempty" jsonschema:"the session's subject label, when one was recorded"`
	Open         []ServeResult `json:"open_instances" jsonschema:"current serve for every running instance"`
	Framing      string        `json:"framing,omitempty"`
	Instructions string        `json:"instructions,omitempty"`
	OpenThreads  string        `json:"open_threads,omitempty" jsonschema:"other open dialogues beyond this one"`
}

type StageAttachmentArgs struct {
	Name    string `json:"name" jsonschema:"target filename (plain name, no paths)"`
	Content string `json:"content,omitempty" jsonschema:"file content to stage (UTF-8 text)"`
	Path    string `json:"path,omitempty" jsonschema:"local file path to stage instead of inline content"`
}

type StageAttachmentResult struct {
	Handle string `json:"handle" jsonschema:"attachment handle; pass it in a report's attachments field"`
}

// --- free reads -------------------------------------------------------------

type SearchArgs struct {
	Terms             []string `json:"terms,omitempty" jsonschema:"text mode: regex terms combined with AND"`
	Query             string   `json:"query,omitempty" jsonschema:"vector mode: free-form phrase (requires a configured embedding provider); both together run hybrid"`
	Type              string   `json:"type,omitempty" jsonschema:"filter: s/signal or d/decision"`
	Layer             string   `json:"layer,omitempty" jsonschema:"filter: stg, cpt, tac, ops, prc"`
	Kind              string   `json:"kind,omitempty" jsonschema:"filter: entry kind"`
	IncludeSuperseded bool     `json:"include_superseded,omitempty"`
	Limit             int      `json:"limit,omitempty"`
	MaxCitations      *int     `json:"max_citations,omitempty" jsonschema:"citation lines per entry; 0 = headers only"`
}

type SearchResult struct {
	Results string `json:"results" jsonschema:"matching entries with citations"`
}

type ViewArgs struct {
	Layout string `json:"layout" jsonschema:"sdd view layout pipeline, e.g. 'active:as-counts' or 'top(15)'"`
}

type ViewResult struct {
	Sections string `json:"sections"`
}

type ShowArgs struct {
	IDs  []string `json:"ids" jsonschema:"full entry IDs to show"`
	Up   int      `json:"up,omitempty" jsonschema:"upstream chain depth; default 2"`
	Down int      `json:"down,omitempty" jsonschema:"downstream chain depth; default 1"`
}

type ShowResult struct {
	Entries string `json:"entries"`
}

type ReadAttachmentArgs struct {
	ID       string `json:"id" jsonschema:"full ID of the entry whose attachment to read"`
	Name     string `json:"name,omitempty" jsonschema:"attachment filename; optional when the entry has exactly one"`
	Offset   int64  `json:"offset,omitempty" jsonschema:"byte position to continue from (next_offset of the previous page)"`
	MaxBytes int    `json:"max_bytes,omitempty" jsonschema:"page size cap; default 65536"`
}

type ReadAttachmentResult struct {
	Name       string   `json:"name"`
	Content    string   `json:"content"`
	Offset     int64    `json:"offset"`
	NextOffset int64    `json:"next_offset,omitempty"`
	TotalBytes int64    `json:"total_bytes"`
	More       bool     `json:"more"`
	Available  []string `json:"available" jsonschema:"the entry's attachment filenames"`
	Path       string   `json:"path,omitempty" jsonschema:"absolute filesystem path; present only for local (stdio) clients, which may read the file directly instead of paging"`
}

type InfoArgs struct{}

type InfoResult struct {
	Participant string `json:"participant,omitempty" jsonschema:"configured local participant (canonical name)"`
	Language    string `json:"language,omitempty" jsonschema:"configured graph language; empty = English"`
	Search      string `json:"search" jsonschema:"available retrieval modes: text or vector,text"`
	Version     string `json:"version,omitempty"`
}

type RegistryArgs struct {
	Class string `json:"class,omitempty" jsonschema:"filter: predicate, query, or command"`
}

type RegistryFuncResult struct {
	Name   string   `json:"name"`
	Class  string   `json:"class"`
	Doc    string   `json:"doc"`
	Reads  []string `json:"reads,omitempty"`
	Writes []string `json:"writes,omitempty"`
}

type RegistryResult struct {
	Functions []RegistryFuncResult `json:"functions"`
}

func (s *Server) registerTools() {
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "start_procedure",
		Description: "Start a procedure instance (a playbook move such as capture). Returns the current " +
			"step's instructions, the report schema to answer with, and the goal that advances it. " +
			"This is the only path that leads to graph writes — writes happen inside procedure " +
			"transitions, never through a direct tool.",
	}, s.startProcedure)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "next",
		Description: "Advance a procedure instance: send state fields per the served report_schema, or " +
			"answer a pending chooser with {chooser, choice, userWords?, fields?}. User choosers must " +
			"carry the user's answer relayed verbatim in userWords.",
	}, s.next)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "abandon",
		Description: "Abandon a running procedure instance, with a reason. Nothing is cleaned up " +
			"implicitly: held WIP markers are surfaced and left standing for resume or grooming.",
	}, s.abandon)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "list_sessions",
		Description: "List open dialogue sessions (running procedure instances) with participant, anchor, and last activity.",
	}, s.listSessions)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "resume_session",
		Description: "Resume a persisted session by handle: replays its log and returns the current serve " +
			"for every running instance. Step position and evidence persist across restarts.",
	}, s.resumeSession)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "stage_attachment",
		Description: "Stage a file into the session scratch and get back a handle to pass in a report's " +
			"attachments field. Never a graph write — the write gate materializes staged files with the entry.",
	}, s.stageAttachment)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "search",
		Description: "Search graph entries: terms (text/regex), query (semantic phrase), or both (hybrid). " +
			"Free read, never gated.",
	}, s.search)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "view",
		Description: "Run an sdd view layout pipeline over the graph — overview sections, topic counts, " +
			"ranked lists. Free read, never gated.",
	}, s.view)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "show",
		Description: "Read entries in full with their upstream and downstream reference chains. Use " +
			"whenever the dialogue touches a specific entry — summaries are pointers, not facts.",
	}, s.show)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "read_attachment",
		Description: "Read an entry's attachment content, paged. Attachments are resolved by entry ID and " +
			"filename — never derive storage paths.",
	}, s.readAttachment)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "info",
		Description: "Session framing header: local participant, configured language, available search modes.",
	}, s.info)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "registry",
		Description: "Engine function contracts (predicates, queries, commands) — what procedure spec authors consult.",
	}, s.registryDocs)
}

// ensureSession returns the SDD session bound to the calling MCP session,
// creating and binding a fresh one on first stateful use.
func (s *Server) ensureSession(ms *mcp.ServerSession) (*shellSession, error) {
	if ss := s.sessions.bound(ms); ss != nil {
		return ss, nil
	}
	now := time.Now()
	id := newSessionID(now)
	logFile, err := s.sessions.openLog(id)
	if err != nil {
		return nil, err
	}

	participant := ""
	if info, err := s.finder.Info(query.InfoQuery{}); err == nil {
		participant = info.LocalParticipant
	}

	ss := &shellSession{
		id:           id,
		participant:  participant,
		logFile:      logFile,
		served:       map[string]bool{},
		lastActivity: now,
	}
	registry, err := s.buildRegistry(ss)
	if err != nil {
		ss.close()
		return nil, err
	}
	graph, err := s.finder.LoadGraph(s.graphDir)
	if err != nil {
		ss.close()
		return nil, fmt.Errorf("loading graph: %v", err)
	}
	ss.engine = engine.New(registry, graph)
	ss.sess = ss.engine.NewSession(id, participant, engine.NewWriterSink(logFile))
	s.sessions.bind(ms, ss)
	return ss, nil
}

func (s *Server) startProcedure(ctx context.Context, req *mcp.CallToolRequest, args StartProcedureArgs) (*mcp.CallToolResult, ServeResult, error) {
	if strings.TrimSpace(args.Canonical) == "" {
		return nil, ServeResult{}, toolError("canonical is required")
	}
	// A fresh binding means this call is the session entry — a junction that
	// carries the open-threads block alongside the framing.
	entering := s.sessions.bound(req.Session) == nil
	ss, err := s.ensureSession(req.Session)
	if err != nil {
		return nil, ServeResult{}, err
	}
	ss.touch(time.Now())
	if err := s.refreshGraph(ss); err != nil {
		return nil, ServeResult{}, err
	}

	if err := applyLabel(ss, args.Label); err != nil {
		return nil, ServeResult{}, err
	}

	spec, err := s.loadProcedure(ss, args.Canonical)
	if err != nil {
		return nil, ServeResult{}, err
	}
	serve, err := ss.sess.Start(spec, args.Params, args.Parent)
	if err != nil {
		return nil, ServeResult{}, err
	}
	res := s.toServeResult(ss, serve)
	if entering && res.OpenThreads == "" {
		res.OpenThreads = s.openThreadsBlock(ss, true)
	}
	return nil, res, nil
}

// maxLabelLen caps session labels — a label is a list row, not a summary.
const maxLabelLen = 120

// applyLabel validates and records an agent-supplied session label. Empty
// means "no update"; the engine skips appending an unchanged value.
func applyLabel(ss *shellSession, label string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil
	}
	if strings.ContainsAny(label, "\n\r") {
		return toolError("label must be a single line")
	}
	if utf8.RuneCountInString(label) > maxLabelLen {
		return toolError("label exceeds %d characters — it is a list row, not a summary", maxLabelLen)
	}
	ss.sess.SetLabel(label)
	return nil
}

func (s *Server) next(ctx context.Context, req *mcp.CallToolRequest, args NextArgs) (*mcp.CallToolResult, ServeResult, error) {
	ss := s.sessions.bound(req.Session)
	if ss == nil {
		return nil, ServeResult{}, toolError("no session is bound — call start_procedure or resume_session first")
	}
	if args.Instance == "" {
		return nil, ServeResult{}, toolError("instance is required")
	}
	if len(args.Report) == 0 {
		return nil, ServeResult{}, toolError("report is required: state fields per the served report_schema, or a chooser answer {chooser, choice, userWords?, fields?}")
	}
	ss.touch(time.Now())
	if err := s.refreshGraph(ss); err != nil {
		return nil, ServeResult{}, err
	}
	if err := applyLabel(ss, args.Label); err != nil {
		return nil, ServeResult{}, err
	}

	var serve *engine.Serve
	var err error
	if chooser, choice, ok := chooserAnswer(args.Report); ok {
		fields, _ := args.Report["fields"].(map[string]any)
		userWords, _ := args.Report["userWords"].(string)
		serve, err = ss.sess.Answer(args.Instance, chooser, choice, fields, userWords)
	} else {
		serve, err = ss.sess.Report(args.Instance, args.Report)
	}
	if err != nil {
		return nil, ServeResult{}, err
	}
	return nil, s.toServeResult(ss, serve), nil
}

// chooserAnswer detects the chooser-answer form of a report: an object
// carrying chooser + choice. The two key names are reserved — no procedure
// state field may use them (spec authors: name fields otherwise).
func chooserAnswer(report map[string]any) (chooser, choice string, ok bool) {
	chooser, hasChooser := report["chooser"].(string)
	choice, hasChoice := report["choice"].(string)
	return chooser, choice, hasChooser && hasChoice && chooser != "" && choice != ""
}

func (s *Server) abandon(ctx context.Context, req *mcp.CallToolRequest, args AbandonArgs) (*mcp.CallToolResult, AbandonResult, error) {
	ss := s.sessions.bound(req.Session)
	if ss == nil {
		return nil, AbandonResult{}, toolError("no session is bound — call start_procedure or resume_session first")
	}
	if args.Instance == "" {
		return nil, AbandonResult{}, toolError("instance is required")
	}
	ss.touch(time.Now())

	var held []string
	if inst, ok := ss.sess.Instance(args.Instance); ok {
		if marker, present := storeString(inst.Store, "wipMarker"); present {
			held = append(held, marker)
		}
	}
	if err := ss.sess.Abandon(args.Instance, args.Reason); err != nil {
		return nil, AbandonResult{}, err
	}
	res := AbandonResult{Abandoned: true, HeldMarkers: held}
	if len(held) > 0 {
		res.Instructions = "The instance holds WIP markers, left standing by design. Tell the user: resume the work later or close the markers through grooming."
	}
	res.OpenThreads = s.openThreadsBlock(ss, true)
	return nil, res, nil
}

func (s *Server) listSessions(ctx context.Context, req *mcp.CallToolRequest, _ ListSessionsArgs) (*mcp.CallToolResult, ListSessionsResult, error) {
	descs, err := s.sessions.listOpenSessions()
	if err != nil {
		return nil, ListSessionsResult{}, err
	}
	if current := s.sessions.bound(req.Session); current != nil {
		for i := range descs {
			if descs[i].Session == current.id {
				descs[i].Current = true
			}
		}
	}
	return nil, ListSessionsResult{Sessions: descs}, nil
}

func (s *Server) resumeSession(ctx context.Context, req *mcp.CallToolRequest, args ResumeSessionArgs) (*mcp.CallToolResult, ResumeSessionResult, error) {
	if args.Session == "" {
		return nil, ResumeSessionResult{}, toolError("session is required")
	}
	if live := s.sessions.lookupID(args.Session); live != nil && live.logFile != nil {
		// The session is already live in this server process (possibly bound
		// to another connection). Rebind rather than replaying over an open
		// log; served-instruction memory resets for the new consumer.
		live.served = map[string]bool{}
		live.framed = false
		live.openThreadsIntroduced = false
		s.sessions.bind(req.Session, live)
		res, err := s.resumeResult(live)
		return nil, res, err
	}

	events, err := s.sessions.readLog(args.Session)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ResumeSessionResult{}, toolError("unknown session %q — list_sessions shows what is resumable", args.Session)
		}
		return nil, ResumeSessionResult{}, err
	}
	desc := deriveDescriptor(args.Session, events)

	ss := &shellSession{
		id:           args.Session,
		participant:  desc.Participant,
		served:       map[string]bool{},
		lastActivity: time.Now(),
	}
	registry, err := s.buildRegistry(ss)
	if err != nil {
		return nil, ResumeSessionResult{}, err
	}
	graph, err := s.finder.LoadGraph(s.graphDir)
	if err != nil {
		return nil, ResumeSessionResult{}, fmt.Errorf("loading graph: %v", err)
	}
	ss.engine = engine.New(registry, graph)

	resolve := func(canonical string) (*engine.Spec, error) {
		return s.loadProcedure(ss, canonical)
	}
	logFile, err := s.sessions.openLog(args.Session)
	if err != nil {
		return nil, ResumeSessionResult{}, err
	}
	sess, err := ss.engine.ReplaySession(args.Session, desc.Participant, events, resolve, engine.NewWriterSink(logFile))
	if err != nil {
		_ = logFile.Close()
		return nil, ResumeSessionResult{}, fmt.Errorf("replaying session %s: %v", args.Session, err)
	}
	ss.logFile = logFile
	ss.sess = sess
	s.sessions.bind(req.Session, ss)
	res, err := s.resumeResult(ss)
	return nil, res, err
}

func (s *Server) resumeResult(ss *shellSession) (ResumeSessionResult, error) {
	res := ResumeSessionResult{
		Session:      ss.id,
		Participant:  ss.sess.Participant,
		Label:        ss.sess.Label,
		Instructions: resumeInstructions,
	}
	for _, inst := range ss.sess.Instances() {
		if inst.Status != engine.StatusRunning {
			continue
		}
		serve, err := ss.sess.Serve(inst.ID)
		if err != nil {
			return ResumeSessionResult{}, err
		}
		res.Open = append(res.Open, s.toServeResult(ss, serve))
	}
	// Framing rides the resume result itself, not the per-instance serves.
	for i := range res.Open {
		res.Open[i].Framing = ""
	}
	ss.framed = false
	res.Framing = s.framingFor(ss)
	// Resume is a junction: surface the other open dialogues (this session's
	// own threads are already the Open list above).
	res.OpenThreads = s.openThreadsBlock(ss, false)
	return res, nil
}

func (s *Server) stageAttachment(ctx context.Context, req *mcp.CallToolRequest, args StageAttachmentArgs) (*mcp.CallToolResult, StageAttachmentResult, error) {
	if err := validAttachmentName(args.Name); err != nil {
		return nil, StageAttachmentResult{}, toolError("name: %v", err)
	}
	if (args.Content == "") == (args.Path == "") {
		return nil, StageAttachmentResult{}, toolError("pass exactly one of content or path")
	}
	ss, err := s.ensureSession(req.Session)
	if err != nil {
		return nil, StageAttachmentResult{}, err
	}
	ss.touch(time.Now())

	dir, err := s.sessions.stagingDir(ss.id)
	if err != nil {
		return nil, StageAttachmentResult{}, err
	}
	content := []byte(args.Content)
	if args.Path != "" {
		content, err = os.ReadFile(args.Path)
		if err != nil {
			return nil, StageAttachmentResult{}, toolError("reading %s: %v", args.Path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, args.Name), content, 0o644); err != nil {
		return nil, StageAttachmentResult{}, fmt.Errorf("staging attachment: %w", err)
	}
	return nil, StageAttachmentResult{Handle: args.Name}, nil
}

func (s *Server) search(ctx context.Context, _ *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, SearchResult, error) {
	if len(args.Terms) == 0 && args.Query == "" {
		return nil, SearchResult{}, toolError("pass terms (text mode), query (vector mode), or both (hybrid)")
	}
	if args.Query != "" && !s.vector {
		return nil, SearchResult{}, toolError("query needs a configured embedding provider — this server has text mode only; use terms")
	}
	if s.searcher == nil {
		return nil, SearchResult{}, toolError("search is not configured on this server")
	}
	graph, err := s.finder.LoadGraph(s.graphDir)
	if err != nil {
		return nil, SearchResult{}, toolError("loading graph: %v", err)
	}

	sq := query.SearchQuery{
		Graph:                graph,
		Terms:                args.Terms,
		Phrase:               args.Query,
		IncludeSuperseded:    args.IncludeSuperseded,
		Limit:                args.Limit,
		MaxCitationsPerEntry: query.DefaultMaxCitationsPerEntry,
	}
	if args.MaxCitations != nil {
		sq.MaxCitationsPerEntry = *args.MaxCitations
	}
	if args.Type != "" {
		t, err := parseEntryType(args.Type)
		if err != nil {
			return nil, SearchResult{}, err
		}
		sq.Filter.Type = t
	}
	if args.Layer != "" {
		l, err := parseLayer(args.Layer)
		if err != nil {
			return nil, SearchResult{}, err
		}
		sq.Filter.Layer = l
	}
	if args.Kind != "" {
		sq.Filter.Kind = model.Kind(args.Kind)
	}

	res, err := s.searcher.Search(ctx, sq)
	if err != nil {
		return nil, SearchResult{}, toolError("searching: %v", err)
	}
	var sb strings.Builder
	presenters.RenderSearch(&sb, res, graph)
	out := sb.String()
	if strings.TrimSpace(out) == "" {
		out = "(no entries matched — try another phrasing, or proceed if the topic is genuinely new)"
	}
	return nil, SearchResult{Results: out}, nil
}

func (s *Server) view(ctx context.Context, _ *mcp.CallToolRequest, args ViewArgs) (*mcp.CallToolResult, ViewResult, error) {
	if strings.TrimSpace(args.Layout) == "" {
		return nil, ViewResult{}, toolError("layout is required")
	}
	graph, err := s.finder.LoadGraph(s.graphDir)
	if err != nil {
		return nil, ViewResult{}, toolError("loading graph: %v", err)
	}
	out, err := s.renderView(graph, args.Layout)
	if err != nil {
		return nil, ViewResult{}, err
	}
	return nil, ViewResult{Sections: out}, nil
}

func (s *Server) show(ctx context.Context, _ *mcp.CallToolRequest, args ShowArgs) (*mcp.CallToolResult, ShowResult, error) {
	if len(args.IDs) == 0 {
		return nil, ShowResult{}, toolError("ids is required")
	}
	graph, err := s.finder.LoadGraph(s.graphDir)
	if err != nil {
		return nil, ShowResult{}, toolError("loading graph: %v", err)
	}
	up := args.Up
	if up == 0 {
		up = query.DefaultUpDepth
	}
	down := args.Down
	if down == 0 {
		down = query.DefaultDownDepth
	}
	out, err := s.renderShow(graph, args.IDs, up, down)
	if err != nil {
		return nil, ShowResult{}, err
	}
	return nil, ShowResult{Entries: out}, nil
}

func (s *Server) readAttachment(ctx context.Context, _ *mcp.CallToolRequest, args ReadAttachmentArgs) (*mcp.CallToolResult, ReadAttachmentResult, error) {
	if args.ID == "" {
		return nil, ReadAttachmentResult{}, toolError("id is required")
	}
	graph, err := s.finder.LoadGraph(s.graphDir)
	if err != nil {
		return nil, ReadAttachmentResult{}, toolError("loading graph: %v", err)
	}
	res, err := s.finder.ReadAttachment(query.ReadAttachmentQuery{
		Graph:    graph,
		GraphDir: s.graphDir,
		EntryID:  args.ID,
		Name:     args.Name,
		Offset:   args.Offset,
		MaxBytes: args.MaxBytes,
	})
	if err != nil {
		return nil, ReadAttachmentResult{}, err
	}
	out := ReadAttachmentResult{
		Name:       res.Name,
		Content:    res.Content,
		Offset:     res.Offset,
		NextOffset: res.NextOffset,
		TotalBytes: res.TotalBytes,
		More:       res.More,
		Available:  res.Available,
	}
	if s.local {
		out.Path = res.Path
	}
	return nil, out, nil
}

func (s *Server) info(ctx context.Context, _ *mcp.CallToolRequest, _ InfoArgs) (*mcp.CallToolResult, InfoResult, error) {
	info, err := s.finder.Info(query.InfoQuery{})
	if err != nil {
		return nil, InfoResult{}, err
	}
	return nil, InfoResult{
		Participant: info.LocalParticipant,
		Language:    info.Language,
		Search:      info.Search,
		Version:     s.version,
	}, nil
}

func (s *Server) registryDocs(ctx context.Context, _ *mcp.CallToolRequest, args RegistryArgs) (*mcp.CallToolResult, RegistryResult, error) {
	var class engine.FuncClass
	switch args.Class {
	case "", string(engine.ClassPredicate), string(engine.ClassQuery), string(engine.ClassCommand):
		class = engine.FuncClass(args.Class)
	default:
		return nil, RegistryResult{}, toolError("unknown class %q: predicate, query, or command", args.Class)
	}
	res := RegistryResult{}
	for _, doc := range s.docsRegistry.Docs(class) {
		res.Functions = append(res.Functions, RegistryFuncResult{
			Name:   doc.Name,
			Class:  string(doc.Class),
			Doc:    doc.Doc,
			Reads:  doc.Reads,
			Writes: doc.Writes,
		})
	}
	return nil, res, nil
}

// --- serve conversion --------------------------------------------------------

// toServeResult converts an engine serve into the tool response, applying
// served-instruction memory (full unit text once per agent session, a
// one-line reminder after) and injecting the session framing on the first
// serve a bound agent sees.
func (s *Server) toServeResult(ss *shellSession, serve *engine.Serve) ServeResult {
	res := ServeResult{
		Session:      ss.id,
		Instance:     serve.Instance,
		Procedure:    serve.Procedure,
		Status:       string(serve.Status),
		Step:         serve.Step,
		Goal:         serve.Goal,
		Instructions: serve.Instructions,
		Missing:      serve.Missing,
		ReportSchema: serve.ReportSchema,
		Produced:     serve.Produced,
	}
	if serve.Unit != "" {
		key := serve.Instance + "/" + serve.Unit
		if ss.served[key] {
			reminder := fmt.Sprintf("(step %s instructions were served earlier this session — follow them; goal: %s)", serve.Step, serve.Goal)
			res.Instructions = engine.ComposeInstructions(reminder, serve.Diagnostics)
		} else {
			ss.served[key] = true
		}
	}
	if serve.Status != engine.StatusRunning {
		// A terminal serve is a junction — completion or procedure-exit
		// abandonment both hand the dialogue back.
		res.OpenThreads = s.openThreadsBlock(ss, true)
	}
	if serve.Chooser != nil {
		ch := &ChooserResult{Kind: string(serve.Chooser.Kind)}
		for _, o := range serve.Chooser.Options {
			opt := ChooserOptionResult{Choice: o.Choice}
			for _, cf := range o.Collect {
				name := cf.Name
				if cf.Optional {
					name += "?"
				}
				opt.Collect = append(opt.Collect, name)
			}
			ch.Options = append(ch.Options, opt)
		}
		res.PendingChooser = ch
	}
	res.Framing = s.framingFor(ss)
	return res
}

// framingFor renders the session framing once per bound agent consumer.
func (s *Server) framingFor(ss *shellSession) string {
	if ss.framed {
		return ""
	}
	ss.framed = true

	var sb strings.Builder
	if info, err := s.finder.Info(query.InfoQuery{}); err == nil {
		fmt.Fprintf(&sb, "Local participant: %s\n", info.LocalParticipant)
		if info.Language != "" {
			fmt.Fprintf(&sb, "Language: %s\n", info.Language)
		}
		fmt.Fprintf(&sb, "Search: %s\n\n", info.Search)
	}
	framing, err := s.renderView(ss.engine.Graph, framingLayout)
	if err != nil {
		// Framing is orientation, not a gate — serve the loop anyway.
		return sb.String()
	}
	return sb.String() + framing
}

// loadProcedure resolves a canonical to its execution head and loads the
// spec against the session's registry.
func (s *Server) loadProcedure(ss *shellSession, canonical string) (*engine.Spec, error) {
	entry := ss.engine.Graph.ResolveProcedure(canonical)
	if entry == nil {
		available := make([]string, 0)
		for _, chain := range ss.engine.Graph.ProcedureChains() {
			if chain.Head != nil && chain.Head.Canonical != "" {
				available = append(available, chain.Head.Canonical)
			}
		}
		return nil, toolError("no procedure %q (available: %s)", canonical, strings.Join(available, ", "))
	}
	return engine.LoadSpec(entry, ss.engine.Registry)
}

// renderView parses a layout, executes it against the graph, and renders
// the plain-text result — the same pipeline `sdd view --layout` drives.
func (s *Server) renderView(graph *model.Graph, layout string) (string, error) {
	parsed, err := query.ParseLayout(layout)
	if err != nil {
		return "", fmt.Errorf("parsing layout %q: %w", layout, err)
	}
	parsed, err = query.ExpandMacros(parsed)
	if err != nil {
		return "", fmt.Errorf("expanding layout %q: %w", layout, err)
	}
	res, err := s.finder.View(query.ViewQuery{
		Graph:    graph,
		Layout:   parsed,
		GraphDir: s.graphDir,
	})
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	presenters.RenderView(&sb, res)
	return sb.String(), nil
}

// renderShow renders entries with chains as plain markdown.
func (s *Server) renderShow(graph *model.Graph, ids []string, up, down int) (string, error) {
	res, err := s.finder.Show(query.ShowQuery{
		Graph:     graph,
		IDs:       ids,
		UpDepth:   up,
		DownDepth: down,
	})
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	presenters.RenderShow(&sb, res, presenters.ShowOptions{})
	return sb.String(), nil
}

func parseEntryType(in string) (model.EntryType, error) {
	if t, ok := model.TypeFromAbbrev[in]; ok {
		return t, nil
	}
	t := model.EntryType(in)
	if _, ok := model.TypeAbbrev[t]; ok {
		return t, nil
	}
	return "", toolError("invalid type %q: use s (signal) or d (decision)", in)
}

func parseLayer(in string) (model.Layer, error) {
	if l, ok := model.LayerFromAbbrev[in]; ok {
		return l, nil
	}
	l := model.Layer(in)
	if _, ok := model.LayerAbbrev[l]; ok {
		return l, nil
	}
	return "", toolError("invalid layer %q: use stg, cpt, tac, ops, or prc", in)
}
