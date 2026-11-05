// Package docbanksetup is `tern-kata docbank setup`: it points a kata daemon at a Docbank and checks that kata can use it. It is the one
// place that knows kata's [docbank] config section. The remote form (a Docbank someone else runs) and the local form (which also writes Docbank's own
// config) share the same Env and the same applyKata step.
package docbanksetup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/BurntSushi/toml"

	"github.com/Dangerboi/tern-kata/internal/atomicfile"
	"github.com/Dangerboi/tern-kata/internal/data"
)

// DefaultFolder is the vault folder kata uses when [docbank] names none.
const DefaultFolder = "/kata"

// KeyFileName is the key file written beside kata's config.toml when the key comes from standard input.
const KeyFileName = "docbank.key"

// Runner runs the executables the setup drives (kata, and docbank in the local form).
type Runner interface {
	// LookPath finds name on PATH.
	LookPath(name string) (string, error)
	// Run runs name with args, with env (NAME=value) added to the process's environment, and returns its combined output.
	Run(ctx context.Context, env []string, name string, args ...string) ([]byte, error)
}

// ExecRunner runs the real programs, in the process's environment.
type ExecRunner struct{}

func (ExecRunner) LookPath(name string) (string, error) { return exec.LookPath(name) }

func (ExecRunner) Run(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	return cmd.CombinedOutput()
}

// Env is everything the setup touches outside its own logic, so tests give it their own folders, programs and HTTP client.
type Env struct {
	// KataHome is the home of the kata daemon being set up: its config.toml is edited and the key file written beside it.
	KataHome string
	// DocbankHome is Docbank's home, whose config.toml the local form writes; the remote form leaves it alone.
	DocbankHome string
	// ConfigDir is tern-kata's config folder, where the local form keeps its supervision marker.
	ConfigDir string
	// Daemon is the kata catalog name of a local profile; empty for the default daemon. kata's daemon commands take no --daemon, so a
	// profile is restarted with KATA_HOME set to its home.
	Daemon string
	// CommandPrefix is put before the restart command printed when kata is not restarted, for the environment the route set
	// (for instance "KATA_HOME=/x "); it is never used to run anything. A profile's own KATA_HOME replaces it.
	CommandPrefix string
	Runner        Runner
	HTTP          *http.Client
	// Locate finds the kata daemon for the final check, called with KataHome as the workspace so a checkout's pin cannot redirect it.
	Locate data.Locator
	// LookupEnv reads --key-env's variable.
	LookupEnv func(string) (string, bool)
	// Stdin is read for `--key-file -`.
	Stdin io.Reader
	// Confirm asks a yes/no question on the terminal; nil when there is none.
	Confirm func(question string) (bool, error)
	Out     io.Writer
}

// Remote is `docbank setup --url ...`: the flags that point kata at a Docbank someone else runs.
type Remote struct {
	URL string
	// KeyFile is a path, or "-" for standard input; KeyEnv is the name of a variable. Exactly one is set.
	KeyFile, KeyEnv string
	Folder          string // empty means DefaultFolder
	// Yes restarts the kata daemon without asking; NoRestart leaves it alone.
	Yes, NoRestart bool
}

// kataSection is kata's [docbank] section as setup writes and compares it.
type kataSection struct {
	URL        string `toml:"url"`
	APIKey     string `toml:"api_key"`
	APIKeyFile string `toml:"api_key_file"`
	APIKeyEnv  string `toml:"api_key_env"`
	Folder     string `toml:"folder"`
}

type kataConfig struct {
	Docbank *kataSection `toml:"docbank"`
}

// probeDeadline bounds the authenticated read of Docbank; checkDeadline bounds the restart and kata's check.
const (
	probeDeadline = 15 * time.Second
	checkDeadline = 60 * time.Second
)

// SetupRemote points the kata daemon at the Docbank r names. Nothing is written until the URL passes kata's scheme rule, kata is on
// PATH, the key reads, and an authenticated read of Docbank with it succeeds.
func (e Env) SetupRemote(ctx context.Context, r Remote) error {
	base, err := CanonicalURL(r.URL)
	if err != nil {
		return err
	}
	folder, err := normalizeFolder(r.Folder)
	if err != nil {
		return err
	}
	if _, err := e.Runner.LookPath("kata"); err != nil {
		return errors.New("kata is not on PATH; install kata first (https://github.com/kenn-io/kata), nothing was written")
	}
	want := kataSection{URL: base, Folder: folder, APIKeyEnv: r.KeyEnv}
	switch {
	case r.KeyFile == "-":
		want.APIKeyFile = filepath.Join(e.KataHome, KeyFileName)
	case r.KeyFile != "":
		if want.APIKeyFile, err = filepath.Abs(r.KeyFile); err != nil {
			return err
		}
	}
	configPath := filepath.Join(e.KataHome, "config.toml")
	existing, err := readConfig(configPath)
	if err != nil {
		return err
	}
	if have, found, err := existingSection(configPath, existing); err != nil {
		return err
	} else if found {
		if !have.matches(want) {
			return fmt.Errorf("%s already has a [docbank] section that differs from what was asked, so nothing was changed:\n%s", configPath, have.describe())
		}
		_, err := fmt.Fprintf(e.Out, "Docbank at %s is already set up in %s; nothing changed.\n", base, configPath)
		return err
	}

	key, err := e.readKey(r)
	if err != nil {
		return err
	}
	if err := e.probe(ctx, base, key); err != nil {
		return err
	}
	if r.KeyFile == "-" {
		if err := e.writeKeyFile(want.APIKeyFile, key); err != nil {
			return err
		}
	}
	if err := writeSection(configPath, existing, want); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(e.Out, "Wrote the [docbank] section of %s: url %s, folder %s, %s.\n", configPath, base, folder, want.keySource()); err != nil {
		return err
	}
	if r.KeyEnv != "" {
		if _, err := fmt.Fprintf(e.Out, "kata reads $%s from the daemon's own environment: it must be set wherever the kata daemon starts.\n", r.KeyEnv); err != nil {
			return err
		}
	}
	return e.applyKata(ctx, base, r.Yes, r.NoRestart)
}

