package tui

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/fsnotify/fsnotify"

	"github.com/dvpervov/aox/internal/codex"
	"github.com/dvpervov/aox/internal/state"
	"github.com/dvpervov/aox/internal/tmux"
)

type row struct {
	order         int
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
	if r.pane.Name != "" {
		return r.pane.Name
	}
	if r.session != nil && r.session.Prompt != "" {
		return r.session.Prompt
	}
	if r.pane.Command == state.Codex {
		return codex.PaneTitle(r.pane.Title)
	}
	return ""
}

type (
	refreshMsg     struct{}
	pollMsg        struct{}
	previewTickMsg struct{}
	rowsMsg        []row
	errMsg         struct{ err error }
	launchedMsg    struct{ pane string }
	previewMsg     struct {
		pane  string
		lines []string
		loop  bool
	}
)

type model struct {
	rows         []row
	cursor       int
	err          error
	renamePane   string
	returnHint   string
	renameGhost  bool
	labeling     bool
	sorting      bool
	choosing     bool
	showHelp     bool
	launchAgent  string
	launchDir    string
	launchNaming bool
	pendingPane  string
	sortKey      string
	sortDesc     bool
	input        textinput.Model
	width        int
	height       int
	previewPane  string
	preview      []string
}

const (
	pollInterval    = time.Second
	previewInterval = 200 * time.Millisecond
	listPercent     = 40
	minPreviewWidth = 100
)

var (
	normalFg    = lipgloss.AdaptiveColor{Light: "235", Dark: "252"}
	selectedFg  = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7571F9"}
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F780E2"))
)

var returnKeys = []struct{ table, key string }{
	{"prefix", "a"},
}

var (
	stateStyles = map[state.State]lipgloss.Style{
		state.Working: lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		state.Waiting: lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true),
		state.Idle:    lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		state.Unknown: lipgloss.NewStyle().Faint(true),
	}
	helpStyle    = lipgloss.NewStyle().Faint(true)
	ghostStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "250", Dark: "238"})
	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F780E2")).Bold(true)
	dividerStyle = lipgloss.NewStyle().Faint(true)
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
			restore, err := tmux.BindSwitch(k.table, k.key, pane)
			if err != nil {
				return err
			}
			defer restore()
		}
	}
	var returnHint string
	if prefix := tmux.Prefix(); prefix != "" && os.Getenv("TMUX_PANE") != "" {
		returnHint = prefix + " a: open aox from any pane\n"
	}
	input := textinput.New()
	input.PlaceholderStyle = ghostStyle
	p := tea.NewProgram(model{input: input, returnHint: returnHint}, tea.WithAltScreen())
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		<-hup
		p.Quit()
	}()
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
			rows = append(rows, row{order: len(rows), pane: p, session: byPane[paneAgent{p.ID, p.Command}]})
		}
	}
	return rowsMsg(rows)
}

