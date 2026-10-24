package data

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testUID = "01HZX3K8M2N4P6Q8R0S2T4V6W8"

func TestReadCatalog(t *testing.T) {
	home := t.TempDir()
	tests := []struct {
		name    string
		file    string // empty: no file
		want    Catalog
		wantErr string
	}{
		{name: "no file", want: Catalog{ConfirmQuit: true}},
		{name: "empty file", file: "# nothing\n", want: Catalog{ConfirmQuit: true}},
		{
			name: "active daemon with remote and local entries in file order",
			file: `active_daemon = " work "
[[daemon]]
name = "work"
url = "https://kata.example.com"
token_env = "WORK_TOKEN"
allow_insecure = true
[[daemon]]
name = "mine"
local = true
home = "` + filepath.ToSlash(filepath.Join(home, "spoke")) + `"
instance_uid = "` + testUID + `"
[[daemon]]
name = "plain"
local = true
`,
			want: Catalog{Active: "work", ConfirmQuit: true, Daemons: []CatalogEntry{
				{Name: "work", URL: "https://kata.example.com", TokenEnv: "WORK_TOKEN", AllowInsecure: true},
				{Name: "mine", Local: true, Home: filepath.Join(home, "spoke"), InstanceUID: testUID},
				{Name: "plain", Local: true},
			}},
		},
		{name: "confirm_quit false", file: "[tui]\nconfirm_quit = false\n", want: Catalog{}},
		{name: "confirm_quit true", file: "[tui]\nconfirm_quit = true\nmouse = true\n", want: Catalog{ConfirmQuit: true}},
		{name: "active daemon not in catalog", file: `active_daemon = "x"`, wantErr: `active_daemon "x" is not in daemon catalog`},
		{name: "duplicate name", file: "[[daemon]]\nname='a'\nlocal=true\n[[daemon]]\nname='a'\nlocal=true\n", wantErr: `duplicate daemon name "a"`},
		{name: "local and url", file: "[[daemon]]\nname='a'\nlocal=true\nurl='http://x'\n", wantErr: "exactly one of local or url"},
		{name: "token and token_env", file: "[[daemon]]\nname='a'\nurl='http://x'\ntoken='t'\ntoken_env='E'\n", wantErr: "mutually exclusive"},
		{name: "home without uid", file: "[[daemon]]\nname='a'\nlocal=true\nhome='/x'\n", wantErr: "requires both home and instance_uid"},
		{name: "relative home", file: "[[daemon]]\nname='a'\nlocal=true\nhome='x'\ninstance_uid='" + testUID + "'\n", wantErr: "absolute path"},
		{name: "bad uid", file: "[[daemon]]\nname='a'\nlocal=true\nhome='/x'\ninstance_uid='no'\n", wantErr: "valid ULID"},
		{name: "unparsable", file: "[[daemon", wantErr: "parse " + filepath.Join(home, "config.toml")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KATA_HOME", home)
			path := filepath.Join(home, "config.toml")
			os.Remove(path)
			if tt.file != "" {
				if err := os.WriteFile(path, []byte(tt.file), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := ReadCatalog()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), path) {
					t.Fatalf("err = %v, want it to name %s and contain %q", err, path, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Active != tt.want.Active || got.ConfirmQuit != tt.want.ConfirmQuit || len(got.Daemons) != len(tt.want.Daemons) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			for i, e := range tt.want.Daemons {
				if got.Daemons[i] != e {
					t.Errorf("daemon %d = %+v, want %+v", i, got.Daemons[i], e)
				}
			}
		})
	}
}

func TestReadCatalogUnreadableFileNamesPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KATA_HOME", home)
	path := filepath.Join(home, "config.toml")
	if err := os.Mkdir(path, 0o700); err != nil { // a directory cannot be read as a file
		t.Fatal(err)
	}
	if _, err := ReadCatalog(); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("err = %v, want it to name %s", err, path)
	}
}

func TestReadCatalogDefaultsToDotKataUnderHome(t *testing.T) {
	user := t.TempDir()
	t.Setenv("KATA_HOME", "")
	t.Setenv("HOME", user)
	t.Setenv("USERPROFILE", user)
	dir := filepath.Join(user, ".kata")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("active_daemon='a'\n[[daemon]]\nname='a'\nlocal=true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadCatalog()
	if err != nil || got.Active != "a" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestCatalogEntryToken(t *testing.T) {
	t.Setenv("SET_TOKEN", " secret ")
	t.Setenv("EMPTY_TOKEN", "")
	e := CatalogEntry{Name: "w", URL: "https://x", TokenEnv: "SET_TOKEN"}
	if tok, err := e.Token(); err != nil || tok != "secret" {
		t.Fatalf("token = %q, %v", tok, err)
	}
	e.TokenEnv = "EMPTY_TOKEN"
	if _, err := e.Token(); err == nil || !strings.Contains(err.Error(), `"EMPTY_TOKEN" is unset or empty`) {
		t.Fatalf("err = %v", err)
	}
	if tok, err := (CatalogEntry{Name: "n", URL: "https://x"}).Token(); err != nil || tok != "" {
		t.Fatalf("no token_env: %q, %v", tok, err)
	}
}
