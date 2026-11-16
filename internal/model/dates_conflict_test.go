package model_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestADatesRefusalSaysNothingWasWritten(t *testing.T) {
	for _, read := range []bool{true, false} {
		got := model.DatesConflict{Label: "Deadline", Key: "deadline_on", Read: read}.Error()
		if !strings.Contains(got, "nothing was written") {
			t.Errorf("read %v: refusal %q", read, got)
		}
	}
}
