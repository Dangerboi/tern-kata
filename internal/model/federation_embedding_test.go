package model

import (
	"testing"
	"time"
)

func TestTheSpokeDetailShowsTheEmbeddingStatusAboveTheProject(t *testing.T) {
	spoke := FederationSpoke{ProjectName: "acme", Embedding: []string{"embedding: degraded", "embedding producer: none"}}
	rows := spoke.DetailRows(PaneCalendar{}, time.Time{})
	want := []FactRow{{Value: "embedding: degraded"}, {Value: "embedding producer: none"}, {}, {Key: "Project", Value: "acme"}}
	for i, w := range want {
		if rows[i] != w {
			t.Errorf("row %d is %+v, want %+v: the embedding fields, a group break, then the Project", i, rows[i], w)
		}
	}
	if got := (FederationSpoke{ProjectName: "acme"}).DetailRows(PaneCalendar{}, time.Time{})[0]; got != (FactRow{Key: "Project", Value: "acme"}) {
		t.Errorf("a spoke without embedding starts with %+v", got)
	}
}
