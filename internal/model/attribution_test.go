package model_test

import (
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestCreationLine(t *testing.T) {
	for _, tt := range []struct {
		name string
		a    model.Attribution
		want string
	}{
		{"no verification state", model.Attribution{Source: "dana", Teammate: "bot"}, ""},
		{"verified with accountable and teammate", model.Attribution{Verification: "verified", Accountable: "alice", Source: "agent", Teammate: "bot"}, "creation: verified · accountable: alice · source: agent / bot"},
		{"verified without teammate", model.Attribution{Verification: "verified", Accountable: "alice", Source: "agent"}, "creation: verified · accountable: alice · source: agent"},
		{"verified without accountable", model.Attribution{Verification: "verified", Source: "agent"}, "creation: verified · source: agent"},
		{"pending hides accountable", model.Attribution{Verification: "pending", Accountable: "alice", Source: "agent", Teammate: "bot"}, "creation: pending · source: agent / bot"},
		{"legacy falls back to the author", model.Attribution{Verification: "legacy"}, "creation: legacy · source: fallback"},
		{"unknown reads legacy", model.Attribution{Verification: "weird", Accountable: "alice", Source: "agent"}, "creation: legacy · source: agent"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Line("fallback"); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
