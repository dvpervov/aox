package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/dvpervov/aox/internal/claude"
	"github.com/dvpervov/aox/internal/codex"
	"github.com/dvpervov/aox/internal/tmux"
	"github.com/dvpervov/aox/internal/tui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "aox:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	switch strings.Join(args, " ") {
	case "":
		return runTUI()
	case "event claude":
		return claude.HandleEvent(os.Stdin)
	case "hooks install claude":
		return claude.InstallHooks()
	case "hooks uninstall claude":
		return claude.UninstallHooks()
	case "event codex":
		return codex.HandleEvent(os.Stdin, os.Stdout)
	case "hooks install codex":
		return codex.InstallHooks()
	case "hooks uninstall codex":
		return codex.UninstallHooks()
	}
	return errors.New("usage: aox [event <claude|codex> | hooks <install|uninstall> <claude|codex>]")
}

const tuiSession = "aox"

func runTUI() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	command := "'" + exe + "'"
	if os.Getenv("TMUX") == "" {
		return tmux.ExecSession(tuiSession, command)
	}
	session, err := tmux.SessionOf(os.Getenv("TMUX_PANE"))
	if err != nil {
		return err
	}
	if session != tuiSession {
		return tmux.SwitchToSession(tuiSession, command)
	}
	return tui.Run()
}
