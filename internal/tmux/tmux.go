package tmux

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Pane struct {
	ID      string
	Session string
	Window  string
	Index   string
	Command string
	Name    string
	Title   string
}

func (p Pane) Location() string {
	return fmt.Sprintf("%s:%s.%s", p.Session, p.Window, p.Index)
}

const commandFormat = "#{pane_current_command}\t#{pane_tty}"
const paneFormat = "#{pane_id}\t#{session_name}\t#{window_index}\t#{pane_index}\t" + commandFormat + "\t#{@aox_name}\t#{pane_title}"

func ListPanes() ([]Pane, error) {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", paneFormat).Output()
	if err != nil {
		return nil, err
	}
	var panes []Pane
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		f := strings.SplitN(line, "\t", 8)
		if len(f) != 8 {
			continue
		}
		panes = append(panes, Pane{ID: f[0], Session: f[1], Window: f[2], Index: f[3], Command: paneCommand(f[4], f[5]), Name: f[6], Title: f[7]})
	}
	return panes, nil
}

func CurrentCommand(pane string) (string, error) {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", pane, commandFormat).Output()
	if err != nil {
		return "", err
	}
	command, tty, _ := strings.Cut(strings.TrimRight(string(out), "\n"), "\t")
	return paneCommand(command, tty), nil
}

func paneCommand(command, tty string) string {
	if command != "node" || tty == "" {
		return command
	}
	// The npm launcher runs the Codex binary in its foreground process group.
	out, err := exec.Command("ps", "-ww", "-t", strings.TrimPrefix(tty, "/dev/"), "-o", "stat=", "-o", "comm=").Output()
	if err != nil {
		return command
	}
	for _, line := range strings.Split(string(out), "\n") {
		status, name, _ := strings.Cut(strings.TrimSpace(line), " ")
		if strings.Contains(status, "+") && filepath.Base(strings.TrimSpace(name)) == "codex" {
			return "codex"
		}
	}
	return command
}

// InPane reports whether the caller was launched from the given pane.
func InPane(pane string) bool {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", pane, "#{pane_pid}").Output()
	if err != nil {
		return false
	}
	panePID, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || panePID <= 0 {
		return false
	}
	out, err = exec.Command("ps", "-A", "-o", "pid=", "-o", "ppid=").Output()
	if err != nil {
		return false
	}
	parents := map[int]int{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		parent, _ := strconv.Atoi(fields[1])
		parents[pid] = parent
	}
	for pid := os.Getppid(); pid > 1; pid = parents[pid] {
		if pid == panePID {
			return true
		}
	}
	return false
}

func switchArgs(pane, sep string) []string {
	return []string{
		"switch-client", "-t", pane, sep,
		"select-window", "-t", pane, sep,
		"select-pane", "-t", pane,
	}
}

func run(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		err = errors.New("tmux: " + strings.TrimSpace(string(exitErr.Stderr)))
	}
	return strings.TrimSpace(string(out)), err
}

func SwitchTo(pane string) error {
	_, err := run(switchArgs(pane, ";")...)
	return err
}

func BindSwitch(table, key, pane string) (restore func(), err error) {
	prev := binding(table, key)
	if isSwitchBinding(prev) {
		prev = ""
	}
	if err := exec.Command("tmux", append([]string{"bind-key", "-T", table, key}, switchArgs(pane, `\;`)...)...).Run(); err != nil {
		return nil, err
	}
	ours := binding(table, key)
	return func() {
		if binding(table, key) != ours {
			return
		}
		if prev == "" {
			exec.Command("tmux", "unbind-key", "-T", table, key).Run()
			return
		}
		cmd := exec.Command("tmux", "source-file", "-")
		cmd.Stdin = strings.NewReader(prev)
		cmd.Run()
	}, nil
}