func rename(pane, name string) tea.Cmd {
	return func() tea.Msg {
		if err := tmux.SetName(pane, strings.ReplaceAll(name, "\t", " ")); err != nil {
			return errMsg{err}
		}
		return load()
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

func capture(pane string, loop bool) tea.Cmd {
	return func() tea.Msg {
		msg := previewMsg{pane: pane, loop: loop}
		if pane == "" {
			return msg
		}
		out, err := tmux.Capture(pane)
		if err != nil {
			return msg
		}
		lines := strings.Split(out, "\n")
		for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
			lines = lines[:len(lines)-1]
		}
		msg.lines = lines
		return msg
	}
}

func (m model) selectedPane() string {
	if len(m.rows) == 0 {
		return ""
	}
	return m.rows[m.cursor].pane.ID
}

func (m model) showPreview() bool {
	return m.width >= minPreviewWidth
}

func (m model) Init() tea.Cmd {
	return tea.Batch(load, poll(), capture("", true))
}

func poll() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return pollMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	prev := m.selectedPane()
	next, cmd := m.update(msg)
	if pane := next.(model).selectedPane(); pane != prev {
		return next, tea.Batch(cmd, capture(pane, false))
	}
	return next, cmd
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case previewTickMsg:
		if !m.showPreview() {
			return m, capture("", true)
		}
		return m, capture(m.selectedPane(), true)
	case previewMsg:
		if msg.pane == m.selectedPane() {
			m.previewPane, m.preview = msg.pane, msg.lines
		}
		if msg.loop {
			return m, tea.Tick(previewInterval, func(time.Time) tea.Msg { return previewTickMsg{} })
		}
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
		selected := m.selectedPane()
		m.rows = msg
		m.sortRows()
		m.cursor = max(0, min(m.cursor, len(m.rows)-1))
		m.selectPane(selected)
		for _, r := range m.rows {
			if r.pane.ID == m.pendingPane {
				m.selectPane(m.pendingPane)
				m.pendingPane = ""
			}
		}
		m.err = nil
	case launchedMsg:
		m.pendingPane = msg.pane
		return m, load
	case errMsg:
		m.err = msg.err
	case tea.KeyMsg:
		if m.renamePane != "" {
			return m.updateRename(msg)
		}
		if m.launchAgent != "" {
			return m.updateLaunch(msg)
		}
		return m.updateList(msg)
	}
	return m, nil
}

