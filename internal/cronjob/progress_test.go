package cronjob_test

import (
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/cronjob"
)

func TestReadProgress(t *testing.T) {
	anchor := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	through := time.Date(2026, 10, 1, 9, 30, 15, 500, time.UTC)
	cases := []struct {
		name  string
		value any
		want  cronjob.Progress
		ok    bool
	}{
		{"a good value", map[string]any{"anchor": "2026-10-01T08:00:00Z", "through": "2026-10-01T09:30:15.0000005Z"}, cronjob.Progress{Anchor: anchor, Through: through}, true},
		{"members kata adds later are ignored", map[string]any{"anchor": "2026-10-01T08:00:00Z", "through": "2026-10-01T09:30:15.0000005Z", "pending": 3, "note": "x"}, cronjob.Progress{Anchor: anchor, Through: through}, true},
		{"a missing through", map[string]any{"anchor": "2026-10-01T08:00:00Z"}, cronjob.Progress{}, false},
		{"a missing anchor", map[string]any{"through": "2026-10-01T08:00:00Z"}, cronjob.Progress{}, false},
		{"an anchor that is not an instant", map[string]any{"anchor": "yesterday", "through": "2026-10-01T08:00:00Z"}, cronjob.Progress{}, false},
		{"an anchor that is not text", map[string]any{"anchor": 5, "through": "2026-10-01T08:00:00Z"}, cronjob.Progress{}, false},
		{"a through that is not an instant", map[string]any{"anchor": "2026-10-01T08:00:00Z", "through": ""}, cronjob.Progress{}, false},
		{"not an object", "2026-10-01T08:00:00Z", cronjob.Progress{}, false},
		{"no value", nil, cronjob.Progress{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := cronjob.ReadProgress(tc.value)
			if ok != tc.ok || (ok && (!got.Anchor.Equal(tc.want.Anchor) || !got.Through.Equal(tc.want.Through))) {
				t.Errorf("ReadProgress = %+v, %v; want %+v, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestProgressKeyPrefix(t *testing.T) {
	if cronjob.ProgressKeyPrefix != "cron.progress." {
		t.Errorf("ProgressKeyPrefix = %q; kata keeps a Native job's delivery state under cron.progress.<job uid>", cronjob.ProgressKeyPrefix)
	}
}
