package claude

import (
	"io"
	"slices"

	"github.com/dvpervov/aox/internal/hooks"
	"github.com/dvpervov/aox/internal/state"
)

func HandleEvent(r io.Reader) error {
	return hooks.HandleEvent(r, state.Claude)
}

func InstallHooks() error {
	path, err := hooks.ConfigPath("CLAUDE_CONFIG_DIR", ".claude", "settings.json")
	if err != nil {
		return err
	}
	return hooks.Install(path, state.Claude, append(slices.Clone(hooks.SessionEvents), "PostToolUse", hooks.NotificationEvent))
}

func UninstallHooks() error {
	path, err := hooks.ConfigPath("CLAUDE_CONFIG_DIR", ".claude", "settings.json")
	if err != nil {
		return err
	}
	return hooks.Uninstall(path, state.Claude)
}