func (m model) updateRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		pane := m.renamePane
		m.renamePane = ""
		return m, rename(pane, strings.TrimSpace(m.input.Value()))
	case "esc":
		m.renamePane = ""
		return m, nil
	case "tab":
		if m.input.Value() == "" {
			m.renameGhost = false
			m.input.SetValue(m.input.Placeholder)
			m.input.CursorEnd()
		}
		return m, nil
	}
	if m.renameGhost {
		m.renameGhost = false
		switch msg.String() {
		case "left", "right", "up", "down", "home", "end":
			m.input.SetValue(m.input.Placeholder)
			m.input.CursorEnd()
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateLaunch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if !m.launchNaming {
			m.launchNaming = true
			m.launchDir = strings.TrimSpace(m.input.Value())
			m.input.SetSuggestions(nil)
			m.input.ShowSuggestions = false
			m.input.SetValue("")
			return m, nil
		}
		agent, dir, name := m.launchAgent, m.launchDir, strings.TrimSpace(m.input.Value())
		m.launchAgent, m.launchNaming = "", false
		return m, launch(agent, dir, name)
	case "esc":
		m.launchAgent, m.launchNaming = "", false
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if !m.launchNaming {
		m.input.SetSuggestions(dirSuggestions(m.input.Value()))
	}
	return m, cmd
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.pendingPane = ""
	if m.choosing {
		m.choosing = false
		agent, ok := launchKeys[msg.String()]
		if !ok {
			return m, nil
		}
		m.launchAgent = agent
		m.input.Placeholder = ""
		m.input.SetValue(state.LastDir())
		m.input.CursorEnd()
		m.input.ShowSuggestions = true
		m.input.SetSuggestions(dirSuggestions(m.input.Value()))
		return m, m.input.Focus()
	}
	if m.labeling {
		m.labeling = false
		if i := strings.Index(rowLabels, msg.String()); len(msg.String()) == 1 && i >= 0 && i < len(m.rows) {
			m.cursor = i
		}
		return m, nil
	}
	if m.sorting {
		m.sorting = false
		key := msg.String()
		if !strings.Contains("sapt", key) || len(key) != 1 {
			return m, nil
		}
		m.sortDesc = key == m.sortKey && !m.sortDesc
		m.sortKey = key
		selected := m.selectedPane()
		m.sortRows()
		m.selectPane(selected)
		return m, nil
	}
	switch msg.String() {
	case "s":
		m.sorting = true
	case "n":
		m.choosing = true
	case "h":
		m.showHelp = !m.showHelp
	case "g":
		m.labeling = len(m.rows) > 0
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
		if len(m.rows) > 0 {
			m.renamePane = m.rows[m.cursor].pane.ID
			m.input.Placeholder = m.rows[m.cursor].title()
			m.input.SetValue("")
			m.input.SetSuggestions(nil)
			m.input.ShowSuggestions = false
			m.renameGhost = m.input.Placeholder != ""
			return m, m.input.Focus()
		}
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	listWidth := m.width
	if m.showPreview() {
		listWidth = m.width * listPercent / 100
	}
	b.WriteString("      " + pad(m.columnTitle("s", "Status"), statusWidth) + columns(m.columnTitle("a", "Agent"), m.columnTitle("p", "Pane"), m.columnTitle("t", "Title")) + "\n")
	b.WriteString(dividerStyle.Render(strings.Repeat("─", listWidth)) + "\n")
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
		b.WriteString(labelStyle.Render(m.rowLabel(i)) + " " + cursor + " " + line.Render(mark+" ") +
			stateStyles[st].Render(fmt.Sprintf("%-*s", statusWidth, stateLabel(st))) +
			line.Render(columns(r.pane.Command, r.pane.Location(), r.title())) + "\n")
	}
	if m.renamePane != "" {
		b.WriteString("\n" + m.input.View() + "\n")
	}
	switch {
	case m.launchNaming:
		b.WriteString("\n" + helpStyle.Render("title (optional):") + "\n" + m.input.View() + "\n")
	case m.launchAgent != "":
		b.WriteString("\n" + helpStyle.Render("new "+m.launchAgent+" in:") + "\n" + m.input.View() + "\n")
	}
	if m.err != nil {
		b.WriteString("\n" + m.err.Error() + "\n")
	}
	help := m.helpLine()
	if hint := m.inputHint(); hint != "" {
		help = hint + "\n" + help
	}
	footer := strings.Split(helpStyle.Render(help), "\n")
	left := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	if m.height == 0 {
		return strings.Join(append(append(left, ""), footer...), "\n")
	}
	bodyHeight := max(0, m.height-len(footer))
	left = left[:min(len(left), bodyHeight)]
	for len(left) < bodyHeight {
		left = append(left, "")
	}
	left = append(left, footer...)
	if !m.showPreview() {
		return strings.Join(left, "\n")
	}
	previewWidth := m.width - listWidth - 1
	var preview []string
	if len(m.rows) > 0 {
		r := m.rows[m.cursor]
		preview = []string{
			helpStyle.Render("preview · " + r.pane.Command + " · " + r.pane.Location()),
			dividerStyle.Render(strings.Repeat("─", previewWidth)),
		}
		if body := m.height - len(preview); m.previewPane == r.pane.ID && body > 0 {
			preview = append(preview, m.preview[max(0, len(m.preview)-body):]...)
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		block(left, listWidth, m.height),
		dividerStyle.Render(strings.TrimSuffix(strings.Repeat("│\n", m.height), "\n")),
		block(preview, previewWidth, m.height),
	)
}

func block(lines []string, width, height int) string {
	lines = lines[:min(len(lines), height)]
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, width, "") + "\x1b[0m"
	}
	return lipgloss.NewStyle().Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

func stateLabel(st state.State) string {
	if st == state.Unknown {
		return "?"
	}
	return string(st)
}

const (
	columnGap   = "   "
	statusWidth = 8
	agentWidth  = 7
	paneWidth   = 16
)

func columns(agent, pane, title string) string {
	return columnGap + pad(agent, agentWidth) + columnGap + pad(pane, paneWidth) + columnGap + title
}

func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-ansi.StringWidth(s)))
}

const rowLabels = "123456789abcdefghijklmnopqrstuvwxyz"

func (m model) rowLabel(i int) string {
	if m.labeling && i < len(rowLabels) {
		return rowLabels[i : i+1]
	}
	return " "
}

