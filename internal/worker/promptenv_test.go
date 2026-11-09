package worker

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPromptEnv(t *testing.T) {
	cases := []struct {
		name            string
		environ         []string
		caseInsensitive bool
		secrets         map[string]string
		hidden          map[string]bool
		want            []string
	}{
		{
			name:    "routing and credential names pass through",
			environ: []string{"PATH=/bin", "KATA_HOME=/k", "KATA_SERVER=http://s", "KATA_DAEMON=d", "KATA_AUTH_TOKEN=t", "KATA_AUTH_TOKEN_FILE=/f", "KATA_TRUST_PRIVATE_NETWORK=1", "KATA_ALLOW_INSECURE=1"},
			want:    []string{"PATH=/bin", "KATA_HOME=/k", "KATA_SERVER=http://s", "KATA_DAEMON=d", "KATA_AUTH_TOKEN=t", "KATA_AUTH_TOKEN_FILE=/f", "KATA_TRUST_PRIVATE_NETWORK=1", "KATA_ALLOW_INSECURE=1", "KATA_AUTHOR=ann"},
		},
		{
			name:    "plugin-steering names are dropped",
			environ: []string{"KATA_REF=abcd", "KATA_TEAMMATE=x", "KATA_INBOX_USER=u", "KATA_DEFAULT_PROJECT=p", "KATA_SESSION_ID=s", "KATA_OMP_FOO=1", "KATA_PI_FOO=1", "HOME=/h"},
			want:    []string{"HOME=/h", "KATA_AUTHOR=ann"},
		},
		{
			name:    "an unknown KATA_ name and TERN_KATA_ internals are dropped",
			environ: []string{"KATA_SOMETHING_NEW=1", "TERN_KATA_SUPERVISOR=tern", "KEEP=1"},
			want:    []string{"KEEP=1", "KATA_AUTHOR=ann"},
		},
		{
			name:    "an inherited KATA_AUTHOR is replaced by the actor's",
			environ: []string{"KATA_AUTHOR=other", "KEEP=1"},
			want:    []string{"KEEP=1", "KATA_AUTHOR=ann"},
		},
		{
			name:    "secrets are present",
			environ: []string{"KEEP=1"},
			secrets: map[string]string{"API_KEY": "v"},
			want:    []string{"KEEP=1", "KATA_AUTHOR=ann", "API_KEY=v"},
		},
		{
			name:    "a variable a secret source reads is dropped, and the job's own secret is added back",
			environ: []string{"KEEP=1", "GITHUB_TOKEN=gh", "OTHER=2"},
			secrets: map[string]string{"GITHUB_TOKEN": "gh"},
			hidden:  map[string]bool{"GITHUB_TOKEN": true},
			want:    []string{"KEEP=1", "OTHER=2", "KATA_AUTHOR=ann", "GITHUB_TOKEN=gh"},
		},
		{
			name:            "Windows compares a source's variable case-insensitively",
			environ:         []string{"Github_Token=gh", "Path=C:\\bin"},
			hidden:          map[string]bool{"GITHUB_TOKEN": true},
			caseInsensitive: true,
			want:            []string{"Path=C:\\bin", "KATA_AUTHOR=ann"},
		},
		{
			name:    "names are case-sensitive off Windows",
			environ: []string{"kata_ref=abcd", "Kata_Home=/k"},
			want:    []string{"kata_ref=abcd", "Kata_Home=/k", "KATA_AUTHOR=ann"},
		},
		{
			name:            "Windows names compare case-insensitively",
			environ:         []string{"Kata_Ref=abcd", "kata_author=other", "Tern_Kata_Run=1", "Kata_Home=C:\\k", "kata_auth_token=t", "Path=C:\\bin", "kata_new=1"},
			caseInsensitive: true,
			want:            []string{"Kata_Home=C:\\k", "kata_auth_token=t", "Path=C:\\bin", "KATA_AUTHOR=ann"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := promptEnv(c.environ, "ann", c.secrets, c.hidden, c.caseInsensitive)
			if !slices.Equal(got, c.want) {
				t.Errorf("promptEnv = %q, want %q", got, c.want)
			}
		})
	}
}