func NewWindow(session, dir, command string) (string, error) {
	args := []string{"new-window", "-t", "=" + session + ":"}
	if exec.Command("tmux", "has-session", "-t", "="+session).Run() != nil {
		args = []string{"new-session", "-d", "-s", session}
	}
	args = append(args, "-c", dir, "-P", "-F", "#{pane_id}", command)
	pane, err := run(args...)
	if err != nil {
		return "", err
	}
	if _, err := run("set-option", "-t", "="+session+":", "detach-on-destroy", "off"); err != nil {
		return "", err
	}
	return pane, nil
}

func Prefix() string {
	out, err := exec.Command("tmux", "show-options", "-gv", "prefix").Output()
	if err != nil {
		return ""
	}
	return strings.NewReplacer("C-", "ctrl+", "M-", "alt+", "S-", "shift+").Replace(strings.TrimSpace(string(out)))
}

func isSwitchBinding(b string) bool {
	return strings.Contains(b, `switch-client -t "%`) && strings.Contains(b, "select-window -t") && strings.Contains(b, "select-pane -t")
}

func binding(table, key string) string {
	out, err := exec.Command("tmux", "list-keys", "-T", table, key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func ExecSession(name, command string) error {
	path, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	return syscall.Exec(path, []string{"tmux", "new-session", "-A", "-s", name, command}, os.Environ())
}

func Capture(pane string) (string, error) {
	out, err := exec.Command("tmux", "capture-pane", "-p", "-e", "-t", pane).Output()
	return string(out), err
}

func SetName(pane, name string) error {
	args := []string{"set-option", "-p", "-t", pane, "@aox_name", name}
	if name == "" {
		args = []string{"set-option", "-p", "-u", "-t", pane, "@aox_name"}
	}
	_, err := run(args...)
	return err
}

func SwitchLast() error {
	last, err := run("display-message", "-p", "#{client_last_session}")
	if err != nil || last == "" || exec.Command("tmux", "has-session", "-t", "="+last).Run() != nil {
		return nil
	}
	_, err = run("switch-client", "-l")
	return err
}

func SessionOf(pane string) (string, error) {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", pane, "#{session_name}").Output()
	return strings.TrimSpace(string(out)), err
}

func SwitchToSession(name, command string) error {
	if exec.Command("tmux", "has-session", "-t", "="+name).Run() != nil {
		if err := exec.Command("tmux", "new-session", "-d", "-s", name, command).Run(); err != nil {
			return err
		}
	}
	return exec.Command("tmux", "switch-client", "-t", "="+name).Run()
}

const attentionOption = "@aox_attention"

func ShowAttention(extraLength int) (restore func(), err error) {
	right, err := run("show-options", "-gv", "status-right")
	if err != nil {
		return nil, err
	}
	length, err := run("show-options", "-gv", "status-right-length")
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(length)
	if err != nil {
		return nil, err
	}
	suffix := "#{" + attentionOption + "}"
	if strings.HasSuffix(right, suffix) {
		right = strings.TrimSuffix(right, suffix)
		n -= extraLength
		length = strconv.Itoa(n)
	}
	ours := right + suffix
	oursLength := strconv.Itoa(n + extraLength)
	if _, err := run("set-option", "-g", "status-right", ours); err != nil {
		return nil, err
	}
	if _, err := run("set-option", "-g", "status-right-length", oursLength); err != nil {
		return nil, err
	}
	return func() {
		run("set-option", "-gu", attentionOption)
		if cur, _ := run("show-options", "-gv", "status-right"); cur == ours {
			run("set-option", "-g", "status-right", right)
		}
		if cur, _ := run("show-options", "-gv", "status-right-length"); cur == oursLength {
			run("set-option", "-g", "status-right-length", length)
		}
	}, nil
}

func SetAttention(text string) error {
	if _, err := run("set-option", "-g", attentionOption, text); err != nil {
		return err
	}
	clients, err := run("list-clients", "-F", "#{client_tty}")
	if err != nil {
		return err
	}
	for _, tty := range strings.Fields(clients) {
		run("refresh-client", "-S", "-t", tty)
	}
	return nil
}

func ServerStart() (time.Time, error) {
	out, err := run("display-message", "-p", "#{start_time}")
	if err != nil {
		return time.Time{}, err
	}
	sec, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(sec, 0), nil
}
