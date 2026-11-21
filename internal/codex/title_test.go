package codex

import (
	"testing"

	"github.com/dvpervov/aox/internal/state"
)

func TestTitleStatus(t *testing.T) {
	var activityTitle string
	for _, step := range []struct {
		title string
		want  state.State
		label string
	}{
		{"Review notes · aox", state.Unknown, "Review notes · aox"},
		{"⠙ Review notes · aox", state.Working, "Review notes · aox"},
		{"[ ! ] Action Required — Review notes · aox", state.Waiting, "Review notes · aox"},
		{"● [ . ] Action Required — Review notes · aox", state.Waiting, "Review notes · aox"},
		{"Review notes · aox", state.Idle, "Review notes · aox"},
		{"Review notes · aox", state.Idle, "Review notes · aox"},
		{"A different task · aox", state.Unknown, "A different task · aox"},
		{"Review notes · aox", state.Unknown, "Review notes · aox"},
		{"⠹", state.Working, ""},
	} {
		got, nextTitle := TitleStatus(step.title, activityTitle)
		if got != step.want {
			t.Errorf("TitleStatus(%q, %q) = %s, want %s", step.title, activityTitle, got, step.want)
		}
		if title := PaneTitle(step.title); title != step.label {
			t.Errorf("PaneTitle(%q) = %q, want %q", step.title, title, step.label)
		}
		activityTitle = nextTitle
	}
}
