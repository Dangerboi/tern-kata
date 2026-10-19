package driver

import (
	"reflect"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestRouteKeepsEveryDaemonCommandAndTheEditorRoundTrip(t *testing.T) {
	load := model.LoadDetail{Workspace: "/repo", UID: "a"}
	edit := model.OpenDraftEditor{UID: "a", Text: "text"}
	other := model.LoadDetail{Workspace: "/repo", UID: "b"}
	var out Output
	out.route(model.Model{}, []model.Cmd{load, edit, other})
	if !reflect.DeepEqual(out.Cmds, []model.Cmd{load, other}) {
		t.Errorf("daemon commands = %#v", out.Cmds)
	}
	if out.Editor == nil || *out.Editor != edit {
		t.Errorf("editor = %#v", out.Editor)
	}
}
