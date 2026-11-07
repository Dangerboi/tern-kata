package data

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestSeenRoundTrip(t *testing.T) {
	at := time.Date(2026, 11, 7, 8, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		save []model.SaveSeen
		want model.SeenLoaded
	}{
		"seen time": {
			save: []model.SaveSeen{{ProjectUID: "P", SetSeen: true, Seen: at}},
			want: model.SeenLoaded{ProjectUID: "P", Seen: at},
		},
		"mode keeps the time": {
			save: []model.SaveSeen{{ProjectUID: "P", SetSeen: true, Seen: at}, {ProjectUID: "P", SetTimeline: true, Timeline: true}},
			want: model.SeenLoaded{ProjectUID: "P", Seen: at, Timeline: true},
		},
		"mode off": {
			save: []model.SaveSeen{{ProjectUID: "P", SetTimeline: true, Timeline: true}, {ProjectUID: "P", SetTimeline: true}},
			want: model.SeenLoaded{ProjectUID: "P"},
		},
		"another Project is untouched": {
			save: []model.SaveSeen{{ProjectUID: "Q", SetSeen: true, Seen: at}},
			want: model.SeenLoaded{ProjectUID: "P"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "seen.toml")
			for _, cmd := range tc.save {
				if err := writeSeen(path, cmd); err != nil {
					t.Fatal(err)
				}
			}
			got, err := readSeen(path, "P")
			if err != nil {
				t.Fatal(err)
			}
			if !got.Seen.Equal(tc.want.Seen) || got.Timeline != tc.want.Timeline || got.ProjectUID != "P" {
				t.Errorf("read %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestSeenMissingFileReadsAsNeverSeen(t *testing.T) {
	got, err := readSeen(filepath.Join(t.TempDir(), "seen.toml"), "P")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Seen.IsZero() || got.Timeline {
		t.Errorf("read %+v, want the zero value", got)
	}
}

func TestSeenBadFileIsAnErrorAndIsNotWritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seen.toml")
	const bad = "[P\nseen = nope"
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSeen(path, "P"); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("read error %v, want one naming %s", err, path)
	}
	if err := writeSeen(path, model.SaveSeen{ProjectUID: "P", SetSeen: true, Seen: time.Now()}); err == nil {
		t.Error("write over a bad file succeeded")
	}
	if text, _ := os.ReadFile(path); string(text) != bad {
		t.Errorf("bad file was changed to %q", text)
	}
}
