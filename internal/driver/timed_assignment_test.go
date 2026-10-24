package driver_test

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestTOpensTheDurationFormNamingTheIssue(t *testing.T) {
	s := feed(t, onAlphaDetail(t), key("t")).State
	if !s.Model.FieldForm.Open || s.Model.FieldForm.Action != model.TimedAssignAction || s.Model.FieldForm.UID != "alpha" {
		t.Fatalf("t opened %+v", s.Model.FieldForm)
	}
	if got, want := s.Model.FieldFormTitle(), "Assignment duration for #aaaa (1m..24h, e.g. 1h)"; got != want {
		t.Fatalf("title %q, want %q", got, want)
	}
}

func TestAValidDurationSendsOneClaimInSeconds(t *testing.T) {
	for text, seconds := range map[string]string{"1m": "60", "90m": "5400", "24h": "86400", " 1h ": "3600"} {
		s := feed(t, onAlphaDetail(t), key("t")).State
		out := feed(t, s, append(keysOf(text), key("enter"))...)
		want := model.WriteField{Workspace: "/repo", UID: "alpha", Action: model.TimedAssignAction, Value: seconds}
		if !reflect.DeepEqual(out.Cmds, []model.Cmd{want}) {
			t.Fatalf("%q sent %#v, want %#v", text, out.Cmds, want)
		}
	}
}

func TestAnInvalidDurationIsRefusedWithoutASend(t *testing.T) {
	for _, text := range []string{"30s", "59s", "25h", "24h1s", "1.5s", "1m0.5s", "soon", "90", "-1h", ""} {
		s := feed(t, onAlphaDetail(t), key("t")).State
		out := feed(t, s, append(keysOf(text), key("enter"))...)
		if len(out.Cmds) != 0 || !out.State.Model.FieldForm.Open || out.State.Model.FieldForm.Err == nil {
			t.Fatalf("%q: cmds %#v form %+v", text, out.Cmds, out.State.Model.FieldForm)
		}
		if text != "" && out.State.Model.FieldForm.Err.Error() != "duration must be a whole number of seconds from 1m through 24h" {
			t.Fatalf("%q: error %q", text, out.State.Model.FieldForm.Err)
		}
	}
}
