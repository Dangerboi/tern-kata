package driver

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
	"github.com/Dangerboi/tern-kata/internal/view"
	"github.com/stencil-hq/tern-sdk/go/tern"
)

func TestAnActionValueReachesTheViewOnlyWhenItIsAString(t *testing.T) {
	act, _, _ := strings.Cut(view.ToggleClick("parent"), "=")
	for _, tt := range []struct {
		name  string
		value any
		want  model.Msg
	}{
		{"string", "parent", model.ActionMsg{Action: model.ToggleExpansion, Value: "parent"}},
		{"number", float64(3), model.ActionMsg{Action: model.ToggleExpansion}},
		{"bool", true, model.ActionMsg{Action: model.ToggleExpansion}},
		{"object", map[string]any{"col": "title"}, model.ActionMsg{Action: model.ToggleExpansion}},
		{"nil", nil, model.ActionMsg{Action: model.ToggleExpansion}},
	} {
		got := decode(&tern.Action{Act: act, Value: tt.value})
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %#v, want %#v", tt.name, got, tt.want)
		}
	}
}
