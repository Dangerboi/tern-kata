package model_test

import (
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestADatesRefusalSaysNothingWasWritten(t *testing.T) {
	got := model.DatesConflict{Label: "Deadline", Key: "deadline_on"}.Error()
	if !strings.Contains(got, "nothing was written") {
		t.Errorf("refusal %q", got)
	}
}
