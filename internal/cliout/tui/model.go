package tui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/networkteam/sdd/internal/cliout"
)

// Messages driving the model. logMsg/progressMsg carry pipe data; the *DoneMsg
// variants signal their stream finished (Recv returned ok=false). firstPaintMsg
// fires a few frames after the first WindowSizeMsg — the gate that lets held
// lines flush only once the renderer has certainly flushed a frame.
type (
	logMsg          cliout.LogEntry
	logDoneMsg      struct{}
	progressMsg     cliout.Progress
	progressDoneMsg struct{}
	firstPaintMsg   struct{}
)

// model is the inline transient view: a single footer line (spinner, optional
// determinate bar, count) rendered in normal terminal flow — no alt-screen.
// Display-eligible log lines arrive already filtered by the coordinator and are
// emitted as durable lines above the footer via tea.Printf, scrolling into
// terminal history; the footer redraws around them and is cleared on quit.
//
// Durable lines are held until a first-paint gate (first WindowSizeMsg plus a
// short tick): a WindowSizeMsg does not imply a renderer flush, and printing
// before the first flush emits a full-height cursor-down against a cellbuf still
// sized to the whole terminal. Holding until the gate keeps that escape out of
// the output. The model quits when the log stream finishes.
type model struct {
	label string

	spinner  spinner.Model
	progress progress.Model
	hasProg  bool
	reporter *cliout.Reporter

	logs *cliout.LogConsumer

	lastProg cliout.Progress
	width    int

	interrupt func()
	done      bool

	held      []cliout.LogEntry // durable lines awaiting the first-paint gate
	painted   bool              // gate open: lines pass straight through
	gateArmed bool              // first-paint tick scheduled
}

// defaultBarWidth is used until the first WindowSizeMsg arrives so the bar
// renders sensibly even before the terminal size is known.
const defaultBarWidth = 28

// firstPaintDelay is ~3 frame intervals at the default 60fps — long enough that
// the renderer has flushed its first frame (and resized its cellbuf to the
// footer height) before any held line is printed above it.
const firstPaintDelay = 50 * time.Millisecond

func newModel(view View, logs *cliout.LogConsumer, backlog []cliout.LogEntry, interrupt func()) model {
	m := model{
		label:     view.Label,
		spinner:   spinner.New(spinner.WithSpinner(spinner.Dot)),
		logs:      logs,
		interrupt: interrupt,
		held:      backlog,
	}
	if view.Progress != nil {
		m.hasProg = true
		m.reporter = view.Progress
		m.progress = progress.New(progress.WithWidth(defaultBarWidth))
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
		m.width = msg.Width
		if m.hasProg {
			m.progress.SetWidth(barWidth(m.width))
		}
		if !m.gateArmed {
			m.gateArmed = true
			return m, tea.Tick(firstPaintDelay, func(time.Time) tea.Msg { return firstPaintMsg{} })
		}
		return m, nil

	case firstPaintMsg:
		m.painted = true
		cmd := m.flushHeld()
		return m, cmd

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
		if !m.painted {
			m.held = append(m.held, e)
			return m, recvLog(m.logs)
		}
		return m, tea.Batch(tea.Printf("%s", cliout.RenderEntry(e)), recvLog(m.logs))

	case logDoneMsg:
		m.done = true
		// Flush anything still held so a fast finish loses nothing.
		return m, tea.Sequence(m.flushHeld(), tea.Quit)

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
			var cmd tea.Cmd
			m.progress, cmd = m.progress.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	return m, nil
}

// flushHeld prints the held durable lines in arrival order and clears the
// buffer. Returns nil when nothing is held. Modifies the receiver in place, so
// call it on the returned model value.
func (m *model) flushHeld() tea.Cmd {
	if len(m.held) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, len(m.held))
	for i, e := range m.held {
		cmds[i] = tea.Printf("%s", cliout.RenderEntry(e))
	}
	m.held = nil
	return tea.Sequence(cmds...)
}

// View renders the inline footer only — one line: spinner, label, (when a
// reporter is wired) a determinate bar with an absolute count, and an optional
// note naming the work in flight. No alt-screen, so bubble tea clears just this
// footer on quit while durable log lines and the result remain.
func (m model) View() tea.View {
	var b strings.Builder
	b.WriteString(m.spinner.View())
	b.WriteByte(' ')
	b.WriteString(cliout.StyleLabel.Render(m.label))
	if m.hasProg {
		b.WriteString("  ")
		b.WriteString(m.progress.View())
		if c := cliout.RenderCount(m.lastProg); c != "" {
			b.WriteString("  ")
			b.WriteString(c)
		}
	}
	if n := m.lastProg.Note; n != "" {
		b.WriteString("  ")
		b.WriteString(cliout.StyleBody.Render(n))
	}
	return tea.NewView(b.String())
}

// barWidth keeps the determinate bar from crowding the label and count on
// narrow terminals while capping it on wide ones.
func barWidth(termWidth int) int {
	w := termWidth - 36 // leave room for spinner + label + count
	return max(min(w, 40), 10)
}
