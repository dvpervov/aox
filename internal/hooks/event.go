package hooks

import (
	"encoding/json"
	"io"
	"os"
	"regexp"
	"slices"
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

const (
	NotificationEvent = "Notification"
	StopFailureEvent  = "StopFailure"
	rateLimitError    = "rate_limit"
	idleNotification  = "idle_prompt"
	quotaResumed      = "quota_auto_resume_fired"
	quotaDisabled     = "quota_auto_resume_disabled"
)

var waitingNotifications = []string{
	"permission_prompt",
	"elicitation_dialog",
	"elicitation_url_dialog",
	"quota_auto_resume_stale",
}

var matchers = map[string]string{
	NotificationEvent: strings.Join(append(slices.Clone(waitingNotifications), idleNotification, quotaResumed, quotaDisabled), "|"),
	StopFailureEvent:  rateLimitError,
}

type hookInput struct {
	SessionID        string `json:"session_id"`
	Event            string `json:"hook_event_name"`
	Prompt           string `json:"prompt"`
	ToolName         string `json:"tool_name"`
	NotificationType string `json:"notification_type"`
	Error            string `json:"error"`
}

func (in hookInput) state() (state.State, bool) {
	switch {
	case in.Event == "PreToolUse" && in.ToolName == "AskUserQuestion":
		return state.Waiting, true
	case in.Event == StopFailureEvent:
		return state.Paused, in.Error == "" || in.Error == rateLimitError
	case in.Event == NotificationEvent && in.NotificationType == quotaResumed:
		return state.Working, true
	case in.Event == NotificationEvent && (in.NotificationType == idleNotification || in.NotificationType == quotaDisabled):
		return state.Idle, true
	case in.Event == NotificationEvent:
		return state.Waiting, slices.Contains(waitingNotifications, in.NotificationType)
	}
	next, ok := transitions[in.Event]
	return next, ok
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
	unlock, err := state.Lock()
	if err != nil {
		return err
	}
	defer unlock()
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
	next, ok := in.state()
	if !ok {
		return nil
	}
	s, err := associate(in, pane, agent, own)
	if err != nil || s == nil {
		return err
	}
	if in.Event == NotificationEvent {
		switch in.NotificationType {
		case idleNotification:
			// idle_prompt doesn't follow an Esc interrupt (verified), but may end WORKING/PAUSED in other cases; never override WAITING.
			if s.State != state.Working && s.State != state.Paused {
				return nil
			}
		case quotaDisabled:
			if s.State != state.Paused {
				return nil
			}
		}
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
