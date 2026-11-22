package hooks

import (
	"encoding/json"
	"testing"

	"github.com/dvpervov/aox/internal/state"
)

func TestQuotaEvents(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    state.State
		handled bool
	}{
		{"rate limit", `{"hook_event_name":"StopFailure","error":"rate_limit"}`, state.Paused, true},
		{"rate limit, no error field", `{"hook_event_name":"StopFailure"}`, state.Paused, true},
		{"unrelated API error", `{"hook_event_name":"StopFailure","error":"server_error"}`, state.Paused, false},
		{"retry started", `{"hook_event_name":"Notification","notification_type":"quota_auto_resume_fired"}`, state.Working, true},
		{"retry disabled", `{"hook_event_name":"Notification","notification_type":"quota_auto_resume_disabled"}`, state.Idle, true},
		{"retry needs input", `{"hook_event_name":"Notification","notification_type":"quota_auto_resume_stale"}`, state.Waiting, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var in hookInput
			if err := json.Unmarshal([]byte(tt.payload), &in); err != nil {
				t.Fatal(err)
			}
			got, handled := in.state()
			if handled != tt.handled || (handled && got != tt.want) {
				t.Errorf("state() = (%q, %t), want (%q, %t)", got, handled, tt.want, tt.handled)
			}
		})
	}
}
