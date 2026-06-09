package tui

import (
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/networkteam/sdd/internal/cliout"
)

const (
	// displayRingSize bounds the in-model history fed to the viewport — caps
	// redraw cost and memory regardless of how chatty the operation is.
	displayRingSize = 256
	// maxViewportLines caps the scrolling log tail; the actual height is the
	// lesser of this and the terminal height minus the header.
	maxViewportLines = 10
)

// Messages driving the model. logMsg/progressMsg carry pipe data; the *DoneMsg
// variants signal their stream finished (Recv returned ok=false).
type (
	logMsg          cliout.LogEntry
	logDoneMsg      struct{}
	progressMsg     cliout.Progress
	progressDoneMsg struct{}
)

// model is the transient view: a spinner, an optional determinate progress
// bar, and a scrolling tail of recent log entries. It drains the log pipe and
// progress reporter via re-issued commands, pushes display-eligible entries
// into a bounded ring, and feeds every entry to the recorder for the durable
// re-emit. It quits when the log stream finishes.
type model struct {
	label string

	spinner  spinner.Model
	progress progress.Model
	hasProg  bool
	reporter *cliout.Reporter
	viewport viewport.Model

	logs   *cliout.LogConsumer
	policy cliout.Policy
	rec    *cliout.Recorder

	ring     []cliout.LogEntry
	lastProg cliout.Progress
	width    int
	height   int
	ready    bool

	interrupt func()
	done      bool
}

func newModel(view View, logs *cliout.LogConsumer, policy cliout.Policy, rec *cliout.Recorder, interrupt func()) model {
	m := model{
		label:     view.Label,
		spinner:   spinner.New(spinner.WithSpinner(spinner.Dot)),
		viewport:  viewport.New(),
		logs:      logs,
		policy:    policy,
		rec:       rec,
		interrupt: interrupt,
	}
	if view.Progress != nil {
		m.hasProg = true
		m.reporter = view.Progress
		m.progress = progress.New()
	}
	return m
}

// recvLog and recvProgress adapt the bubble-tea-free consumers into commands.
// They are package-local so cliout itself stays free of any tea import.
func recvLog(c *cliout.LogConsumer) tea.Cmd {
	return func() tea.Msg {
		if e, ok := c.Recv(); ok {
			return logMsg(e)
		}
		return logDoneMsg{}
	}
}

func recvProgress(r *cliout.Reporter) tea.Cmd {
	return func() tea.Msg {
		if p, ok := r.Recv(); ok {
			return progressMsg(p)
		}
		return progressDoneMsg{}
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{recvLog(m.logs), m.spinner.Tick}
	if m.hasProg {
		cmds = append(cmds, recvProgress(m.reporter), m.progress.Init())
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		m.resize()
		return m, nil

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			if m.interrupt != nil {
				m.interrupt()
			}
			return m, tea.Quit
		}
		return m, nil

	case logMsg:
		e := cliout.LogEntry(msg)
		m.rec.Observe(e)
		if m.policy.ShowInDisplay(e.Level) {
			m.pushRing(e)
			m.refreshViewport()
		}
		return m, recvLog(m.logs)

	case logDoneMsg:
		m.done = true
		return m, tea.Quit

	case progressMsg:
		m.lastProg = cliout.Progress(msg)
		var cmd tea.Cmd
		if m.hasProg {
			cmd = m.progress.SetPercent(m.lastProg.Ratio())
		}
		return m, tea.Batch(cmd, recvProgress(m.reporter))

	case progressDoneMsg:
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		if m.hasProg {
			m.progress, _ = updateProgress(m.progress, msg)
		}
		return m, nil
	}
	return m, nil
}

// updateProgress isolates the progress.Model update so the FrameMsg branch
// stays a single statement and the concrete-type handoff is explicit.
func updateProgress(p progress.Model, msg tea.Msg) (progress.Model, tea.Cmd) {
	return p.Update(msg)
}

func (m model) View() tea.View {
	var b strings.Builder
	b.WriteString(m.spinner.View())
	b.WriteByte(' ')
	b.WriteString(styleLabel.Render(m.label))
	if m.hasProg {
		if c := renderCount(m.lastProg); c != "" {
			b.WriteString("  ")
			b.WriteString(c)
		}
		b.WriteByte('\n')
		b.WriteString(m.progress.View())
	}
	b.WriteByte('\n')
	if m.ready {
		b.WriteString(m.viewport.View())
	}

	v := tea.NewView(b.String())
	v.AltScreen = true // transient: alt-screen drops on quit, restoring scrollback
	return v
}

// resize sizes the viewport to the terminal, leaving room for the header
// (spinner line plus the progress line when present) and a trailing blank.
func (m *model) resize() {
	header := 1
	if m.hasProg {
		header++
	}
	vh := max(min(m.height-header-1, maxViewportLines), 1)
	m.viewport.SetWidth(m.width)
	m.viewport.SetHeight(vh)
	if m.hasProg {
		m.progress.SetWidth(m.width)
	}
	m.refreshViewport()
}

func (m *model) pushRing(e cliout.LogEntry) {
	m.ring = append(m.ring, e)
	if len(m.ring) > displayRingSize {
		m.ring = m.ring[len(m.ring)-displayRingSize:]
	}
}

func (m *model) refreshViewport() {
	lines := make([]string, len(m.ring))
	for i, e := range m.ring {
		lines[i] = renderEntry(e)
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
	m.viewport.GotoBottom()
}
