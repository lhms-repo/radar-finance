package auth

import (
	"testing"
	"time"
)

// TestThrottle walks through sequences of login attempts and checks blocking.
func TestThrottle(t *testing.T) {
	type step struct {
		advance     time.Duration
		action      string // "fail", "ok" or "check"
		key         string
		wantAllowed bool
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{
			name: "allows attempts below the limit",
			steps: []step{
				{action: "fail", key: "ana"},
				{action: "fail", key: "ana"},
				{action: "check", key: "ana", wantAllowed: true},
			},
		},
		{
			name: "blocks after the limit",
			steps: []step{
				{action: "fail", key: "ana"},
				{action: "fail", key: "ana"},
				{action: "fail", key: "ana"},
				{action: "check", key: "ana", wantAllowed: false},
			},
		},
		{
			name: "block is per login and ignores case",
			steps: []step{
				{action: "fail", key: "ana"},
				{action: "fail", key: "ANA"},
				{action: "fail", key: "Ana"},
				{action: "check", key: "ana", wantAllowed: false},
				{action: "check", key: "bruno", wantAllowed: true},
			},
		},
		{
			name: "block ends after the lock period",
			steps: []step{
				{action: "fail", key: "ana"},
				{action: "fail", key: "ana"},
				{action: "fail", key: "ana"},
				{advance: 5 * time.Minute, action: "check", key: "ana", wantAllowed: true},
			},
		},
		{
			name: "failures outside the window are forgotten",
			steps: []step{
				{action: "fail", key: "ana"},
				{action: "fail", key: "ana"},
				{advance: 16 * time.Minute, action: "fail", key: "ana"},
				{action: "check", key: "ana", wantAllowed: true},
			},
		},
		{
			name: "success resets the counter",
			steps: []step{
				{action: "fail", key: "ana"},
				{action: "fail", key: "ana"},
				{action: "ok", key: "ana"},
				{action: "fail", key: "ana"},
				{action: "check", key: "ana", wantAllowed: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
			th := NewThrottle(3, 15*time.Minute, 5*time.Minute, func() time.Time { return now })
			for i, s := range tt.steps {
				now = now.Add(s.advance)
				switch s.action {
				case "fail":
					th.Failure(s.key)
				case "ok":
					th.Success(s.key)
				case "check":
					allowed, wait := th.Allowed(s.key)
					if allowed != s.wantAllowed {
						t.Fatalf("step %d: allowed = %v, want %v", i, allowed, s.wantAllowed)
					}
					if !allowed && wait <= 0 {
						t.Fatalf("step %d: blocked without a wait time", i)
					}
				}
			}
		})
	}
}
