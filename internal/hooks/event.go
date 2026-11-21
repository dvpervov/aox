package hooks

import (
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/dvpervov/aox/internal/state"
	"github.com/dvpervov/aox/internal/tmux"
)

const titleLen = 60

var SessionEvents = []string{
	"SessionStart",
	"UserPromptSubmit",
	"PermissionRequest",
	"PreToolUse",
	"Stop",
	"SessionEnd",
}

var transitions = map[string]state.State{
	"SessionStart":      state.Unknown,
	"UserPromptSubmit":  state.Working,
	"PermissionRequest": state.Waiting,
	"PreToolUse":        state.Working,
	"PostToolUse":       state.Working,
	"Stop":              state.Idle,
	"Interrupt":         state.Idle,
}

type hookInput struct {
	SessionID string `json:"session_id"`
	Event     string `json:"hook_event_name"`
	Prompt    string `json:"prompt"`
}

func HandleEvent(r io.Reader, agent string) error {
	if err := handle(r, agent); err != nil {
		return err
	}
	return state.Touch()
}

func handle(r io.Reader, agent string) error {
	var in hookInput
	if err := json.NewDecoder(r).Decode(&in); err != nil {
		return err
	}
	pane := os.Getenv("TMUX_PANE")
	if pane == "" || in.SessionID == "" {
		return nil
	}
	if agent == state.Codex {
		// A shared Codex daemon can pass along a different client's TMUX_PANE.
		if !tmux.InPane(pane) {
			return nil
		}
		in.SessionID = agent + ":" + in.SessionID
	}
	own, err := state.Load(in.SessionID)
	if err != nil {
		return err
	}
	if in.Event == "SessionEnd" {
		if own == nil || own.Pane != pane {
			return nil
		}
		return state.Remove(own.ID)
	}
	next, ok := transitions[in.Event]
	if !ok {
		return nil
	}
	s, err := associate(in, pane, agent, own)
	if err != nil || s == nil {
		return err
	}
	s.State = next
	if in.Event == "UserPromptSubmit" && s.Prompt == "" {
		s.Prompt = summarize(in.Prompt)
	}
	return state.Save(s)
}

func associate(in hookInput, pane, agent string, own *state.Session) (*state.Session, error) {
	if own != nil && own.Pane == pane {
		return own, nil
	}
	sessions, err := state.List()
	if err != nil {
		return nil, err
	}
	var current []state.Session
	for _, s := range sessions {
		if s.Pane == pane && s.AgentName() == agent && s.ID != in.SessionID {
			current = append(current, s)
		}
	}
	if in.Event != "SessionStart" && len(current) > 0 {
		return nil, nil
	}
	if cmd, err := tmux.CurrentCommand(pane); err != nil || cmd != agent {
		return nil, nil
	}
	for _, s := range current {
		if err := state.Remove(s.ID); err != nil {
			return nil, err
		}
	}
	if own == nil {
		own = &state.Session{ID: in.SessionID, Agent: agent}
	}
	own.Pane = pane
	return own, nil
}

var pastedBlock = regexp.MustCompile(`(?s)<pasted_content[^>]*>.*?</pasted_content[^>]*>`)

func summarize(prompt string) string {
	prompt = pastedBlock.ReplaceAllString(prompt, "[pasted]")
	r := []rune(strings.Join(strings.Fields(prompt), " "))
	if len(r) <= titleLen {
		return string(r)
	}
	return string(r[:titleLen-1]) + "…"
}
