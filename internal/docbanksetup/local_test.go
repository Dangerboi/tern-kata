package docbanksetup

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStatusNamesAnyLoopbackSpellingOfTheAddress(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   bool
	}{
		{"running, listening on 127.0.0.1:8080", true},
		{"running, listening on [::1]:8080", true},
		{"running, listening on localhost:8080", true},
		{"running, listening on [::1]:80801", false},
		{"running, listening on [::1]:9090", false},
	} {
		if got := statusNames(tc.status, "127.0.0.1:8080"); got != tc.want {
			t.Errorf("statusNames(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestWriteServerDropsGroupAndOtherBitsAndKeepsTheRest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no permission bits")
	}
	port := 8080
	for _, tc := range []struct{ from, want os.FileMode }{{0o644, 0o600}, {0o664, 0o600}, {0o755, 0o700}, {0o640, 0o600}, {0o600, 0o600}, {0o400, 0o400}} {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte("[vault]\nname = \"x\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, tc.from); err != nil {
			t.Fatal(err)
		}
		was, tightened, err := writeServer(path, "[vault]\nname = \"x\"\n", docbankServer{BindAddr: "127.0.0.1", APIPort: &port, APIKey: "k"})
		if err != nil {
			t.Fatalf("from %04o: %v", tc.from, err)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != tc.want {
			t.Errorf("from %04o: mode %v, %v; want %04o", tc.from, info, err, tc.want)
		}
		if wantTight := tc.from&0o077 != 0; tightened != wantTight || (tightened && was != tc.from) {
			t.Errorf("from %04o: tightened %v, was %04o", tc.from, tightened, was)
		}
	}
}

func TestWriteSectionWithoutASecretKeepsTheFilesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no permission bits")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeSection(path, "", kataSection{URL: "http://127.0.0.1:1", APIKeyFile: "/k", Folder: "f"}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("mode %v, %v; want 0644", info, err)
	}
}

func TestRestartHintFitsTheShell(t *testing.T) {
	env := [][2]string{{"KATA_HOME", `C:\Users\o'brien\.kata`}, {"KATA_DB", "it's a db"}}
	for _, tc := range []struct{ goos, want string }{
		{"windows", `$env:KATA_HOME='C:\Users\o''brien\.kata'; $env:KATA_DB='it''s a db'; kata daemon restart`},
		{"linux", `KATA_HOME='C:\Users\o'\''brien\.kata' KATA_DB='it'\''s a db' kata daemon restart`},
		{"darwin", `KATA_HOME='C:\Users\o'\''brien\.kata' KATA_DB='it'\''s a db' kata daemon restart`},
	} {
		if got := RestartHint(tc.goos, env, "kata daemon restart"); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.goos, got, tc.want)
		}
	}
	if got := RestartHint("windows", nil, "kata daemon restart"); got != "kata daemon restart" {
		t.Errorf("no env: %q", got)
	}
	if got := RestartHint("linux", [][2]string{{"KATA_HOME", "/h"}}, "kata daemon restart"); got != "KATA_HOME=/h kata daemon restart" {
		t.Errorf("plain: %q", got)
	}
}
