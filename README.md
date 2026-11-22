# aox

See all your coding agents — Claude Code and Codex — running in tmux, in one place: what each is working on, which ones are busy, and which are waiting for you. Jump to any of them with one key.

Built for terminal-first developers: it lives in tmux next to your agents, editor and shell, and never asks you to leave the keyboard.

```
      Status    Agent     Pane               Title
──────────────────────────────────────────────────────────────
  > ● WAITING   claude    botlab:1.1         fix the parser crash
      WORKING   codex     api:0.0            add pagination to /users
      IDLE      claude    7:0.0              write release notes
      ?         claude    botlab:5.0
```

You can keep starting and using agents exactly as before — aox picks them up automatically.

## Requirements

- **tmux** 3.0 or newer — agents must run inside tmux panes (tested with 3.4).
- **Claude Code** and/or **Codex CLI** (Codex with hooks support; tested with 0.171).
- **Go** 1.24 or newer to build.
- Linux (other Unix systems may work but are untested).

## Install

```sh
go install github.com/dvpervov/aox@latest
```

Then register the hooks for the agents you use (once):

```sh
aox hooks install claude    # adds hooks to ~/.claude/settings.json
aox hooks install codex     # adds hooks to ~/.codex/hooks.json
```

For Codex, open Codex once and trust the new hooks with `/hooks` — Codex skips hooks until you do.

The hooks point to the aox binary by its full path. If you move or reinstall aox elsewhere, run `aox hooks install` again.

## Use

Run `aox` from anywhere. It opens in its own tmux session named `aox` (inside tmux it switches you there; outside tmux it starts or attaches tmux). Press `h` in aox for keys, and see `aox --help` for commands.

Agents appear automatically, with their status:

| Status | Meaning |
|---|---|
| `WAITING` (red, ●) | needs your input — e.g. a permission prompt or a question |
| `WORKING` | busy with your request |
| `IDLE` | finished its turn |
| `?` | not enough information yet |

What you can do:

- **Jump** to any session's pane, and come back to aox from anywhere with **`prefix a`** (e.g. `ctrl+b a`) — set while aox runs, your tmux config untouched.
- **Notice** sessions that need you: the tmux status bar shows **`● N waiting`** from any pane.
- **Preview** the selected session live on wide terminals (100+ columns).
- **Sort** by status, agent, pane or title, and go to any row with a couple of keys.
- **Start** a new Claude or Codex agent in a directory of your choice; it opens in the tmux session `aox-agents`, and you return to aox when it exits.
- **Rename** sessions; a title is the first prompt unless you give it a name, and names stay with the tmux pane.

## Codex notes

Codex normally runs sessions through a shared background daemon, and its hooks can't tell which tmux pane they belong to. aox ignores those events, so such sessions get their status from Codex's terminal title instead and may show `?` until Codex starts working. For full status and prompt titles, start Codex with:

```sh
codex --no-daemon
```

aox uses `$CODEX_HOME/hooks.json` when `CODEX_HOME` is set, and `$CLAUDE_CONFIG_DIR/settings.json` for Claude when `CLAUDE_CONFIG_DIR` is set.

## Uninstall

```sh
aox hooks uninstall claude
aox hooks uninstall codex
```

This removes only aox's hooks. aox's state lives in `~/.local/state/aox/` and can be deleted.

## aox vs GUI agent IDEs

GUI agent IDEs (Cursor-like) bundle agents, editor, diffs and git into one application. aox follows the Unix way: it does one thing — managing coding-agent sessions: monitoring, launching and navigating them — and composes with the tools you already use. tmux runs the sessions, the agents' own CLIs do the work, and your editor and git handle the code.

| | aox | GUI agent IDEs |
|---|---|---|
| Overview of all agent sessions and their status | Yes | Yes |
| Highlights sessions waiting for you | Yes | Yes |
| Live preview, quick switching, renaming | Yes | Yes |
| Start new agents | Yes | Yes |
| Picks up agents you started yourself, anywhere in tmux | Yes | Only its own sessions |
| Uses the official Claude Code and Codex CLIs, unchanged | Yes | Agents run inside the IDE |
| Works over SSH, in any terminal | Yes | Desktop app |
| Footprint | One small binary, no daemon | Full desktop application |
| Editor, diff review, git | Your own (Vim/Neovim, git, …) | Built in |

Choose aox if you live in the terminal and want to keep your editor, your git workflow and the agents' own CLIs, while still seeing at a glance which agent needs you.

## How it works

Claude Code and Codex call `aox event <agent>` on lifecycle events (prompt submitted, tool use, permission request, turn finished, session start/end). aox records the latest state per session in small JSON files under `~/.local/state/aox/sessions/`, and the TUI matches them to tmux panes. There is no daemon or server; tmux and the agents remain the source of truth.
