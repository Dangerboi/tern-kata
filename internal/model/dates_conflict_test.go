package model_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestADatesRefusalAfterASavedKeyNamesWhatWasWritten(t *testing.T) {
	none := model.DatesConflict{Label: "Deadline", Key: "deadline_on", Read: true}.Error()
	if !strings.Contains(none, "nothing was written") {
		t.Errorf("a refusal before any write: %q", none)
	}
	for _, read := range []bool{true, false} {
		got := model.DatesConflict{Label: "Deadline", Key: "deadline_on", Read: read, Saved: []string{"Scheduled"}}.Error()
		if strings.Contains(got, "nothing was written") || !strings.Contains(got, "already saved: Scheduled") {
			t.Errorf("read %v: refusal after a saved key: %q", read, got)
		}
	}
}
