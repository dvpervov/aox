# aox

aox is a tmux-based session manager for Claude Code and Codex. It shows active sessions, their status, and which ones need attention. You can preview sessions and switch between them directly.

aox follows the Unix philosophy: do one thing well. It manages agent sessions, leaving terminal management to tmux, code editing to your editor, agent execution to their respective CLIs, and worktrees and diff review to git tools.

It's a single Go binary with no config files, running inside your own tmux with no server or daemon.

```
      Status    Agent     Pane               Title
──────────────────────────────────────────────────────────────
  > ● WAITING   claude    botlab:1.1         fix the parser crash
      WORKING   codex     api:0.0            add pagination to /users
      IDLE      claude    7:0.0              write release notes
      ?         claude    botlab:5.0
```

## Install

Requires tmux 3.0+, Go 1.24+, and Claude Code and/or Codex. Tested on Linux.

```sh
go install github.com/dvpervov/aox@latest
aox hooks install claude    # and/or: aox hooks install codex
```

For Codex, trust the new hooks once with `/hooks` inside Codex. Re-run `aox hooks install` if you move the aox binary. To remove the hooks: `aox hooks uninstall <agent>`.

## Use

Run `aox` — it opens in its own tmux session. Keep starting agents in tmux as you always do; aox picks them up automatically.

From aox you can jump to any session (and back with `prefix a`), see which sessions are blocked on you (also as `● N waiting` in the tmux status bar), preview them live, and start new agents.

Press `h` in aox for keys, and see `aox --help` for commands.
