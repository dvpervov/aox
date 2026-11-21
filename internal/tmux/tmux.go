package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type Pane struct {
	ID      string
	Session string
	Window  string
	Index   string
	Command string
	Title   string
}

func (p Pane) Location() string {
	return fmt.Sprintf("%s:%s.%s", p.Session, p.Window, p.Index)
}

const commandFormat = "#{pane_current_command}\t#{pane_tty}"
const paneFormat = "#{pane_id}\t#{session_name}\t#{window_index}\t#{pane_index}\t" + commandFormat + "\t#{pane_title}"

func ListPanes() ([]Pane, error) {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", paneFormat).Output()
	if err != nil {
		return nil, err
	}
	var panes []Pane
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		f := strings.SplitN(line, "\t", 7)
		if len(f) != 7 {
			continue
		}
		panes = append(panes, Pane{ID: f[0], Session: f[1], Window: f[2], Index: f[3], Command: paneCommand(f[4], f[5]), Title: f[6]})
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

func SwitchTo(pane string) error {
	return exec.Command("tmux", switchArgs(pane, ";")...).Run()
}

func BindSwitch(key, pane string) error {
	return exec.Command("tmux", append([]string{"bind-key", key}, switchArgs(pane, `\;`)...)...).Run()
}

func ExecSession(name, command string) error {
	path, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	return syscall.Exec(path, []string{"tmux", "new-session", "-A", "-s", name, command}, os.Environ())
}

func SwitchLast() error {
	return exec.Command("tmux", "switch-client", "-l").Run()
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

func Unbind(key string) error {
	return exec.Command("tmux", "unbind-key", key).Run()
}
