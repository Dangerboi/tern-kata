package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestCodeWords(t *testing.T) {
	for _, tt := range []struct {
		name      string
		fn        func(string) string
		code, out string
	}{
		{"hold", model.HoldWords, "agent_quota", "Quota reached"},
		{"hold", model.HoldWords, "secret_unavailable", "Secret unavailable"},
		{"hold unknown", model.HoldWords, "from_the_future", "from_the_future"},
		{"close", model.CloseReasonWords, "wontfix", "Won't fix"},
		{"close", model.CloseReasonWords, "audit-no-change", "No change"},
		{"close unknown", model.CloseReasonWords, "later", "later"},
		{"relation", model.RelationWords, "blocked_by", "blocked by"},
		{"relation unknown", model.RelationWords, "x_of", "x_of"},
	} {
		if got := tt.fn(tt.code); got != tt.out {
			t.Errorf("%s %q = %q, want %q", tt.name, tt.code, got, tt.out)
		}
	}
}

func TestCountAndNoun(t *testing.T) {
	for _, tt := range []struct {
		n         int
		one, many string
		count     string
		noun      string
	}{
		{0, "daemon", "", "0 daemons", "daemons"},
		{1, "daemon", "", "1 daemon", "daemon"},
		{3, "daemon", "", "3 daemons", "daemons"},
		{2, "federation", "federations", "2 federations", "federations"},
		{2, "Issue", "Issues", "2 Issues", "Issues"},
	} {
		if got := model.Count(tt.n, tt.one, tt.many); got != tt.count {
			t.Errorf("Count(%d) = %q, want %q", tt.n, got, tt.count)
		}
		if got := model.Noun(tt.n, tt.one, tt.many); got != tt.noun {
			t.Errorf("Noun(%d) = %q, want %q", tt.n, got, tt.noun)
		}
	}
}

func TestRefNameFollowsTheCollection(t *testing.T) {
	var m model.Model
	if got := m.RefName("docs", "ab12"); got != "docs#ab12" {
		t.Errorf("RefName with no Project Collection = %q", got)
	}
	if got := m.RefName("", "ab12"); got != "#ab12" {
		t.Errorf("RefName without a Project = %q", got)
	}
}
