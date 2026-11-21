package codex

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"

	"github.com/dvpervov/aox/internal/hooks"
	"github.com/dvpervov/aox/internal/state"
)

// TitleStatus reads the activity markers written by the Codex terminal UI.
// An unmarked title confirms idle only if it matches a previously marked title.
func TitleStatus(title, activityTitle string) (state.State, string) {
	status, label := splitTitle(title)
	if status != state.Unknown {
		return status, label
	}
	if activityTitle != "" && label == activityTitle {
		return state.Idle, activityTitle
	}
	return state.Unknown, ""
}

func PaneTitle(title string) string {
	_, label := splitTitle(title)
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, label))
}

func splitTitle(title string) (state.State, string) {
	title = strings.TrimPrefix(strings.TrimSpace(title), "● ")
	for _, marker := range []string{"[ ! ] Action Required", "[ . ] Action Required"} {
		if suffix, ok := strings.CutPrefix(title, marker); ok && (suffix == "" || strings.HasPrefix(suffix, " ")) {
			suffix = strings.TrimSpace(suffix)
			for _, separator := range []string{"· ", "— ", "- ", "| "} {
				if label, ok := strings.CutPrefix(suffix, separator); ok {
					return state.Waiting, label
				}
			}
			return state.Waiting, suffix
		}
	}
	frame, suffix, _ := strings.Cut(title, " ")
	switch frame {
	case "⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏":
		return state.Working, strings.TrimSpace(suffix)
	}
	return state.Unknown, title
}

func HandleEvent(r io.Reader, w io.Writer) error {
	if err := hooks.HandleEvent(r, state.Codex); err != nil {
		return err
	}
	// Stop expects JSON on stdout. An empty object leaves Codex's behavior alone.
	_, err := fmt.Fprintln(w, "{}")
	return err
}

func InstallHooks() error {
	path, err := hooks.ConfigPath("CODEX_HOME", ".codex", "hooks.json")
	if err != nil {
		return err
	}
	events := append(slices.Clone(hooks.SessionEvents), "PostToolUse", "Interrupt")
	return hooks.Install(path, state.Codex, events)
}

func UninstallHooks() error {
	path, err := hooks.ConfigPath("CODEX_HOME", ".codex", "hooks.json")
	if err != nil {
		return err
	}
	return hooks.Uninstall(path, state.Codex)
}
