package closetranscript

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecideIsOnOnlyWhenKatasConfigAndEnvironmentLeaveTheChoiceOpen(t *testing.T) {
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
			if got := Decide(environ, false); got != c.want {
				t.Errorf("Decide = %v, want %v", got, c.want)
			}
		})
	}
}

func TestDecideReadsTheDefaultHomeWithoutKataHome(t *testing.T) {
	user := t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("USERPROFILE", user)
	if !Decide(nil, false) {
		t.Error("no ~/.kata/config.toml: want on")
	}
	if err := os.MkdirAll(filepath.Join(user, ".kata"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(user, ".kata", "config.toml"), []byte("[close.transcript]\nenabled = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if Decide(nil, false) {
		t.Error("enabled = false in ~/.kata: want off")
	}
}
