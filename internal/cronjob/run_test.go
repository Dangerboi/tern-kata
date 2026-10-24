package cronjob_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

func TestReadRunStatus(t *testing.T) {
	cases := []struct {
		text string
		want cronjob.RunStatus
	}{
		{"running", cronjob.RunRunning},
		{"succeeded", cronjob.RunSucceeded},
		{"failed", cronjob.RunFailed},
		{"cancelled", cronjob.RunCancelled},
		{"unknown", cronjob.RunUnknown},
		{"paused", cronjob.RunUnknown},
		{"Succeeded", cronjob.RunUnknown},
		{"", cronjob.RunUnknown},
	}
	for _, tc := range cases {
		if got := cronjob.ReadRunStatus(tc.text); got != tc.want {
			t.Errorf("ReadRunStatus(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}
