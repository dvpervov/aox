# aox

A minimal terminal manager for coding-agent sessions (Claude Code, Codex) running in tmux.

## Problem

With several agents running across tmux windows and panes, finding out which sessions are active, what each is doing, and which need your input means switching between windows by hand. aox solves observability and navigation across agent sessions — not agent execution.

## Goals

- **Discovery** — automatically find agent sessions running inside tmux.
- **Overview** — show all sessions, their tasks, and statuses in one TUI.
- **Attention detection** — highlight sessions waiting for your input.
- **Navigation** — jump from the TUI straight to the session's tmux pane.
- **Non-intrusive** — keep using Claude Code and Codex normally; no special launcher or registration.

## Usage

```sh
aox hooks install claude    # once; register Claude Code hooks in ~/.claude/settings.json
aox                         # run the TUI in its own tmux window
aox hooks uninstall claude  # remove aox hooks
```

In the TUI: `j`/`k` or arrows to move, `enter` to jump to the session's pane, `r` to rename, `q` to quit.

While the TUI runs, `prefix a` jumps back to it from any tmux pane. The binding is set on the running tmux server when aox starts and removed when it quits; no tmux config changes are needed.

## Principles

- tmux owns sessions, panes, and navigation; agent CLIs own reasoning, history, and tools; aox only observes and navigates.
- Native interfaces over scraping: tmux commands and agent hooks.
- No daemon, server, or database — small local state files only.
- Conservative status: prefer `UNKNOWN` over unsupported conclusions.
- No speculative features.
