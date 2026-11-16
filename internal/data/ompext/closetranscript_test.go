package ompext

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCloseTranscriptOnIsOnOnlyWhenKatasConfigAndEnvironmentLeaveTheChoiceOpen(t *testing.T) {
	cases := []struct {
		name    string
		config  *string // nil: no file
		unread  bool
		environ []string
		want    bool
	}{
		{name: "enabled = true", config: new("[close.transcript]\nenabled = true\n")},
		{name: "enabled = false", config: new("[close.transcript]\nenabled = false\n")},
		{name: "no key", config: new("[close.transcript]\nagentsview_url = \"http://x\"\n"), want: true},
		{name: "other sections only", config: new("[tui]\nconfirm_quit = false\n"), want: true},
		{name: "no file", want: true},
		{name: "unreadable file", unread: true},
		{name: "invalid file", config: new("[close.transcript\n")},
		{name: "variable already in the environment", environ: []string{"KATA_CLOSE_TRANSCRIPT_ENABLED=false"}},
		{name: "a blank variable is unset, as kata reads it", environ: []string{"KATA_CLOSE_TRANSCRIPT_ENABLED= "}, want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, "config.toml")
			switch {
			case c.unread:
				if err := os.Mkdir(path, 0o700); err != nil { // reading a directory fails with something other than not-exist
					t.Fatal(err)
				}
			case c.config != nil:
				if err := os.WriteFile(path, []byte(*c.config), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			environ := append([]string{"KATA_HOME=" + home}, c.environ...)
			if got := CloseTranscriptOn(environ, false); got != c.want {
				t.Errorf("CloseTranscriptOn = %v, want %v", got, c.want)
			}
		})
	}
}

func TestCloseTranscriptOnReadsTheDefaultHomeWithoutKataHome(t *testing.T) {
	user := t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("USERPROFILE", user)
	if !CloseTranscriptOn(nil, false) {
		t.Error("no ~/.kata/config.toml: want on")
	}
	if err := os.MkdirAll(filepath.Join(user, ".kata"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(user, ".kata", "config.toml"), []byte("[close.transcript]\nenabled = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if CloseTranscriptOn(nil, false) {
		t.Error("enabled = false in ~/.kata: want off")
	}
}

func TestWithCloseTranscriptDecidesTheVariableAfresh(t *testing.T) {
	home := t.TempDir() // no config.toml: on
	on := CloseTranscriptEnv + "=" + CloseTranscriptOnValue
	cases := []struct {
		name            string
		env, decideFrom []string
		caseInsensitive bool
		want            []string
	}{
		{name: "adds on", env: []string{"A=1"}, want: []string{"A=1", on}},
		{name: "drops an inherited value", env: []string{CloseTranscriptEnv + "=off", "A=1"}, want: []string{"A=1", on}},
		{name: "off drops it and adds nothing", env: []string{CloseTranscriptEnv + "=on", "A=1"}, decideFrom: []string{KataSwitch + "=false"}, want: []string{"A=1"}},
		{name: "a lower-case name is the same one on Windows", env: []string{"tern_kata_close_transcript=on"}, decideFrom: []string{"kata_close_transcript_enabled=1"}, caseInsensitive: true, want: nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			decide := append([]string{"KATA_HOME=" + home}, c.decideFrom...)
			got := WithCloseTranscript(c.env, decide, c.caseInsensitive)
			if !slices.Equal(got, c.want) {
				t.Fatalf("env %q, want %q", got, c.want)
			}
		})
	}
}

func TestHasFlagMatchesAFlagWithOrWithoutAValue(t *testing.T) {
	args := []string{"--profile=work", "--no-extensions", "-p", "x"}
	for flag, want := range map[string]bool{"--profile": true, "--no-extensions": true, "-p": true, "--trusted-extension": false, "--no": false} {
		if got := HasFlag(args, flag); got != want {
			t.Errorf("HasFlag(%s) = %v, want %v", flag, got, want)
		}
	}
	if !HasFlag(args, "--trusted-extension", "--profile") {
		t.Error("any of several flags should match")
	}
}
