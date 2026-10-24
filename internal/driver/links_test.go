package driver_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/driver"
	"github.com/Dangerboi/tern-kata/internal/model"
)

func keysOf(text string) []any {
	in := make([]any, 0, len(text)+1)
	for _, r := range text {
		in = append(in, key(string(r)))
	}
	return in
}

// onAlphaDetail is the Pane with alpha's Issue detail focused.
func onAlphaDetail(t *testing.T) driver.State {
	t.Helper()
	return feed(t, browsing(t, 160), key("enter")).State
}

func TestBAndLOpenTheLinkFormAndSendOneLinkWriteOfTheirType(t *testing.T) {
	for _, tt := range []struct {
		chord  string
		action model.Action
		title  string
	}{
		{"b", model.AddBlockerAction, "This Issue blocks (Issue id)"},
		{"l", model.AddRelatedAction, "Related to (Issue id)"},
	} {
		for _, ref := range []string{"bbbb", "#bbbb", "other#bbbb"} {
			s := feed(t, onAlphaDetail(t), key(tt.chord)).State
			if !s.Model.FieldForm.Open || s.Model.FieldForm.Action != tt.action || s.Model.FieldForm.UID != "alpha" {
				t.Fatalf("%s opened %+v", tt.chord, s.Model.FieldForm)
			}
			if got := model.FieldTitle(tt.action); got != tt.title {
				t.Fatalf("title %q, want %q", got, tt.title)
			}
			out := feed(t, s, append(keysOf(ref), key("enter"))...)
			want := model.WriteField{Workspace: "/repo", UID: "alpha", Action: tt.action, Value: strings.TrimPrefix(ref, "#")}
			if !reflect.DeepEqual(out.Cmds, []model.Cmd{want}) {
				t.Fatalf("%s %q sent %#v, want %#v", tt.chord, ref, out.Cmds, want)
			}
		}
	}
}

func TestAKataRefusalOfALinkShowsOnTheFieldAndKeepsItOpen(t *testing.T) {
	for _, chord := range []string{"b", "l"} {
		s := feed(t, onAlphaDetail(t), key(chord)).State
		s = feed(t, s, append(keysOf("aaaa"), key("enter"), model.FieldWritten{UID: "alpha", Err: errors.New("an issue cannot link to itself")})...).State
		if !s.Model.FieldForm.Open || s.Model.FieldForm.Err == nil || s.Model.FieldForm.Err.Error() != "an issue cannot link to itself" {
			t.Fatalf("%s: form %+v, want it open with kata's refusal", chord, s.Model.FieldForm)
		}
		s = feed(t, s, key("escape")).State
		if s.Model.FieldForm.Open {
			t.Fatalf("%s: esc left the form open", chord)
		}
	}
}

func TestAnEmptyLinkReferenceIsRefusedWithoutASend(t *testing.T) {
	out := feed(t, onAlphaDetail(t), key("b"), key("enter"))
	if len(out.Cmds) != 0 || out.State.Model.FieldForm.Err == nil {
		t.Fatalf("cmds %#v err %v", out.Cmds, out.State.Model.FieldForm.Err)
	}
}
