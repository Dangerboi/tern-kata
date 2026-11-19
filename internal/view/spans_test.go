package view

import (
	"reflect"
	"testing"

	"github.com/stencil-hq/tern-sdk/go/tern/ui"
)

func TestMonoAndRefSpansAreExplicit(t *testing.T) {
	if got := monoSpan("a.go", "muted"); !reflect.DeepEqual(got, (ui.Span{T: "a.go", S: "muted mono"})) {
		t.Errorf("monoSpan = %+v", got)
	}
	if got := refSpan("#ab12", false); !reflect.DeepEqual(got, (ui.Span{T: "#ab12", S: "mono"})) {
		t.Errorf("refSpan = %+v", got)
	}
	if got := refSpan("#ab12", true); !reflect.DeepEqual(got, (ui.Span{T: "#ab12", S: "accent link mono"})) {
		t.Errorf("linked refSpan = %+v", got)
	}
	if got := timeSpan("18:21"); !reflect.DeepEqual(got, (ui.Span{T: "18:21", S: "muted mono"})) {
		t.Errorf("timeSpan = %+v", got)
	}
	if got := keySpan("W"); !reflect.DeepEqual(got, (ui.Span{T: "W", S: KeyStyle})) {
		t.Errorf("keySpan = %+v", got)
	}
}

func TestProseTurnsMarkersIntoKeycaps(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []ui.Span
	}{
		{"Create one with {N}.", []ui.Span{{T: "Create one with ", S: "muted"}, {T: "N", S: KeyStyle}, {T: ".", S: "muted"}}},
		{"{ctrl+o} opens", []ui.Span{{T: "Ctrl+O", S: KeyStyle}, {T: " opens", S: "muted"}}},
		{"a {{literal}} brace", []ui.Span{{T: "a {literal}} brace", S: "muted"}}},
		{"open { brace", []ui.Span{{T: "open { brace", S: "muted"}}},
		{"no keys", []ui.Span{{T: "no keys", S: "muted"}}},
		{"", nil},
	} {
		if got := prose(tc.text, "muted"); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("prose(%q) = %+v, want %+v", tc.text, got, tc.want)
		}
	}
}
