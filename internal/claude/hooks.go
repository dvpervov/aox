package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dvpervov/aox/internal/state"
)

var hookEvents = []string{
	"SessionStart",
	"UserPromptSubmit",
	"PermissionRequest",
	"PreToolUse",
	"Stop",
	"SessionEnd",
}

const eventArgs = " event claude"

func InstallHooks() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	command := "'" + exe + "'" + eventArgs
	return editHooks(func(hooks map[string]any) error {
		removeOwned(hooks)
		for _, event := range hookEvents {
			groups, ok := hooks[event].([]any)
			if !ok && hooks[event] != nil {
				return fmt.Errorf("hooks.%s is not an array", event)
			}
			hooks[event] = append(groups, map[string]any{
				"hooks": []any{map[string]any{"type": "command", "command": command}},
			})
		}
		return nil
	})
}

func UninstallHooks() error {
	return editHooks(func(hooks map[string]any) error {
		removeOwned(hooks)
		return nil
	})
}

func settingsPath() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func editHooks(edit func(map[string]any) error) error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	var settings map[string]any
	perm := fs.FileMode(0o600)
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&settings); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if fi, err := os.Stat(path); err == nil {
			perm = fi.Mode().Perm()
		}
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	if settings == nil {
		settings = map[string]any{}
	}
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		if settings["hooks"] != nil {
			return fmt.Errorf("%s: hooks is not an object", path)
		}
		hooks = map[string]any{}
	}
	if err := edit(hooks); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	} else {
		settings["hooks"] = hooks
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(settings); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return state.WriteFile(path, out.Bytes(), perm)
}

func removeOwned(hooks map[string]any) {
	for event, v := range hooks {
		groups, ok := v.([]any)
		if !ok {
			continue
		}
		var kept []any
		removed := false
		for _, g := range groups {
			group, ok := g.(map[string]any)
			inner, ok2 := group["hooks"].([]any)
			if !ok || !ok2 {
				kept = append(kept, g)
				continue
			}
			var keptInner []any
			for _, h := range inner {
				if isOwned(h) {
					removed = true
				} else {
					keptInner = append(keptInner, h)
				}
			}
			if len(keptInner) == len(inner) {
				kept = append(kept, g)
			} else if len(keptInner) > 0 {
				group["hooks"] = keptInner
				kept = append(kept, group)
			}
		}
		switch {
		case !removed:
		case len(kept) == 0:
			delete(hooks, event)
		default:
			hooks[event] = kept
		}
	}
}

func isOwned(h any) bool {
	m, ok := h.(map[string]any)
	if !ok {
		return false
	}
	cmd, _ := m["command"].(string)
	exe, found := strings.CutSuffix(cmd, eventArgs)
	return found && filepath.Base(strings.Trim(exe, "'")) == "aox"
}