// applyKata is the step after kata's config.toml says where Docbank is, shared by the remote and local forms: restart the kata daemon
// (asking first unless yes; never when noRestart or when there is no terminal to ask on) and, once restarted, check that it has
// attachments on.
func (e Env) applyKata(ctx context.Context, base string, yes, noRestart bool) error {
	command, prefix, env := "kata daemon restart", e.CommandPrefix, []string(nil)
	if e.Daemon != "" {
		prefix, env = "KATA_HOME="+e.KataHome+" ", []string{"KATA_HOME=" + e.KataHome}
	}
	restart := yes
	if !yes && !noRestart && e.Confirm != nil {
		var err error
		if restart, err = e.Confirm("Restart the kata daemon now? Open Panes and agents reconnect. [y/N] "); err != nil {
			return err
		}
	}
	if noRestart || !restart {
		_, err := fmt.Fprintf(e.Out, "The kata daemon was not restarted, and it reads config.toml only at start, so attachments stay off until you run:\n  %s%s\n", prefix, command)
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, checkDeadline)
	defer cancel()
	if out, err := e.Runner.Run(ctx, env, "kata", "daemon", "restart"); err != nil {
		return fmt.Errorf("`%s` failed: %w\n%s", command, err, strings.TrimSpace(string(out)))
	}
	if _, err := fmt.Fprintf(e.Out, "Restarted the kata daemon (`%s`).\n", command); err != nil {
		return err
	}
	if err := data.CheckAttachments(ctx, e.Locate, e.KataHome); err != nil {
		return fmt.Errorf("the config is written and the kata daemon restarted, but attachments do not work yet: %w", err)
	}
	_, err := fmt.Fprintf(e.Out, "Checked: the kata daemon has attachments on, with Docbank at %s.\n", base)
	return err
}

