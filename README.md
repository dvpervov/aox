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
aox hooks install codex     # once; register Codex hooks in ~/.codex/hooks.json
aox                         # open the TUI; it always runs in tmux session "aox" (created on first use)
aox hooks uninstall claude  # remove aox hooks
aox hooks uninstall codex
```

After installing Codex hooks, open Codex in tmux and review and trust the aox hooks with `/hooks`. The installer uses `$CODEX_HOME/hooks.json` when `CODEX_HOME` is set. If your Codex version uses the shared app-server daemon, launch with `codex --no-daemon` to get hook-based status and prompt titles. The shared daemon can supply the wrong pane to hooks, so aox only accepts Codex events launched from the reported pane. Without hook state, aox reads Codex's terminal title activity markers and shows `UNKNOWN` until it sees a recognizable status. When there's no saved session title, aox displays the Codex terminal title with activity markers removed. See the [Codex hooks documentation](https://learn.chatgpt.com/docs/hooks) and [upstream pane attribution issue](https://github.com/openai/codex/issues/48500).

In the TUI: `j`/`k` or arrows to move, `enter` to jump to the session's pane, `esc` to go back to the previous tmux session, `r` to rename, `q` to quit.

While the TUI runs, `prefix a` or `Alt+a` jumps back to it from any tmux pane. The bindings are set on the running tmux server when aox starts and removed when it quits; no tmux config changes are needed.

## Principles

- tmux owns sessions, panes, and navigation; agent CLIs own reasoning, history, and tools; aox only observes and navigates.
- Native interfaces over scraping: tmux commands and agent hooks.
- No daemon, server, or database — small local state files only.
- Conservative status: prefer `UNKNOWN` over unsupported conclusions.
- No speculative features.
