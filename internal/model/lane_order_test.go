package model_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Dangerboi/tern-kata/internal/model"
)

func TestParseLaneOrders(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value any
		want  model.LaneOrders
		err   string
	}{
		{"no value", nil, nil, ""},
		{"a Lane's uids", map[string]any{"triage": []any{"b", "a"}}, model.LaneOrders{"triage": {"b", "a"}}, ""},
		{"a repeated uid keeps its first place", map[string]any{"triage": []any{"b", "a", "b"}}, model.LaneOrders{"triage": {"b", "a"}}, ""},
		{"an empty list drops the Lane", map[string]any{"triage": []any{}, "review": []any{"x"}}, model.LaneOrders{"review": {"x"}}, ""},
		{"not an object", "triage", nil, "object"},
		{"a label CheckLabel would change", map[string]any{"In Review": []any{"a"}}, nil, `"In Review"`},
		{"a list that is not a list", map[string]any{"triage": "a"}, nil, "triage"},
		{"an empty uid", map[string]any{"triage": []any{""}}, nil, "uid"},
		{"a uid that is a number", map[string]any{"triage": []any{3.0}}, nil, "uid"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := model.ParseLaneOrders(tt.value)
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("error %v, want one naming %q", err, tt.err)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, %v, want %v", got, err, tt.want)
			}
		})
	}
}
