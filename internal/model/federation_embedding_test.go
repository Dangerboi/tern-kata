package model

import (
	"slices"
	"testing"
)

func TestTheSpokeDetailShowsTheEmbeddingStatusAboveTheProject(t *testing.T) {
	spoke := FederationSpoke{ProjectName: "acme", Embedding: []string{"embedding: degraded", "embedding producer: none"}}
	lines := spoke.DetailLines()
	if !slices.Equal(lines[:4], []string{"embedding: degraded", "embedding producer: none", "", "acme"}) {
		t.Errorf("detail starts %q, want the embedding fields, a blank line, then the project", lines[:4])
	}
	if got := (FederationSpoke{ProjectName: "acme"}).DetailLines()[0]; got != "acme" {
		t.Errorf("a spoke without embedding starts with %q", got)
	}
}
