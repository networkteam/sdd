package mcpserver

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
)

// briefingLayout mirrors the /sdd-catchup data injections so a connecting
// agent starts from the same picture a skill-equipped session gets.
const briefingLayout = `aspirations,` +
	`focus,` +
	`kind(done):rank(by(date)):n(10):name("Recent done"):as-list,` +
	`kind(plan,activity):active:rank(heat(exp-7d)):n(8):expand(refs(inactive)):name("Active and hot"):as-list,` +
	`kind(gap,question,insight):active:rank(heat(exp-14d)):n(15):name("Open and warm"):as-list`

const topicsLayout = `active:as-counts`

// preflightTimeout matches the CLI default for sdd new.
const preflightTimeout = 2 * time.Minute

type OpenSessionArgs struct{}

type OpenSessionResult struct {
	SessionToken string `json:"session_token" jsonschema:"token identifying this dialogue session; pass it to sdd_ground and sdd_capture"`
	Participant  string `json:"participant,omitempty" jsonschema:"the local human participant configured for this graph"`
	Language     string `json:"language,omitempty" jsonschema:"configured graph language; entries are authored in it"`
	Briefing     string `json:"briefing" jsonschema:"current graph state: aspirations, focus, recent completions, active work, open signals"`
	Instructions string `json:"instructions"`
}

type GroundArgs struct {
	SessionToken string `json:"session_token" jsonschema:"token from sdd_open_session"`
	Topic        string `json:"topic" jsonschema:"short phrase naming what the dialogue is about, e.g. 'pre-flight verdict oscillation'"`
}

type GroundResult struct {
	RelatedEntries string `json:"related_entries" jsonschema:"entries related to the topic, with match citations"`
	TopicsInUse    string `json:"topics_in_use" jsonschema:"topic labels already in use across active entries, with entry counts"`
	Instructions   string `json:"instructions"`
}

type RefArg struct {
	ID   string `json:"id" jsonschema:"full ID of the referenced entry"`
	Kind string `json:"kind" jsonschema:"why the reference exists: grounded-in, builds-on, refines, addresses, surfaces, surfaced-by, depends-on, required-by, or related"`
	Desc string `json:"desc,omitempty" jsonschema:"optional one-line why for this reference"`
}

type CaptureArgs struct {
	SessionToken string   `json:"session_token" jsonschema:"token from sdd_open_session"`
	Type         string   `json:"type" jsonschema:"entry type: s (signal = something noticed) or d (decision = something committed to)"`
	Layer        string   `json:"layer" jsonschema:"thinking depth: stg (why/direction), cpt (approach), tac (structure/trade-offs), ops (steps), prc (how we work)"`
	Kind         string   `json:"kind,omitempty" jsonschema:"signal kinds: gap (default), fact, question, insight, done, actor, annotation; decision kinds: directive (default), activity, plan, contract, aspiration, role, focus, procedure"`
	Description  string   `json:"description" jsonschema:"the entry body; first sentence must work as a standalone summary; fold dialogue reasoning in"`
	Refs         []RefArg `json:"refs,omitempty" jsonschema:"references connecting this entry's reasoning to existing entries"`
	Closes       []string `json:"closes,omitempty" jsonschema:"entry IDs this entry resolves or fulfills"`
	Supersedes   []string `json:"supersedes,omitempty" jsonschema:"entry IDs this entry replaces"`
	Topics       []string `json:"topics,omitempty" jsonschema:"topic labels; reuse labels from sdd_ground's topics_in_use when one fits"`
	Confidence   string   `json:"confidence,omitempty" jsonschema:"high (strong conviction), medium (reasonable but unvalidated), or low (hypothesis)"`
	Intent       string   `json:"intent,omitempty" jsonschema:"directive lifecycle posture: pending (demands follow-up), guiding (standing context), or settled (born terminal); required on directives, rejected on other kinds"`
	Participants []string `json:"participants,omitempty" jsonschema:"canonical participant names; omit to default to the configured local participant"`
	// SkipPreflight bypasses the validation gate. The instructions tell the
	// agent to only set it on explicit human direction; exposing it at all is
	// part of the experiment (does the agent reach for it under pushback?).
	SkipPreflight bool `json:"skip_preflight,omitempty" jsonschema:"bypass validation; only on explicit user direction after seeing the findings"`
}

type Finding struct {
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Observation string `json:"observation"`
}

type CaptureResult struct {
	Created      bool      `json:"created"`
	Blocked      bool      `json:"blocked"`
	ID           string    `json:"id,omitempty"`
	Path         string    `json:"path,omitempty"`
	Summary      string    `json:"summary,omitempty" jsonschema:"generated summary; verify it against what the user confirmed"`
	Findings     []Finding `json:"findings,omitempty"`
	Instructions string    `json:"instructions"`
}

