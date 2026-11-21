package codex

import (
	"fmt"
	"io"
	"slices"

	"github.com/dvpervov/aox/internal/hooks"
	"github.com/dvpervov/aox/internal/state"
)

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
