package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fsnotify/fsnotify"

	"github.com/dvpervov/aox/internal/codex"
	"github.com/dvpervov/aox/internal/state"
	"github.com/dvpervov/aox/internal/tmux"
)

type row struct {
	pane          tmux.Pane
	session       *state.Session
	terminalState state.State
	activityTitle string
}

func (r row) state() state.State {
	if r.session != nil && r.session.State != state.Unknown {
		return r.session.State
	}
	if r.pane.Command == state.Codex && r.terminalState != "" {
		return r.terminalState
	}
	return state.Unknown
}

func (r row) title() string {
	if r.session != nil {
		if title := r.session.Title(); title != "" {
			return title
		}
	}
	if r.pane.Command == state.Codex {
		return codex.PaneTitle(r.pane.Title)
	}
	return ""
}

type (
	refreshMsg struct{}
	pollMsg    struct{}
	rowsMsg    []row
	errMsg     struct{ err error }
)

type model struct {
	rows     []row
	cursor   int
	err      error
	renameID string
	input    textinput.Model
}

const pollInterval = time.Second

var (
	normalFg    = lipgloss.AdaptiveColor{Light: "235", Dark: "252"}
	selectedFg  = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7571F9"}
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F780E2"))
)

var returnKeys = []struct{ table, key string }{
	{"prefix", "a"},
	{"root", "M-a"},
}

var (
	stateStyles = map[state.State]lipgloss.Style{
		state.Working: lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		state.Waiting: lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true),
		state.Idle:    lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		state.Unknown: lipgloss.NewStyle().Faint(true),
	}
	helpStyle = lipgloss.NewStyle().Faint(true)
)

func Run() error {
	dir, err := state.Dir()
	if err != nil {
		return err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	if err := w.Add(dir); err != nil {
		return err
	}
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		for _, k := range returnKeys {
			if err := tmux.BindSwitch(k.table, k.key, pane); err != nil {
				return err
			}
			defer tmux.Unbind(k.table, k.key)
		}
	}
	p := tea.NewProgram(model{input: textinput.New()}, tea.WithAltScreen())
	go func() {
		for {
			select {
			case _, ok := <-w.Events:
				if !ok {
					return
				}
				p.Send(refreshMsg{})
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				p.Send(errMsg{err})
			}
		}
	}()
	_, err = p.Run()
	return err
}

func load() tea.Msg {
	panes, err := tmux.ListPanes()
	if err != nil {
		return errMsg{err}
	}
	sessions, err := state.List()
	if err != nil {
		return errMsg{err}
	}
	type paneAgent struct{ pane, agent string }
	byPane := map[paneAgent]*state.Session{}
	for i := range sessions {
		s := &sessions[i]
		key := paneAgent{s.Pane, s.AgentName()}
		if cur, ok := byPane[key]; !ok || s.Updated.After(cur.Updated) {
			byPane[key] = s
		}
	}
	var rows []row
	for _, p := range panes {
		if p.Command == state.Claude || p.Command == state.Codex {
			rows = append(rows, row{pane: p, session: byPane[paneAgent{p.ID, p.Command}]})
		}
	}
	return rowsMsg(rows)
}

func rename(id, name string) tea.Cmd {
	return func() tea.Msg {
		s, err := state.Load(id)
		if err != nil {
			return errMsg{err}
		}
		if s == nil {
			return nil
		}
		s.Name = name
		if err := state.Save(s); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func switchTo(pane string) tea.Cmd {
	return tmuxCmd(func() error { return tmux.SwitchTo(pane) })
}

func tmuxCmd(f func() error) tea.Cmd {
	return func() tea.Msg {
		if err := f(); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(load, poll())
}

func poll() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return pollMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshMsg:
		return m, load
	case pollMsg:
		return m, tea.Batch(load, poll())
	case rowsMsg:
		activityTitles := make(map[string]string, len(m.rows))
		for _, r := range m.rows {
			if r.activityTitle != "" {
				activityTitles[r.pane.ID] = r.activityTitle
			}
		}
		for i := range msg {
			r := &msg[i]
			if r.pane.Command == state.Codex {
				r.terminalState, r.activityTitle = codex.TitleStatus(r.pane.Title, activityTitles[r.pane.ID])
			}
		}
		m.rows = msg
		m.cursor = max(0, min(m.cursor, len(m.rows)-1))
		m.err = nil
	case errMsg:
		m.err = msg.err
	case tea.KeyMsg:
		if m.renameID != "" {
			return m.updateRename(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m model) updateRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		id := m.renameID
		m.renameID = ""
		return m, rename(id, strings.TrimSpace(m.input.Value()))
	case "esc":
		m.renameID = ""
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
	case "down", "j":
		m.cursor = max(0, min(m.cursor+1, len(m.rows)-1))
	case "enter":
		if len(m.rows) > 0 {
			return m, switchTo(m.rows[m.cursor].pane.ID)
		}
	case "esc":
		return m, tmuxCmd(tmux.SwitchLast)
	case "r":
		if len(m.rows) > 0 && m.rows[m.cursor].session != nil {
			m.renameID = m.rows[m.cursor].session.ID
			m.input.SetValue(m.rows[m.cursor].title())
			m.input.CursorEnd()
			return m, m.input.Focus()
		}
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	if len(m.rows) == 0 {
		b.WriteString("No agent sessions\n")
	}
	for i, r := range m.rows {
		st := r.state()
		selected := i == m.cursor
		cursor := " "
		if selected {
			cursor = cursorStyle.Render(">")
		}
		mark := " "
		line := lipgloss.NewStyle().Foreground(normalFg)
		switch {
		case st == state.Waiting:
			mark = "●"
			line = stateStyles[state.Waiting]
		case selected:
			line = line.Foreground(selectedFg)
		}
		b.WriteString(cursor + " " + line.Render(mark+" ") +
			stateStyles[st].Render(fmt.Sprintf("%-8s", stateLabel(st))) +
			line.Render(fmt.Sprintf(" %-7s %-16s %s", r.pane.Command, r.pane.Location(), r.title())) + "\n")
	}
	if m.renameID != "" {
		b.WriteString("\n" + m.input.View() + "\n")
	}
	if m.err != nil {
		b.WriteString("\n" + m.err.Error() + "\n")
	}
	b.WriteString("\n" + helpStyle.Render("enter: jump  esc: back  r: rename  q: quit") + "\n")
	return b.String()
}

func stateLabel(st state.State) string {
	if st == state.Unknown {
		return "?"
	}
	return string(st)
}
