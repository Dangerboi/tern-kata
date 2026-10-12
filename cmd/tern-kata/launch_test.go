package main

import (
	"reflect"
	"slices"
	"testing"
)

func lookupFrom(env map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}
}

func TestPlanLaunch(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		env          map[string]string
		issueRef     string
		fallbackArgs []string
		err          string
	}{
		{name: "no arguments", fallbackArgs: []string{}},
		{name: "issue separate value", args: []string{"--issue", "proj/ab12"}, issueRef: "proj/ab12", fallbackArgs: []string{"--project", "proj", "proj#ab12"}},
		{name: "issue equals value", args: []string{"--issue=proj/ab12"}, issueRef: "proj/ab12", fallbackArgs: []string{"--project", "proj", "proj#ab12"}},
		{name: "issue missing value", args: []string{"--issue"}, err: "--issue requires <project>/<short_id>"},
		{name: "as missing value", args: []string{"--as"}, err: "--as requires an actor"},
		{name: "malformed issue ref", args: []string{"--issue", "noslash"}, err: `Issue reference must be <project>/<short_id>: "noslash"`},
		{name: "empty issue equals value", args: []string{"--issue="}, err: `Issue reference must be <project>/<short_id>: ""`},
		{name: "unknown arguments pass through", args: []string{"--as", "ann", "--foo", "bar", "--issue=p/1", "-x"}, issueRef: "p/1", fallbackArgs: []string{"--foo", "bar", "-x", "--project", "p", "p#1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := PlanLaunch(tt.args, lookupFrom(tt.env))
			if tt.err != "" {
				if err == nil || err.Error() != tt.err {
					t.Fatalf("error = %v, want %q", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if plan.IssueRef != tt.issueRef {
				t.Errorf("IssueRef = %q, want %q", plan.IssueRef, tt.issueRef)
			}
			if !reflect.DeepEqual(plan.FallbackArgs, tt.fallbackArgs) {
				t.Errorf("FallbackArgs = %q, want %q", plan.FallbackArgs, tt.fallbackArgs)
			}
		})
	}
}

func TestPlanLaunchActorPrecedence(t *testing.T) {
	all := map[string]string{"KATA_AUTHOR": "author", "USER": "user", gitUserNameKey: "git"}
	without := func(keys ...string) map[string]string {
		env := map[string]string{}
		for k, v := range all {
			if !slices.Contains(keys, k) {
				env[k] = v
			}
		}
		return env
	}
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"as flag wins", []string{"--as", "flag"}, all, "flag"},
		{"as equals form wins", []string{"--as=flag"}, all, "flag"},
		{"KATA_AUTHOR next", nil, all, "author"},
		{"USER next", nil, without("KATA_AUTHOR"), "user"},
		{"git user.name next", nil, without("KATA_AUTHOR", "USER"), "git"},
		{"anonymous last", nil, nil, "anonymous"},
		{"empty values are unset", []string{"--as="}, map[string]string{"KATA_AUTHOR": "", "USER": "user"}, "user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := PlanLaunch(tt.args, lookupFrom(tt.env))
			if err != nil {
				t.Fatal(err)
			}
			if plan.Actor != tt.want {
				t.Errorf("Actor = %q, want %q", plan.Actor, tt.want)
			}
		})
	}
}

func TestPlanLaunchGitLookupOnlyWhenNeeded(t *testing.T) {
	asked := []string{}
	lookup := func(key string) (string, bool) {
		asked = append(asked, key)
		return "", false
	}
	if _, err := PlanLaunch([]string{"--as", "flag"}, lookup); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 0 {
		t.Errorf("lookups with --as = %q, want none", asked)
	}
}

func TestLaunchPlanFallbackEnv(t *testing.T) {
	environ := []string{"PATH=/bin", "KATA_AUTHOR=old", "HOME=/h"}
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"as replaces existing KATA_AUTHOR", []string{"--as", "ann"}, []string{"PATH=/bin", "HOME=/h", "KATA_AUTHOR=ann"}},
		{"equals form", []string{"--as=ann"}, []string{"PATH=/bin", "HOME=/h", "KATA_AUTHOR=ann"}},
		{"no as leaves environment untouched", nil, environ},
		{"empty as leaves environment untouched", []string{"--as="}, environ},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := PlanLaunch(tt.args, lookupFrom(nil))
			if err != nil {
				t.Fatal(err)
			}
			if got := plan.FallbackEnv(environ); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FallbackEnv = %q, want %q", got, tt.want)
			}
		})
	}
	lower := []string{"kata_author=old", "KATA_AUTHOR=old"}
	plan, _ := PlanLaunch([]string{"--as", "ann"}, lookupFrom(nil))
	if got, want := plan.FallbackEnv(lower), []string{"kata_author=old", "KATA_AUTHOR=ann"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FallbackEnv = %q, want %q", got, want)
	}
}
