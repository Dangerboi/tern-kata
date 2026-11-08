package model_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestProjectGroupsAreOrderedByName(t *testing.T) {
	mk := func(uid, proj, puid string) model.Issue {
		return model.Issue{UID: uid, Status: "open", ProjectName: proj, ProjectUID: &puid, UpdatedAt: time.Now()}
	}
	groups := model.GroupIssues(model.Collection{View: "all-open", Title: "All Open"},
		[]model.Issue{mk("1", "tern", "p1"), mk("2", "tern-kata", "p2"), mk("3", "app 2", "p3"), mk("4", "app", "p4")}, model.NewPaneCalendar("2026-10-19", "UTC", ""))
	var got []string
	for _, g := range groups {
		got = append(got, g.Title)
	}
	if want := "app,app 2,tern,tern-kata"; strings.Join(got, ",") != want {
		t.Errorf("groups %v, want %s", got, want)
	}
}
