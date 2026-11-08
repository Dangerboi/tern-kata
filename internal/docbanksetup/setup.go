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
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/BurntSushi/toml"

	"github.com/Dangerboi/tern-kata/internal/atomicfile"
	"github.com/Dangerboi/tern-kata/internal/data"
	"github.com/Dangerboi/tern-kata/internal/model"
	"go.kenn.io/kit/consolewindow"
)

// DefaultFolder is the vault folder kata uses when [docbank] names none.
const DefaultFolder = "/kata"

// KeyFileName is the key file written beside kata's config.toml when the key comes from standard input.
const KeyFileName = "docbank.key"

type Runner interface {
	LookPath(name string) (string, error)
	Run(ctx context.Context, env []string, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) LookPath(name string) (string, error) { return exec.LookPath(name) }

func (ExecRunner) Run(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	consolewindow.Hide(cmd)
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
	// CommandEnv is the environment the route set, as name/value pairs, written before the restart command printed when kata is not
	// restarted; it is never used to run anything. A profile's own KATA_HOME replaces it.
	CommandEnv [][2]string
	Runner     Runner
	HTTP       *http.Client
	// Locate finds the kata daemon for the final check, called with KataHome as the workspace so a checkout's pin cannot redirect it.
	Locate data.Locator
	// DefaultKataHome is the home of the default kata daemon (KATA_HOME or ~/.kata), the only daemon whose Docbank the plugin keeps
	// running; empty when it cannot be resolved (then no daemon is supervised).
	DefaultKataHome string
	// ProbeTimeout bounds the authenticated read of Docbank; zero means the default.
	ProbeTimeout time.Duration
	LookupEnv    func(string) (string, bool)
	Stdin        io.Reader
	Confirm      func(question string) (bool, error)
	Out          io.Writer
}

// Common is what both forms take besides where Docbank is.
type Common struct {
	Folder         string // empty means DefaultFolder
	Yes, NoRestart bool
}

// Remote is `docbank setup --url ...`: the flags that point kata at a Docbank someone else runs.
type Remote struct {
	Common
	URL string
	// KeyFile is a path, or "-" for standard input; KeyEnv is the name of a variable. Exactly one is set.
	KeyFile, KeyEnv string
}

type kataSection struct {
	URL        string `toml:"url"`
	APIKey     string `toml:"api_key"`
	APIKeyFile string `toml:"api_key_file"`
	APIKeyEnv  string `toml:"api_key_env"`
	Folder     string `toml:"folder"`
}

// defaultProbeTimeout bounds the authenticated read of Docbank; checkDeadline bounds the restart and kata's check.
const (
	defaultProbeTimeout = 15 * time.Second
	checkDeadline       = 60 * time.Second
	// probeMaxBody bounds the vault-root answer read; a node is a few hundred bytes.
	probeMaxBody = 1 << 20
	kataMissing  = "kata is not on PATH; install kata first (https://github.com/kenn-io/kata), nothing was written"
)

func (e Env) probeTimeout() time.Duration {
	if e.ProbeTimeout > 0 {
		return e.ProbeTimeout
	}
	return defaultProbeTimeout
}

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
		return errors.New(kataMissing)
	}
	want := kataSection{URL: base, Folder: folder, APIKeyEnv: r.KeyEnv}
	switch {
	case r.KeyFile == "-":
		want.APIKeyFile = filepath.Join(e.KataHome, KeyFileName)
	case r.KeyFile != "":
		if want.APIKeyFile, err = filepath.Abs(r.KeyFile); err != nil {
			return err
		}
		if err := checkKeyFile(want.APIKeyFile); err != nil {
			return err
		}
	}
	configPath := filepath.Join(e.KataHome, "config.toml")
	for _, p := range []string{configPath, want.APIKeyFile} {
		if p == "" {
			continue
		}
		if _, err := resolveLink(p); err != nil {
			return err
		}
	}
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
		return e.alreadySetUp(ctx, r, base, configPath)
	}

	key, err := e.readKey(r)
	if err != nil {
		return err
	}
	if err := e.probe(ctx, base, key); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.KeyFile == "-" {
		if err := e.writeKeyFile(want.APIKeyFile, key); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
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
	return e.applyKata(ctx, r.Common, r.KeyEnv)
}

