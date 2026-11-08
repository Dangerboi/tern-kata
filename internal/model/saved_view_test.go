package model

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckViewName(t *testing.T) {
	others := []SavedView{{Name: "Mine"}, {Name: "Bugs"}}
	long := strings.Repeat("é", MaxViewNameLen)
	tests := []struct {
		name, in, self, want, err string
	}{
		{"trimmed", "  Work  ", "", "Work", ""},
		{"empty", "   ", "", "", "Name is required"},
		{"64 characters", long, "", long, ""},
		{"65 characters", long + "x", "", "", "Name is longer than 64 characters"},
		{"duplicate ignoring case", "mINE", "", "", "already used by another view"},
		{"own name on edit", "MINE", "mine", "MINE", ""},
		{"another's name on edit", "bugs", "mine", "", "already used by another view"},
		{"title", "today", "", "", "fixed Collection Today"},
		{"title with space", "ALL open", "", "", "fixed Collection All Open"},
		{"key", "all-open", "", "", "fixed Collection All Open"},
		{"key upper", "Deadlines", "", "", "fixed Collection Deadlines"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CheckViewName(tt.in, others, tt.self)
			if tt.err == "" {
				if err != nil || got != tt.want {
					t.Fatalf("got %q, %v; want %q", got, err, tt.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.err) || !strings.HasPrefix(err.Error(), "Name") {
				t.Fatalf("got %v; want a Name error containing %q", err, tt.err)
			}
		})
	}
}

func TestCheckViewFilters(t *testing.T) {
	got, err := CheckViewFilters(Filters{Status: "open", Owner: " ann ", Label: " bug,ui ", Relationship: "blocked_by", Search: " x "})
	if err != nil || got != (Filters{Status: "open", Owner: "ann", Label: "bug,ui", Relationship: "blocked_by", Search: "x"}) {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := CheckViewFilters(Filters{Status: "done"}); err == nil || !strings.HasPrefix(err.Error(), "Status") {
		t.Fatalf("status: %v", err)
	}
	if _, err := CheckViewFilters(Filters{Relationship: "friend"}); err == nil || !strings.HasPrefix(err.Error(), "Relationship") {
		t.Fatalf("relationship: %v", err)
	}
}

func TestCheckSavedView(t *testing.T) {
	ok := SavedView{Name: " A ", Base: Collection{View: "all-open"}, Filters: Filters{Status: "open"}}
	got, err := CheckSavedView(ok, nil, "")
	if err != nil || got.Name != "A" || got.Base.Title != "All Open" {
		t.Fatalf("got %+v, %v", got, err)
	}
	project := SavedView{Name: "P", Base: ProjectCollection("U1", "Work")}
	if got, err := CheckSavedView(project, nil, ""); err != nil || got.Base.ProjectUID != "U1" {
		t.Fatalf("project base: %+v, %v", got, err)
	}
	if _, err := CheckSavedView(SavedView{Name: "P", Base: ProjectCollection("", "Work")}, nil, ""); err == nil || !strings.HasPrefix(err.Error(), "Base") {
		t.Fatalf("project without uid: %v", err)
	}
	if _, err := CheckSavedView(SavedView{Name: "P", Base: Collection{View: "results"}}, nil, ""); err == nil || !strings.HasPrefix(err.Error(), "Base") {
		t.Fatalf("bad base: %v", err)
	}
	if _, err := CheckSavedView(SavedView{Name: "", Base: Collection{View: "today"}}, nil, ""); err == nil || !strings.HasPrefix(err.Error(), "Name") {
		t.Fatalf("name first: %v", err)
	}
}

func TestResolveViewName(t *testing.T) {
	views := []SavedView{{Name: "Mine"}}
	for _, name := range []string{"today", "TODAY", "All Open", "all-open"} {
		if ref, ok := ResolveViewName(name, views); !ok || ref.Collection == nil || ref.View != nil {
			t.Errorf("%q: %+v %v", name, ref, ok)
		}
	}
	if ref, ok := ResolveViewName(" mine ", views); !ok || ref.View == nil || ref.View.Name != "Mine" {
		t.Errorf("saved view: %+v %v", ref, ok)
	}
	if _, ok := ResolveViewName("nope", views); ok {
		t.Error("unknown name resolved")
	}
}

func TestFiltersForResolvesMe(t *testing.T) {
	v := SavedView{Filters: Filters{Owner: "@me", Label: "x"}}
	if got := v.FiltersFor("ann"); got.Owner != "ann" || got.Label != "x" {
		t.Fatalf("got %+v", got)
	}
	if v.Filters.Owner != "@me" {
		t.Fatal("the view was changed")
	}
	if got := (SavedView{Filters: Filters{Owner: "bob"}}).FiltersFor("ann"); got.Owner != "bob" {
		t.Fatalf("got %+v", got)
	}
	if got := (SavedView{Filters: Filters{Owner: "me"}}).FiltersFor("ann"); got.Owner != "me" {
		t.Fatalf("plain me is not a macro: %+v", got)
	}
}

func TestSavedViewRefusalsCarryTheirField(t *testing.T) {
	good := SavedView{Name: "A", Base: Collection{View: "today"}}
	for name, tt := range map[string]struct {
		view SavedView
		want Field
	}{
		"name":         {SavedView{Name: "", Base: good.Base}, ViewName},
		"base":         {SavedView{Name: "A", Base: Collection{View: "nowhere"}}, ViewBase},
		"project base": {SavedView{Name: "A", Base: ProjectCollection("", "x")}, ViewBase},
		"status":       {SavedView{Name: "A", Base: good.Base, Filters: Filters{Status: "done"}}, FilterStatus},
		"relationship": {SavedView{Name: "A", Base: good.Base, Filters: Filters{Relationship: "friend"}}, FilterRelationship},
	} {
		_, err := CheckSavedView(tt.view, nil, "")
		var refusal ViewFieldError
		if !errors.As(err, &refusal) || refusal.Field != tt.want {
			t.Errorf("%s: %v carries field %q, want %q", name, err, refusal.Field, tt.want)
		}
	}
}

func TestCheckViewNameRefusesControlCharacters(t *testing.T) {
	for _, in := range []string{"a\nb", "a\tb", "a\x00b", "a\rb"} {
		_, err := CheckViewName(in, nil, "")
		if err == nil || !strings.HasPrefix(err.Error(), "Name") || !strings.Contains(err.Error(), "control characters") {
			t.Errorf("CheckViewName(%q) = %v; want a Name error about control characters", in, err)
		}
	}
}