type ShowEntryArgs struct {
	IDs       []string `json:"ids" jsonschema:"entry IDs to show (full IDs preferred)"`
	UpDepth   int      `json:"up_depth,omitempty" jsonschema:"upstream (grounding) expansion depth; default 2"`
	DownDepth int      `json:"down_depth,omitempty" jsonschema:"downstream (consumers) expansion depth; default 1"`
}

type ShowEntryResult struct {
	Entries      string `json:"entries" jsonschema:"full entry content with upstream and downstream reference trees"`
	Instructions string `json:"instructions"`
}

func (s *Server) registerTools() {
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "sdd_open_session",
		Description: "Open an SDD dialogue session. Returns the project briefing data " +
			"(focus, recent work, open signals), session framing, and the session token " +
			"required by sdd_ground and sdd_capture. Call this first, once per conversation.",
	}, s.openSession)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "sdd_ground",
		Description: "Research the graph around a topic before proposing an entry: returns " +
			"related entries and the topic labels already in use. Required before sdd_capture " +
			"can run in a session. Call again with different phrasings to widen.",
	}, s.ground)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "sdd_capture",
		Description: "Write a new entry to the graph after the user explicitly confirmed the " +
			"proposal in dialogue. Validation runs first and high-severity findings block " +
			"creation; the result carries findings and next-step instructions either way.",
	}, s.capture)

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name: "sdd_show_entry",
		Description: "Read one or more entries in full, each with its upstream (grounding) and " +
			"downstream (consumers) reference chains. Use whenever the dialogue touches a " +
			"specific entry — summaries are pointers, not facts.",
	}, s.showEntry)
}

func (s *Server) openSession(ctx context.Context, _ *mcp.CallToolRequest, _ OpenSessionArgs) (*mcp.CallToolResult, OpenSessionResult, error) {
	graph, err := s.finder.LoadGraph(s.graphDir)
	if err != nil {
		return nil, OpenSessionResult{}, toolError("loading graph: %v", err)
	}

	info, err := s.finder.Info(query.InfoQuery{})
	if err != nil {
		return nil, OpenSessionResult{}, toolError("reading session info: %v", err)
	}

	briefing, err := s.renderView(graph, briefingLayout)
	if err != nil {
		return nil, OpenSessionResult{}, toolError("rendering briefing: %v", err)
	}

	sess := s.sessions.open(time.Now())
	return nil, OpenSessionResult{
		SessionToken: sess.token,
		Participant:  info.LocalParticipant,
		Language:     info.Language,
		Briefing:     briefing,
		Instructions: openSessionInstructions,
	}, nil
}

func (s *Server) ground(ctx context.Context, _ *mcp.CallToolRequest, args GroundArgs) (*mcp.CallToolResult, GroundResult, error) {
	if strings.TrimSpace(args.Topic) == "" {
		return nil, GroundResult{}, toolError("topic is required")
	}

	graph, err := s.finder.LoadGraph(s.graphDir)
	if err != nil {
		return nil, GroundResult{}, toolError("loading graph: %v", err)
	}

	related := "(search is not configured on this server — rely on the briefing and sdd_show_entry)"
	if s.searcher != nil {
		sq := query.SearchQuery{
			Graph:                graph,
			Limit:                10,
			MaxCitationsPerEntry: 2,
		}
		if s.vector {
			sq.Phrase = args.Topic
		} else {
			sq.Terms = strings.Fields(args.Topic)
		}
		res, err := s.searcher.Search(ctx, sq)
		if err != nil {
			return nil, GroundResult{}, toolError("searching graph: %v", err)
		}
		var sb strings.Builder
		presenters.RenderSearch(&sb, res, graph)
		related = sb.String()
		if strings.TrimSpace(related) == "" {
			related = "(no entries matched this topic phrasing — try another phrasing, or proceed if the topic is genuinely new)"
		}
	}

	topics, err := s.renderView(graph, topicsLayout)
	if err != nil {
		return nil, GroundResult{}, toolError("rendering topics: %v", err)
	}

	// Mark grounded only after the data is assembled: a failed ground call
	// must not open the capture gate.
	if !s.sessions.markGrounded(args.SessionToken, time.Now()) {
		return nil, GroundResult{}, toolError("unknown session token — call sdd_open_session first")
	}

	return nil, GroundResult{
		RelatedEntries: related,
		TopicsInUse:    topics,
		Instructions:   groundInstructions,
	}, nil
}