// CanonicalURL is kata's reading of a Docbank url: http or https with a host, no credentials, query or fragment, no trailing slash.
// Plain http is for this machine only (localhost or a loopback address), as kata itself refuses it.
func CanonicalURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", fmt.Errorf("--url must be an http(s) address such as https://docbank.example, got %q", raw)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("--url must not carry credentials, a query or a fragment")
	}
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		return "", fmt.Errorf("--url %s: HTTPS is required unless Docbank is on this machine (localhost or a loopback address); nothing was written", u.Redacted())
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// normalizeFolder is kata's rule for folder: a clean absolute path; empty means DefaultFolder.
func normalizeFolder(folder string) (string, error) {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		return DefaultFolder, nil
	}
	if !strings.HasPrefix(folder, "/") || path.Clean(folder) != folder || strings.ContainsAny(folder, `\`) || strings.ContainsFunc(folder, unicode.IsControl) {
		return "", fmt.Errorf("--folder %q must be a clean absolute path such as /kata", folder)
	}
	return folder, nil
}

// readKey reads the key from where r names it. It never leaves the process except in the request header and the key file.
func (e Env) readKey(r Remote) (string, error) {
	var key string
	switch {
	case r.KeyFile == "-":
		raw, err := io.ReadAll(io.LimitReader(e.Stdin, 64<<10))
		if err != nil {
			return "", fmt.Errorf("read the key from standard input: %w", err)
		}
		key = string(raw)
	case r.KeyFile != "":
		raw, err := os.ReadFile(r.KeyFile)
		if err != nil {
			return "", fmt.Errorf("read the key file: %w", err)
		}
		key = string(raw)
	default:
		value, _ := e.LookupEnv(r.KeyEnv)
		if strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("--key-env %s: the variable is unset or empty", r.KeyEnv)
		}
		key = value
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("the API key is empty")
	}
	if strings.ContainsFunc(key, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return "", errors.New("the API key must be one line with no spaces")
	}
	return key, nil
}

// probe is the authenticated read of Docbank's API that must succeed before anything is written: it resolves the vault root. A refused
// key and an unanswered request are reported as the Pane reports them.
func (e Env) probe(ctx context.Context, base, key string) error {
	ctx, cancel := context.WithTimeout(ctx, probeDeadline)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/path?path=%2F", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", key)
	resp, err := e.HTTP.Do(req)
	if err != nil {
		return errors.New(data.DocbankUnreachableMessage)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return errors.New(data.DocbankUnauthorizedMessage)
	case resp.StatusCode != http.StatusOK:
		return errors.New(data.DocbankUnreachableMessage)
	}
	return nil
}

// writeKeyFile stores key at path, mode 0600, written atomically; a file that already holds it is left alone.
func (e Env) writeKeyFile(path, key string) error {
	shown := path
	path = resolveLink(path)
	if have, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(have)) == key {
		return nil
	}
	if err := atomicfile.Write(path, []byte(key+"\n"), atomicfile.Options{Mode: 0o600, DirMode: 0o700}); err != nil {
		return fmt.Errorf("write the key file: %w", err)
	}
	_, err := fmt.Fprintf(e.Out, "Wrote the API key to %s (mode 0600).\n", shown)
	return err
}

// readConfig is kata's config.toml as text; a missing file is empty text.
func readConfig(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(raw), nil
}

// existingSection is the [docbank] section text holds, found false when there is none. A file that does not parse is refused, so
// setup never edits a config kata itself could not read.
func existingSection(path, text string) (section kataSection, found bool, err error) {
	var cfg kataConfig
	meta, err := toml.Decode(text, &cfg)
	if err != nil {
		return kataSection{}, false, fmt.Errorf("%s does not parse, so nothing was changed: %w", path, err)
	}
	if !meta.IsDefined("docbank") || cfg.Docbank == nil {
		return kataSection{}, false, nil
	}
	return *cfg.Docbank, true, nil
}

// matches reports whether the section found in the file sets what want sets (the folder defaulting as kata defaults it).
func (have kataSection) matches(want kataSection) bool {
	if have.Folder == "" {
		have.Folder = DefaultFolder
	}
	if have.APIKeyFile != "" {
		have.APIKeyFile = filepath.Clean(have.APIKeyFile)
	}
	if u, err := CanonicalURL(have.URL); err == nil {
		have.URL = u
	}
	return have.APIKey == "" && have == want
}

// describe shows the section as found, with an inline key hidden.
func (s kataSection) describe() string {
	var b strings.Builder
	line := func(name, value string) {
		if value != "" {
			fmt.Fprintf(&b, "  %s = %s\n", name, tomlString(value))
		}
	}
	line("url", s.URL)
	if s.APIKey != "" {
		b.WriteString("  api_key = (set, not shown)\n")
	}
	line("api_key_file", s.APIKeyFile)
	line("api_key_env", s.APIKeyEnv)
	line("folder", s.Folder)
	return strings.TrimRight(b.String(), "\n")
}

func (s kataSection) keySource() string {
	if s.APIKeyFile != "" {
		return "api_key_file " + s.APIKeyFile
	}
	return "api_key_env " + s.APIKeyEnv
}

// text is the section as written to the file.
func (s kataSection) text() string {
	var b strings.Builder
	b.WriteString("[docbank]\n")
	fmt.Fprintf(&b, "url = %s\n", tomlString(s.URL))
	if s.APIKeyFile != "" {
		fmt.Fprintf(&b, "api_key_file = %s\n", tomlString(s.APIKeyFile))
	} else {
		fmt.Fprintf(&b, "api_key_env = %s\n", tomlString(s.APIKeyEnv))
	}
	fmt.Fprintf(&b, "folder = %s\n", tomlString(s.Folder))
	return b.String()
}

// writeSection appends the section to existing and writes the result over path atomically. The new file is parsed before it replaces
// the old one and must hold exactly the section; the text before it is the old file, byte for byte.
func writeSection(path, existing string, want kataSection) error {
	path = resolveLink(path)
	next := existing
	if next != "" {
		if !strings.HasSuffix(next, "\n") {
			next += "\n"
		}
		next += "\n"
	}
	next += want.text()
	mode := fs.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm() // an edit keeps the file's own mode
	}
	return atomicfile.Write(path, []byte(next), atomicfile.Options{Mode: mode, DirMode: 0o700, Check: func(tmp string) error {
		raw, err := os.ReadFile(tmp)
		if err != nil {
			return err
		}
		got, found, err := existingSection(path, string(raw))
		if err != nil {
			return fmt.Errorf("the new %s would not read back, so nothing was written: %w", path, err)
		}
		if !found || !got.matches(want) || !bytes.HasPrefix(raw, []byte(existing)) {
			return fmt.Errorf("the new %s would not read back as asked, so nothing was written", path)
		}
		return nil
	}})
}

// resolveLink is the file path names when path is a symlink that resolves, so an edit replaces the file the link points at and the
// link stays; path itself otherwise.
func resolveLink(path string) string {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		return target
	}
	return path
}

// tomlString is s as a TOML basic string.
func tomlString(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

// AskYesNo prints question to out and reads one line from in; "y" or "yes" in any case is a yes.
func AskYesNo(in *bufio.Reader, out io.Writer, question string) (bool, error) {
	if _, err := io.WriteString(out, question); err != nil {
		return false, err
	}
	line, err := in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
