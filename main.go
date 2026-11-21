package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/dvpervov/aox/internal/claude"
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
		return tui.Run()
	case "event claude":
		return claude.HandleEvent(os.Stdin)
	case "hooks install claude":
		return claude.InstallHooks()
	case "hooks uninstall claude":
		return claude.UninstallHooks()
	}
	return errors.New("usage: aox [event claude | hooks install claude | hooks uninstall claude]")
}
