package tmux

import (
	"fmt"
	"os/exec"
	"strings"
)

type Pane struct {
	ID      string
	Session string
	Window  string
	Index   string
	Command string
}

func (p Pane) Location() string {
	return fmt.Sprintf("%s:%s.%s", p.Session, p.Window, p.Index)
}

const paneFormat = "#{pane_id}\t#{session_name}\t#{window_index}\t#{pane_index}\t#{pane_current_command}"

func ListPanes() ([]Pane, error) {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", paneFormat).Output()
	if err != nil {
		return nil, err
	}
	var panes []Pane
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 5 {
			continue
		}
		panes = append(panes, Pane{ID: f[0], Session: f[1], Window: f[2], Index: f[3], Command: f[4]})
	}
	return panes, nil
}

func CurrentCommand(pane string) (string, error) {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", pane, "#{pane_current_command}").Output()
	return strings.TrimSpace(string(out)), err
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

func Unbind(key string) error {
	return exec.Command("tmux", "unbind-key", key).Run()
}