var statusRank = map[state.State]int{state.Waiting: 0, state.Working: 1, state.Idle: 2, state.Unknown: 3}

func (r row) sortValue(key string) string {
	switch key {
	case "s":
		return fmt.Sprint(statusRank[r.state()])
	case "a":
		return r.pane.Command
	case "p":
		return r.pane.Location()
	case "t":
		return strings.ToLower(r.title())
	}
	return ""
}

func (m *model) sortRows() {
	sort.SliceStable(m.rows, func(i, j int) bool {
		a, b := m.rows[i].sortValue(m.sortKey), m.rows[j].sortValue(m.sortKey)
		if a != b {
			return (a < b) != m.sortDesc
		}
		return m.rows[i].order < m.rows[j].order
	})
}

func (m *model) selectPane(pane string) {
	for i, r := range m.rows {
		if r.pane.ID == pane {
			m.cursor = i
			return
		}
	}
}

func (m model) columnTitle(key, name string) string {
	switch {
	case key != m.sortKey:
	case m.sortDesc:
		name += " ↑"
	default:
		name += " ↓"
	}
	if m.sorting {
		return labelStyle.Render(name[:1]) + helpStyle.Render(name[1:])
	}
	return helpStyle.Render(name)
}

func (m model) inputHint() string {
	switch {
	case m.renamePane != "":
		return "enter: save   esc: cancel"
	case m.launchNaming:
		return "enter: start   esc: cancel"
	case m.launchAgent != "":
		return "enter: next   esc: cancel"
	}
	return ""
}

func (m model) helpLine() string {
	if m.choosing {
		return "new:   l: claude   o: codex"
	}
	if m.sorting {
		return "sort by:   s: status   a: agent   p: pane   t: title"
	}
	if !m.showHelp {
		return "h: help"
	}
	return m.returnHint + "enter: jump   g: go to line   s: sort   esc: last session\nn: start new agent   r: rename   q: quit\nh: hide help"
}

const agentsSession = "aox-agents"

var launchKeys = map[string]string{"l": state.Claude, "o": state.Codex}

func launch(agent, dir, name string) tea.Cmd {
	return func() tea.Msg {
		path, err := expandHome(dir)
		if err != nil {
			return errMsg{err}
		}
		if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
			return errMsg{fmt.Errorf("not a directory: %s", dir)}
		}
		command := agent
		if self := os.Getenv("TMUX_PANE"); self != "" {
			// Job control gives the agent its own process group, so tmux reports it as the pane command.
			command = "set -m; " + agent + `; [ "$(tmux display-message -p '#{session_attached}')" -gt 0 ] && tmux switch-client -t ` + self +
				` \; select-window -t ` + self + ` \; select-pane -t ` + self
		}
		pane, err := tmux.NewWindow(agentsSession, path, command)
		if err != nil {
			return errMsg{err}
		}
		if name != "" {
			if err := tmux.SetName(pane, strings.ReplaceAll(name, "\t", " ")); err != nil {
				return errMsg{err}
			}
		}
		if err := state.SaveLastDir(dir); err != nil {
			return errMsg{err}
		}
		if err := tmux.SwitchTo(pane); err != nil {
			return errMsg{err}
		}
		return launchedMsg{pane}
	}
}

func expandHome(dir string) (string, error) {
	if dir != "~" && !strings.HasPrefix(dir, "~/") {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, dir[1:]), nil
}

func dirSuggestions(value string) []string {
	if value == "~" {
		return []string{"~/"}
	}
	dir, base := value[:strings.LastIndex(value, "/")+1], value[strings.LastIndex(value, "/")+1:]
	path, err := expandHome(dir)
	if err != nil {
		return nil
	}
	if path == "" {
		path = "."
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	var suggestions []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, base) || strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		if fi, err := os.Stat(filepath.Join(path, name)); err != nil || !fi.IsDir() {
			continue
		}
		suggestions = append(suggestions, dir+name+"/")
	}
	return suggestions
}