func TestChildEnvDropsInheritedIssueBinding(t *testing.T) {
	t.Setenv("KATA_REF", "abcd")
	t.Setenv("KATA_HOME", "/k")
	for _, kv := range childEnv("ann", nil, nil) {
		if strings.HasPrefix(kv, "KATA_REF=") {
			t.Fatalf("childEnv kept %q", kv)
		}
	}
	if !slices.Contains(childEnv("ann", nil, nil), "KATA_HOME=/k") {
		t.Error("childEnv dropped KATA_HOME")
	}
}

func TestChildEnvCarriesTheCloseTranscriptVariableExactlyWhenKatasConfigLeavesTheChoice(t *testing.T) {
	for _, c := range []struct {
		name, config, switchEnv string
		want                    bool
	}{
		{name: "no key", config: "[close.transcript]\nagentsview_url = \"http://x\"\n", want: true},
		{name: "no file", want: true},
		{name: "enabled = false", config: "[close.transcript]\nenabled = false\n"},
		{name: "enabled = true", config: "[close.transcript]\nenabled = true\n"},
		{name: "switch false in the environment", switchEnv: "false"},
		{name: "switch true in the environment", switchEnv: "true"},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("KATA_HOME", home)
			t.Setenv("TERN_KATA_CLOSE_TRANSCRIPT", "inherited")
			if c.switchEnv != "" {
				t.Setenv("KATA_CLOSE_TRANSCRIPT_ENABLED", c.switchEnv)
			}
			if c.config != "" {
				if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(c.config), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got := slices.Contains(childEnv("ann", nil, nil), "TERN_KATA_CLOSE_TRANSCRIPT=on")
			if got != c.want {
				t.Errorf("variable on = %v, want %v", got, c.want)
			}
			if c.switchEnv != "" && !slices.Contains(childEnv("ann", nil, nil), "KATA_CLOSE_TRANSCRIPT_ENABLED="+c.switchEnv) {
				t.Errorf("the user's KATA_CLOSE_TRANSCRIPT_ENABLED=%s did not reach the prompt step", c.switchEnv)
			}
			for _, kv := range childEnv("ann", nil, nil) {
				if strings.HasPrefix(kv, "TERN_KATA_CLOSE_TRANSCRIPT=") && kv != "TERN_KATA_CLOSE_TRANSCRIPT=on" {
					t.Errorf("kept %q", kv)
				}
			}
		})
	}
}

func TestCommandEnvDropsReservedNamesByTheirCase(t *testing.T) {
	environ := []string{"PATH=/bin", "kata_auth_token=x", "Tern_Kata_Run=old", "KATA_HOME=/k"}
	j := &Journal{RunUID: "r1", ProjectName: "demo"}
	secrets := map[string]string{"TOKEN": "s"}

	got := commandEnv(environ, j, "/run", secrets, nil, true)
	want := []string{"PATH=/bin", "TOKEN=s", "TERN_KATA_RUN=r1", "TERN_KATA_RUN_DIR=/run", "KATA_PROJECT=demo"}
	if !slices.Equal(got, want) {
		t.Fatalf("case-insensitive: got %q, want %q", got, want)
	}

	got = commandEnv(environ, j, "/run", secrets, nil, false)
	want = []string{"PATH=/bin", "kata_auth_token=x", "Tern_Kata_Run=old", "TOKEN=s", "TERN_KATA_RUN=r1", "TERN_KATA_RUN_DIR=/run", "KATA_PROJECT=demo"}
	if !slices.Equal(got, want) {
		t.Fatalf("case-sensitive: got %q, want %q", got, want)
	}
}