// applyKata is the step after kata's config.toml says where Docbank is, shared by the remote and local forms: restart the kata daemon
// (asking first unless c.Yes; never when c.NoRestart or when there is no terminal to ask on) and, once restarted, check that it has
// attachments on. keyEnv is --key-env's variable, when there is one.
func (e Env) applyKata(ctx context.Context, c Common, keyEnv string) error {
	command, hintEnv, env := "kata daemon restart", e.CommandEnv, []string(nil)
	if e.Daemon != "" {
		hintEnv, env = [][2]string{{"KATA_HOME", e.KataHome}}, []string{"KATA_HOME=" + e.KataHome}
	}
	restart := c.Yes
	if !c.Yes && !c.NoRestart && e.Confirm != nil {
		var err error
		if restart, err = e.Confirm("Restart the kata daemon now? Open Panes and agents reconnect. [y/N] "); err != nil {
			return err
		}
	}
	if c.NoRestart || !restart {
		_, err := fmt.Fprintf(e.Out, "The kata daemon was not restarted, and it reads config.toml only at start, so attachments stay off until you run:\n  %s\n", RestartHint(runtime.GOOS, hintEnv, command))
		return err
	}
	if err := ctx.Err(); err != nil {
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
	// kata has no read-only call that reaches Docbank (every call that does reads or writes an Attachment), so what is checked is
	// that the daemon loaded the [docbank] section: it has attachments on.
	on, err := data.AttachmentsConfigured(ctx, e.Locate, e.KataHome)
	if err == nil && !on {
		err = errors.New(model.DocbankOffMessage)
	}
	if err != nil {
		return fmt.Errorf("the config is written and the kata daemon restarted, but attachments do not work yet: %w", err)
	}
	if _, err := fmt.Fprintln(e.Out, "Checked: the kata daemon has attachments on."); err != nil {
		return err
	}
	if keyEnv != "" {
		_, err := fmt.Fprintf(e.Out, "The kata daemon reads $%s from its own environment, so the key is not checked here: it must be set wherever the kata daemon starts.\n", keyEnv)
		return err
	}
	return nil
}

// alreadySetUp is a rerun whose [docbank] section already says what was asked. The running kata daemon is still asked, since a restart
// that was skipped or failed leaves attachments off: it is only "already set up" when attachments are on, and otherwise the restart is
// offered as after a write.
func (e Env) alreadySetUp(ctx context.Context, r Remote, base, configPath string) error {
	check, cancel := context.WithTimeout(ctx, checkDeadline)
	on, err := data.AttachmentsConfigured(check, e.Locate, e.KataHome)
	cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err == nil && on {
		_, err := fmt.Fprintf(e.Out, "Docbank at %s is already set up in %s, and the kata daemon has attachments on; nothing changed.\n", base, configPath)
		return err
	}
	if err != nil {
		_, err = fmt.Fprintf(e.Out, "%s already has this [docbank] section, but the kata daemon could not be asked whether attachments are on: %v\n", configPath, err)
	} else {
		_, err = fmt.Fprintf(e.Out, "%s already has this [docbank] section, but the running kata daemon does not have attachments on, so the config is not in use yet.\n", configPath)
	}
	if err != nil {
		return err
	}
	return e.applyKata(ctx, r.Common, r.KeyEnv)
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
		raw, err := readKeyFile(r.KeyFile)
		if err != nil {
			return "", err
		}
		key = raw
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

// probe is the authenticated read of Docbank's API that must succeed before anything is written: it resolves the vault root, which
// Docbank answers with a folder node. A refused key, an unanswered request and a redirect are reported as the Pane reports them;
// an answer that is not Docbank's is reported as such. Redirects are never followed, so the key goes only to the url asked.
func (e Env) probe(ctx context.Context, base, key string) error {
	probeCtx, cancel := context.WithTimeout(ctx, e.probeTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, base+"/api/v1/path?path=%2F", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", key)
	client := *e.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err() // the caller gave up; a probe that merely timed out is Docbank not answering
		}
		return errors.New(data.DocbankUnreachableMessage)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return errors.New(data.DocbankUnauthorizedMessage)
	case resp.StatusCode != http.StatusOK:
		return errors.New(data.DocbankUnreachableMessage)
	}
	var root struct {
		Kind string `json:"kind"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, probeMaxBody)).Decode(&root) != nil || root.Kind != "dir" {
		return fmt.Errorf("%s answered, but not as a Docbank does (its vault root is not a folder), so the url is probably not a Docbank's; nothing was written", base)
	}
	return nil
}

// writeKeyFile stores key at path, mode 0600, written atomically; a file that already holds it is left alone, except that group and
// other permission bits on it are removed (on Unix) and said so.
func (e Env) writeKeyFile(path, key string) error {
	shown := path
	path, err := resolveLink(path)
	if err != nil {
		return err
	}
	if have, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(have)) == key {
		if runtime.GOOS == "windows" {
			return nil
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm()&0o077 == 0 {
			return err
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("tighten the key file: %w", err)
		}
		_, err = fmt.Fprintf(e.Out, "The API key in %s was readable by others (mode %04o); now mode 0600.\n", shown, info.Mode().Perm())
		return err
	}
	if err := atomicfile.Write(path, []byte(key+"\n"), atomicfile.Options{Mode: 0o600, DirMode: 0o700}); err != nil {
		return fmt.Errorf("write the key file: %w", err)
	}
	_, err = fmt.Fprintf(e.Out, "Wrote the API key to %s (mode 0600).\n", shown)
	return err
}

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
func existingSection(path, text string) (kataSection, bool, error) {
	return decodeSection[kataSection](path, text, "docbank")
}

func decodeSection[T any](path, text, name string) (section T, found bool, err error) {
	var tables map[string]toml.Primitive
	meta, err := toml.Decode(text, &tables)
	if err != nil {
		return section, false, fmt.Errorf("%s does not parse, so nothing was changed: %w", path, err)
	}
	raw, ok := tables[name]
	if !ok {
		return section, false, nil
	}
	if err := meta.PrimitiveDecode(raw, &section); err != nil {
		return section, false, fmt.Errorf("%s does not parse, so nothing was changed: %w", path, err)
	}
	return section, true, nil
}

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

func writeSection(path, existing string, want kataSection) error {
	_, _, err := appendSection(path, existing, "docbank", want.text(), false, func(got kataSection) bool { return got.matches(want) })
	return err
}

// appendSection appends text, the [name] section, to existing and writes the result over path atomically (creating the folder). The
// new file is parsed before it replaces the old one: it must hold a [name] section that ok accepts after the old file's bytes.
// An existing file keeps its mode, except that when text carries a secret the group and other bits are dropped (not on Windows,
// whose modes mean nothing); tightened is then true and was is the mode it had.
func appendSection[T any](path, existing, name, text string, secret bool, ok func(T) bool) (was fs.FileMode, tightened bool, err error) {
	path, err = resolveLink(path)
	if err != nil {
		return 0, false, err
	}
	next := existing
	if next != "" {
		if !strings.HasSuffix(next, "\n") {
			next += "\n"
		}
		next += "\n"
	}
	next += text
	mode := fs.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm() // an edit keeps the file's own mode
		if secret && runtime.GOOS != "windows" && mode&0o077 != 0 {
			was, tightened, mode = mode, true, mode&^0o077
		}
	}
	err = atomicfile.Write(path, []byte(next), atomicfile.Options{Mode: mode, DirMode: 0o700, Check: func(tmp string) error {
		now, err := readConfig(path)
		if err != nil {
			return err
		}
		if now != existing {
			return fmt.Errorf("%s changed while setup ran, so nothing was written; run setup again", path)
		}
		raw, err := os.ReadFile(tmp)
		if err != nil {
			return err
		}
		got, found, err := decodeSection[T](path, string(raw), name)
		if err != nil {
			return fmt.Errorf("the new %s would not read back, so nothing was written: %w", path, err)
		}
		if !found || !ok(got) || !bytes.HasPrefix(raw, []byte(existing)) {
			return fmt.Errorf("the new %s would not read back as asked, so nothing was written", path)
		}
		return nil
	}})
	return was, tightened && err == nil, err
}

// resolveLink is the file path names when path is a symlink that resolves, so an edit replaces the file the link points at and the
// link stays; path itself when it is not a symlink. A symlink whose target is missing is refused, so the link is never replaced.
func resolveLink(path string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", fmt.Errorf("look at %s: %w", path, err)
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		return path, nil
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		to, _ := os.Readlink(path)
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%s is a symlink to %s, which does not exist, so nothing was written", path, to)
		}
		return "", fmt.Errorf("%s is a symlink to %s that cannot be resolved, so nothing was written: %w", path, to, err)
	}
	return target, nil
}

func tomlString(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

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

// ShellWord is s as one shell word: unchanged when it holds only characters a shell never treats specially, else in single
// quotes with each embedded quote written as '\”.
func ShellWord(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_@%+=:,./-", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// RestartHint is command with env set before it, written for the shell a person on goos types it into: `NAME='v' command` for a
// POSIX shell, `$env:NAME='v'; command` for PowerShell on Windows (cmd.exe has no one-line form that PowerShell's also runs).
func RestartHint(goos string, env [][2]string, command string) string {
	var b strings.Builder
	for _, kv := range env {
		if goos == "windows" {
			fmt.Fprintf(&b, "$env:%s='%s'; ", kv[0], strings.ReplaceAll(kv[1], "'", "''"))
		} else {
			fmt.Fprintf(&b, "%s=%s ", kv[0], ShellWord(kv[1]))
		}
	}
	return b.String() + command
}
