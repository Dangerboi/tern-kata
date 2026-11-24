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
		{"No lane's empty key", map[string]any{"": []any{"a"}}, model.LaneOrders{"": {"a"}}, ""},
		{"Done is no Lane label", map[string]any{" done": []any{"a"}}, nil, `" done"`},
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

func TestApplyPutsAChangeOnTheStoredOrder(t *testing.T) {
	stored := model.LaneOrders{"triage": {"x", "c", "a"}, "review": {"r"}}
	for _, tt := range []struct {
		name   string
		change model.OrderChange
		want   []string
	}{
		{"shown cards the order does not name go first", model.OrderChange{Shown: []string{"b", "d", "c"}}, []string{"b", "d", "x", "c", "a"}},
		{"a shown card named twice is put once", model.OrderChange{Shown: []string{"b", "b"}}, []string{"b", "x", "c", "a"}},
		{"remove takes uids out", model.OrderChange{Remove: []string{"x", "a", "unknown"}}, []string{"c"}},
		{"remove takes out what shown names", model.OrderChange{Shown: []string{"b"}, Remove: []string{"b"}}, []string{"x", "c", "a"}},
		{"move before the anchor", model.OrderChange{Move: "a", Anchor: "x"}, []string{"a", "x", "c"}},
		{"move after the anchor", model.OrderChange{Move: "x", Anchor: "c", After: true}, []string{"c", "x", "a"}},
		{"move after the last card", model.OrderChange{Move: "x", Anchor: "a", After: true}, []string{"c", "a", "x"}},
		{"move with no anchor goes last", model.OrderChange{Move: "x"}, []string{"c", "a", "x"}},
		{"move to an unknown anchor goes last", model.OrderChange{Move: "x", Anchor: "nowhere", After: true}, []string{"c", "a", "x"}},
		{"move of a card the order lacks adds it", model.OrderChange{Move: "n", Anchor: "c"}, []string{"x", "n", "c", "a"}},
		{"move onto itself goes last", model.OrderChange{Move: "c", Anchor: "c"}, []string{"x", "a", "c"}},
		{"shown, then remove, then move", model.OrderChange{Shown: []string{"b", "a"}, Remove: []string{"x"}, Move: "a", Anchor: "b"}, []string{"a", "b", "c"}},
		{"reset drops the Lane and ignores the rest", model.OrderChange{Reset: true, Shown: []string{"b"}, Move: "a"}, nil},
		{"removing every card drops the Lane", model.OrderChange{Remove: []string{"x", "c", "a"}}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := stored.Apply("triage", tt.change)
			if !reflect.DeepEqual(got["triage"], tt.want) {
				t.Errorf("Lane order %v, want %v", got["triage"], tt.want)
			}
			if !reflect.DeepEqual(got["review"], []string{"r"}) {
				t.Errorf("another Lane's order changed: %v", got)
			}
			if !reflect.DeepEqual(stored, model.LaneOrders{"triage": {"x", "c", "a"}, "review": {"r"}}) {
				t.Errorf("Apply wrote the orders it was given: %v", stored)
			}
		})
	}
	seeded := model.LaneOrders(nil).Apply("", model.OrderChange{Shown: []string{"b", "a"}, Move: "a", Anchor: "b"})
	if want := (model.LaneOrders{"": {"a", "b"}}); !reflect.DeepEqual(seeded, want) {
		t.Errorf("a first order seeded from the shown cards: %v, want %v", seeded, want)
	}
}

func TestAnOnlyOrderedChangeDoesNothingToALaneWithoutAnOrder(t *testing.T) {
	change := model.OrderChange{Shown: []string{"b", "a"}, Move: "a", Anchor: "b", OnlyOrdered: true}
	stored := model.LaneOrders{"review": {"r"}}
	if got := stored.Apply("triage", change); !reflect.DeepEqual(got, stored) {
		t.Errorf("a Lane with no order got %v, want nothing changed from %v", got, stored)
	}
	if got := model.LaneOrders(nil).Apply("triage", change); len(got) != 0 {
		t.Errorf("no orders became %v", got)
	}
	stored = model.LaneOrders{"triage": {"x", "b"}}
	if got, want := stored.Apply("triage", change), (model.LaneOrders{"triage": {"x", "a", "b"}}); !reflect.DeepEqual(got, want) {
		t.Errorf("a Lane with an order became %v, want %v", got, want)
	}
}
