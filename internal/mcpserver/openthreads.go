package mcpserver

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/networkteam/sdd/internal/engine"
)

// Open-threads blocks ride base (session shell) serves only — the junction
// the dialogue stands on before, between, and after moves. Mid-procedure
// serves never carry one, so a user is not reminded of other work after
// every interview answer; that property holds by construction because the
// only attach point is the shell instance's serve.

// openThreadsIntro is the base instruction served the first time a block
// appears on a connection; later blocks carry the one-line reminder, the
// same served-once memory as instruction units (connection-keyed, hashed).
const openThreadsIntro = `Open work, in continuation order — this dialogue's other threads first, then
other open dialogues (resume_session picks one up). Present it to the user in
their language as options to continue, never as an obligation:`

const openThreadsReminder = "(open work, in continuation order — offer continuations as before)"

// openThreadsBlock renders the open work visible from a junction: the bound
// session's other running instances first, then every other open dialogue
// from the session store. Empty when there is nothing open — junctions with
// no parked work stay quiet.
func (s *Server) openThreadsBlock(ms *mcp.ServerSession, ss *shellSession, includeOwnThreads bool) string {
	var lines []string

	if includeOwnThreads && ss.sess != nil {
		for _, inst := range ss.sess.Instances() {
			if inst.Status != engine.StatusRunning || inst.ID == ss.shellInstance {
				continue
			}
			lines = append(lines, fmt.Sprintf("- (this dialogue) %s: %s at %s", inst.ID, inst.Spec.Canonical, inst.Step))
		}
	}

	if descs, err := s.sessions.listOpenSessions(); err == nil {
		for _, d := range descs {
			if d.Session == ss.id {
				continue
			}
			var b strings.Builder
			fmt.Fprintf(&b, "- %s", d.Session)
			if d.Label != "" {
				fmt.Fprintf(&b, " %q", d.Label)
			}
			if d.Participant != "" {
				fmt.Fprintf(&b, " (%s)", d.Participant)
			}
			var open []string
			for _, inst := range d.Open {
				open = append(open, inst.Procedure+" at "+inst.Step)
			}
			if len(open) > 0 {
				fmt.Fprintf(&b, " — open: %s", strings.Join(open, ", "))
			}
			if d.LastActivity != "" {
				fmt.Fprintf(&b, ", last active %s", d.LastActivity)
			}
			lines = append(lines, b.String())
		}
	}

	if len(lines) == 0 {
		return ""
	}
	header := openThreadsReminder
	if !s.servedBefore(ms, openThreadsIntro) {
		header = openThreadsIntro
	}
	return header + "\n" + strings.Join(lines, "\n")
}
