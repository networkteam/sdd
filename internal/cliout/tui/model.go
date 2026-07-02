package tui

import (
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/networkteam/sdd/internal/cliout"
)

// Messages driving the model. logMsg/progressMsg carry pipe data; the *DoneMsg
// variants signal their stream finished (Recv returned ok=false).
type (
	logMsg          cliout.LogEntry
	logDoneMsg      struct{}
	progressMsg     cliout.Progress
	progressDoneMsg struct{}
)

// model is the inline transient view: a single footer line (spinner, optional
// determinate bar, count) rendered in normal terminal flow — no alt-screen.
// When streamLogs is set, display-eligible log entries are emitted as durable
// lines above the footer via tea.Printf, so they scroll into terminal history
// like ordinary output; the footer redraws around them and is cleared on quit.
// Every entry is still fed to the recorder for the durable re-emit. The model
// quits when the log stream finishes.
type model struct {
	label string

	spinner  spinner.Model
	progress progress.Model
	hasProg  bool
	reporter *cliout.Reporter

	logs       *cliout.LogConsumer
	policy     cliout.Policy
	rec        *cliout.Recorder
	streamLogs bool

	lastProg cliout.Progress
	width    int

	interrupt func()
	done      bool
}

// defaultBarWidth is used until the first WindowSizeMsg arrives so the bar
// renders sensibly even before the terminal size is known.
const defaultBarWidth = 28

func newModel(view View, logs *cliout.LogConsumer, policy cliout.Policy, rec *cliout.Recorder, interrupt func()) model {
	m := model{
		label:      view.Label,
		spinner:    spinner.New(spinner.WithSpinner(spinner.Dot)),
		logs:       logs,
		policy:     policy,
		rec:        rec,
		streamLogs: view.StreamLogs,
		interrupt:  interrupt,
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
		if m.streamLogs && m.policy.ShowInDisplay(e.Level) {
			// Durable: print above the footer; it persists in scrollback.
			return m, tea.Batch(tea.Printf("%s", renderEntry(e)), recvLog(m.logs))
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
			var cmd tea.Cmd
			m.progress, cmd = m.progress.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	return m, nil
}

// View renders the inline footer only — one line: spinner, label, (when a
// reporter is wired) a determinate bar with an absolute count, and an optional
// note naming the work in flight. No alt-screen, so bubble tea clears just this
// footer on quit while durable log lines and the result remain.
func (m model) View() tea.View {
	var b strings.Builder
	b.WriteString(m.spinner.View())
	b.WriteByte(' ')
	b.WriteString(styleLabel.Render(m.label))
	if m.hasProg {
		b.WriteString("  ")
		b.WriteString(m.progress.View())
		if c := renderCount(m.lastProg); c != "" {
			b.WriteString("  ")
			b.WriteString(c)
		}
	}
	if n := m.lastProg.Note; n != "" {
		b.WriteString("  ")
		b.WriteString(styleBody.Render(n))
	}
	return tea.NewView(b.String())
}

// barWidth keeps the determinate bar from crowding the label and count on
// narrow terminals while capping it on wide ones.
func barWidth(termWidth int) int {
	w := termWidth - 36 // leave room for spinner + label + count
	return max(min(w, 40), 10)
}
