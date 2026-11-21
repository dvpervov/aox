package hooks

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

func Install(path, agent string, events []string) error {
	eventArgs := " event " + agent
	command, err := hookCommand(eventArgs)
	if err != nil {
		return err
	}
	return editHooks(path, func(hooks map[string]any) error {
		removeOwned(hooks, eventArgs, command)
		for _, event := range events {
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

func Uninstall(path, agent string) error {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	eventArgs := " event " + agent
	command, _ := hookCommand(eventArgs)
	return editHooks(path, func(hooks map[string]any) error {
		removeOwned(hooks, eventArgs, command)
		return nil
	})
}

func ConfigPath(env, dir, file string) (string, error) {
	if override := os.Getenv(env); override != "" {
		return filepath.Join(override, file), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, dir, file), nil
}

func hookCommand(eventArgs string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	return "'" + strings.ReplaceAll(exe, "'", "'\\''") + "'" + eventArgs, nil
}

func editHooks(path string, edit func(map[string]any) error) error {
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

func removeOwned(hooks map[string]any, eventArgs, command string) {
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
				if isOwned(h, eventArgs, command) {
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

func isOwned(h any, eventArgs, command string) bool {
	m, ok := h.(map[string]any)
	if !ok || m["type"] != "command" {
		return false
	}
	cmd, _ := m["command"].(string)
	exe, found := strings.CutSuffix(cmd, eventArgs)
	return found && (cmd == command || filepath.Base(strings.Trim(exe, "'")) == "aox")
}
