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

	"github.com/dvpervov/aox/internal/claude"
	"github.com/dvpervov/aox/internal/state"
	"github.com/dvpervov/aox/internal/tmux"
)

type row struct {
	pane    tmux.Pane
	session *state.Session
}

func (r row) state() state.State {
	if r.session == nil {
		return state.Unknown
	}
	return r.session.State
}

func (r row) title() string {
	if r.session == nil {
		return ""
	}
	return r.session.Title()
}

type (
	refreshMsg struct{}
	reloadMsg  int
	rowsMsg    []row
	errMsg     struct{ err error }
)

type model struct {
	rows      []row
	cursor    int
	err       error
	renaming  bool
	input     textinput.Model
	reloadGen int
}

const (
	reloadDelay = time.Second
	returnKey   = "a"
)

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
		if err := tmux.BindSwitch(returnKey, pane); err != nil {
			return err
		}
		defer tmux.Unbind(returnKey)
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
	byPane := map[string]*state.Session{}
	for i := range sessions {
		s := &sessions[i]
		if cur, ok := byPane[s.Pane]; !ok || s.Updated.After(cur.Updated) {
			byPane[s.Pane] = s
		}
	}
	var rows []row
	for _, p := range panes {
		if p.Command == claude.Command {
			rows = append(rows, row{pane: p, session: byPane[p.ID]})
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
	return func() tea.Msg {
		if err := tmux.SwitchTo(pane); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m model) Init() tea.Cmd {
	return load
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshMsg:
		m.reloadGen++
		gen := m.reloadGen
		return m, tea.Batch(load, tea.Tick(reloadDelay, func(time.Time) tea.Msg { return reloadMsg(gen) }))
	case reloadMsg:
		if int(msg) == m.reloadGen {
			return m, load
		}
	case rowsMsg:
		m.rows = msg
		m.cursor = max(0, min(m.cursor, len(m.rows)-1))
		m.err = nil
	case errMsg:
		m.err = msg.err
	case tea.KeyMsg:
		if m.renaming {
			return m.updateRename(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m model) updateRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.renaming = false
		return m, rename(m.rows[m.cursor].session.ID, strings.TrimSpace(m.input.Value()))
	case "esc":
		m.renaming = false
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
	case "r":
		if len(m.rows) > 0 && m.rows[m.cursor].session != nil {
			m.renaming = true
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
		b.WriteString("No Claude sessions\n")
	}
	for i, r := range m.rows {
		st := r.state()
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		mark := " "
		line := lipgloss.NewStyle()
		if st == state.Waiting {
			mark = "●"
			line = stateStyles[state.Waiting]
		}
		b.WriteString(cursor + " " + line.Render(mark+" ") +
			stateStyles[st].Render(fmt.Sprintf("%-8s", st)) +
			line.Render(fmt.Sprintf(" %-7s %-16s %s", "claude", r.pane.Location(), r.title())) + "\n")
	}
	if m.renaming {
		b.WriteString("\n" + m.input.View() + "\n")
	}
	if m.err != nil {
		b.WriteString("\n" + m.err.Error() + "\n")
	}
	b.WriteString("\n" + helpStyle.Render("enter: jump  r: rename  q: quit") + "\n")
	return b.String()
}