func (s *Server) capture(ctx context.Context, _ *mcp.CallToolRequest, args CaptureArgs) (*mcp.CallToolResult, CaptureResult, error) {
	sess := s.sessions.lookup(args.SessionToken)
	if sess == nil {
		return nil, CaptureResult{}, toolError("unknown session token — call sdd_open_session first")
	}
	// The gate: enforced structurally, not by instruction. A session that
	// never grounded cannot write, regardless of what the client argues.
	if !sess.grounded {
		return nil, CaptureResult{
			Blocked: true,
			Findings: []Finding{{
				Severity:    "high",
				Category:    "grounding-gate",
				Observation: "No grounding call happened in this session before capture.",
			}},
			Instructions: captureGateInstructions,
		}, nil
	}

	entryType, err := parseEntryType(args.Type)
	if err != nil {
		return nil, CaptureResult{}, err
	}
	layer, err := parseLayer(args.Layer)
	if err != nil {
		return nil, CaptureResult{}, err
	}

	refs := make([]model.Ref, 0, len(args.Refs))
	for _, r := range args.Refs {
		refs = append(refs, model.Ref{ID: r.ID, Kind: model.RefKind(r.Kind), Desc: r.Desc})
	}

	participants := args.Participants
	if len(participants) == 0 {
		if info, err := s.finder.Info(query.InfoQuery{}); err == nil && info.LocalParticipant != "" {
			participants = []string{info.LocalParticipant}
		}
	}

	var (
		createdID      string
		createdSummary string
		pfFindings     []Finding
	)
	cmd := &command.NewEntryCmd{
		Type:             entryType,
		Layer:            layer,
		Kind:             model.Kind(args.Kind),
		Intent:           args.Intent,
		Description:      args.Description,
		Refs:             refs,
		Closes:           args.Closes,
		Supersedes:       args.Supersedes,
		Participants:     participants,
		Confidence:       args.Confidence,
		TopicLabels:      args.Topics,
		SkipPreflight:    args.SkipPreflight,
		PreflightTimeout: preflightTimeout,
		OnPreflight: func(result *query.PreflightResult) {
			for _, f := range result.Findings {
				pfFindings = append(pfFindings, Finding{
					Severity:    string(f.Severity),
					Category:    f.Category,
					Observation: f.Observation,
				})
			}
		},
		OnNewEntry: func(id, summary string) {
			createdID = id
			createdSummary = summary
		},
	}

	handlerErr := s.handler.NewEntry(ctx, cmd)
	if handlerErr != nil {
		// A blocked pre-flight is a structured outcome the agent acts on,
		// not a protocol error: findings were delivered via OnPreflight and
		// at least one is high.
		if hasHigh(pfFindings) {
			return nil, CaptureResult{
				Blocked:      true,
				Findings:     pfFindings,
				Instructions: captureBlockedInstructions,
			}, nil
		}
		return nil, CaptureResult{}, toolError("capture failed: %v", handlerErr)
	}

	relPath := ""
	if rel, err := model.IDToRelPath(createdID); err == nil {
		relPath = filepath.Join(s.graphDir, rel)
	}
	return nil, CaptureResult{
		Created:      true,
		ID:           createdID,
		Path:         relPath,
		Summary:      createdSummary,
		Findings:     pfFindings,
		Instructions: captureCreatedInstructions,
	}, nil
}

func (s *Server) showEntry(ctx context.Context, _ *mcp.CallToolRequest, args ShowEntryArgs) (*mcp.CallToolResult, ShowEntryResult, error) {
	if len(args.IDs) == 0 {
		return nil, ShowEntryResult{}, toolError("ids is required")
	}

	graph, err := s.finder.LoadGraph(s.graphDir)
	if err != nil {
		return nil, ShowEntryResult{}, toolError("loading graph: %v", err)
	}

	upDepth := args.UpDepth
	if upDepth == 0 {
		upDepth = query.DefaultUpDepth
	}
	downDepth := args.DownDepth
	if downDepth == 0 {
		downDepth = query.DefaultDownDepth
	}

	res, err := s.finder.Show(query.ShowQuery{
		Graph:     graph,
		IDs:       args.IDs,
		UpDepth:   upDepth,
		DownDepth: downDepth,
	})
	if err != nil {
		return nil, ShowEntryResult{}, toolError("showing entries: %v", err)
	}

	var sb strings.Builder
	presenters.RenderShow(&sb, res, presenters.ShowOptions{})
	return nil, ShowEntryResult{
		Entries:      sb.String(),
		Instructions: showEntryInstructions,
	}, nil
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

func hasHigh(findings []Finding) bool {
	for _, f := range findings {
		if f.Severity == string(query.SeverityHigh) {
			return true
		}
	}
	return false
}
