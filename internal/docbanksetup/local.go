package docbanksetup

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Dangerboi/tern-kata/internal/atomicfile"
)

// DefaultPort is the port Docbank is given when --port names none.
const DefaultPort = 8080

// DocbankRepo is where Docbank is installed from, named when it is not on PATH.
const DocbankRepo = "https://github.com/kenn-io/docbank"

// Local is `docbank setup` without --url: the flags that set Docbank up on this machine and point kata at it.
type Local struct {
	// Port is the fixed port Docbank listens on; zero means DefaultPort, or the port an existing [server] section already has.
	Port   int
	Folder string // empty means DefaultFolder
	// Yes restarts the kata daemon without asking; NoRestart leaves it alone.
	Yes, NoRestart bool
}

// docbankServer is the [server] section of Docbank's config.toml as setup reads and writes it.
type docbankServer struct {
	BindAddr string `toml:"bind_addr"`
	APIPort  *int   `toml:"api_port"`
	APIKey   string `toml:"api_key"`
}

type docbankConfig struct {
	Server *docbankServer `toml:"server"`
}

// SetupLocal gives Docbank on this machine a fixed address and key, restarts it so it reads them, and then points kata at it exactly
// as SetupRemote does. Nothing is written until docbank and kata are on PATH, kata's [docbank] section is compatible, and the port is
// free (or already this Docbank's).
func (e Env) SetupLocal(ctx context.Context, l Local) error {
	if e.DocbankHome == "" {
		return errors.New("cannot tell where Docbank's home is; set DOCBANK_HOME")
	}
	folder, err := normalizeFolder(l.Folder)
	if err != nil {
		return err
	}
	if l.Port < 0 || l.Port > 65535 {
		return fmt.Errorf("--port %d must be between 1 and 65535", l.Port)
	}
	if _, err := e.Runner.LookPath("docbank"); err != nil {
		return fmt.Errorf("docbank is not on PATH; install Docbank first (%s), nothing was written", DocbankRepo)
	}
	if _, err := e.Runner.LookPath("kata"); err != nil {
		return errors.New("kata is not on PATH; install kata first (https://github.com/kenn-io/kata), nothing was written")
	}

	docPath := filepath.Join(e.DocbankHome, "config.toml")
	docText, err := readConfig(docPath)
	if err != nil {
		return err
	}
	server, found, err := existingServer(docPath, docText)
	if err != nil {
		return err
	}
	var port int
	var key string
	if found {
		var missing []string
		if server.APIPort == nil {
			missing = append(missing, "api_port = <a free port>")
		}
		if server.APIKey == "" {
			missing = append(missing, `api_key = "<a key>"`)
		}
		if len(missing) > 0 {
			return fmt.Errorf("%s has a [server] section without %s: add it under [server] (setup needs a fixed port and key, or no [server] section at all), nothing was changed", docPath, strings.Join(missing, " and "))
		}
		port, key = *server.APIPort, server.APIKey
		if port < 1 || port > 65535 {
			return fmt.Errorf("%s: api_port %d is not a port, nothing was changed", docPath, port)
		}
		if l.Port != 0 && l.Port != port {
			return fmt.Errorf("--port %d differs from api_port %d in %s, so nothing was changed; drop --port or edit that file", l.Port, port, docPath)
		}
	} else {
		if port = l.Port; port == 0 {
			port = DefaultPort
		}
		if key, err = newKey(); err != nil {
			return err
		}
	}

	keyPath := filepath.Join(e.KataHome, KeyFileName)
	keyOnDisk := false
	if raw, err := os.ReadFile(keyPath); err == nil && strings.TrimSpace(string(raw)) != "" {
		if strings.TrimSpace(string(raw)) != key {
			return fmt.Errorf("%s already holds a different key than Docbank's, so nothing was changed; remove it or give Docbank that key", keyPath)
		}
		keyOnDisk = true
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read the key file: %w", err)
	}

	addr := "127.0.0.1:" + strconv.Itoa(port)
	base := "http://" + addr
	want := kataSection{URL: base, Folder: folder, APIKeyFile: keyPath}
	kataPath := filepath.Join(e.KataHome, "config.toml")
	kataText, err := readConfig(kataPath)
	if err != nil {
		return err
	}
	have, haveKata, err := existingSection(kataPath, kataText)
	if err != nil {
		return err
	}
	if haveKata && !have.matches(want) {
		return fmt.Errorf("%s already has a [docbank] section that differs from what was asked, so nothing was changed:\n%s", kataPath, have.describe())
	}
	write := !found
	if found && keyOnDisk && haveKata {
		return e.SetupRemote(ctx, Remote{URL: base, KeyFile: keyPath, Folder: l.Folder, Yes: l.Yes, NoRestart: l.NoRestart}) // finished: says so
	}

	running := statusNames(e.docbankStatus(ctx), addr)
	if !running {
		ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("port %d on 127.0.0.1 is in use by something other than this Docbank, so nothing was written; free it or pick another with --port", port)
		}
		_ = ln.Close()
	}

	if write {
		if err := writeServer(docPath, docText, docbankServer{BindAddr: "127.0.0.1", APIPort: &port, APIKey: key}); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(e.Out, "Wrote the [server] section of %s: 127.0.0.1, port %d, an API key.\n", docPath, port); err != nil {
			return err
		}
	}
	if !keyOnDisk {
		if err := e.writeKeyFile(keyPath, key); err != nil {
			return err
		}
	}
	if write || !running {
		if err := e.restartDocbank(ctx, addr); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(e.Out, "Docbank is up at %s.\nDocbank reports usage unless its daemon's environment sets DOCBANK_TELEMETRY_ENABLED=0.\n", addr); err != nil {
		return err
	}
	return e.SetupRemote(ctx, Remote{URL: base, KeyFile: keyPath, Folder: l.Folder, Yes: l.Yes, NoRestart: l.NoRestart})
}

// docbankStatus is what `docbank daemon status` prints; a Docbank that is not running exits non-zero, which is an answer too.
func (e Env) docbankStatus(ctx context.Context) string {
	ctx, cancel := contextWithCheckDeadline(ctx)
	defer cancel()
	out, _ := e.Runner.Run(ctx, nil, "docbank", "daemon", "status")
	return string(out)
}

// restartDocbank runs `docbank daemon restart` and then requires `docbank daemon status` to name addr.
func (e Env) restartDocbank(ctx context.Context, addr string) error {
	ctx, cancel := contextWithCheckDeadline(ctx)
	defer cancel()
	if out, err := e.Runner.Run(ctx, nil, "docbank", "daemon", "restart"); err != nil {
		return fmt.Errorf("`docbank daemon restart` failed: %w\n%s\nkata's config was not touched", err, strings.TrimSpace(string(out)))
	}
	out, _ := e.Runner.Run(ctx, nil, "docbank", "daemon", "status")
	if !statusNames(string(out), addr) {
		return fmt.Errorf("Docbank did not come up at %s, so kata's config was not touched; `docbank daemon status` says:\n%s", addr, strings.TrimSpace(string(out)))
	}
	_, err := fmt.Fprintf(e.Out, "Restarted Docbank (`docbank daemon restart`).\n")
	return err
}

func contextWithCheckDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, checkDeadline)
}

// statusNames reports whether Docbank's status output names addr as a whole address (8080 is not 80801).
func statusNames(status, addr string) bool {
	return regexp.MustCompile(regexp.QuoteMeta(addr) + `\b`).MatchString(status)
}

// newKey is 32 random bytes from the operating system's secure source, as hex.
func newKey() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("make an API key: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// existingServer is the [server] section text holds, found false when there is none. A file that does not parse is refused.
func existingServer(path, text string) (server docbankServer, found bool, err error) {
	var cfg docbankConfig
	meta, err := toml.Decode(text, &cfg)
	if err != nil {
		return docbankServer{}, false, fmt.Errorf("%s does not parse, so nothing was changed: %w", path, err)
	}
	if !meta.IsDefined("server") || cfg.Server == nil {
		return docbankServer{}, false, nil
	}
	return *cfg.Server, true, nil
}

// serverText is the section as appended to Docbank's config.
func serverText(s docbankServer) string {
	var b strings.Builder
	b.WriteString("[server]\n")
	fmt.Fprintf(&b, "bind_addr = %s\n", tomlString(s.BindAddr))
	fmt.Fprintf(&b, "api_port = %d\n", *s.APIPort)
	fmt.Fprintf(&b, "api_key = %s\n", tomlString(s.APIKey))
	return b.String()
}

// writeServer appends the section to existing and writes the result over path atomically, creating the folder. The new file is parsed
// before it replaces the old one and must hold exactly the section after the old file's bytes.
func writeServer(path, existing string, want docbankServer) error {
	path = resolveLink(path)
	next := existing
	if next != "" {
		if !strings.HasSuffix(next, "\n") {
			next += "\n"
		}
		next += "\n"
	}
	next += serverText(want)
	mode := fs.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return atomicfile.Write(path, []byte(next), atomicfile.Options{Mode: mode, DirMode: 0o700, Check: func(tmp string) error {
		raw, err := os.ReadFile(tmp)
		if err != nil {
			return err
		}
		got, found, err := existingServer(path, string(raw))
		if err != nil {
			return fmt.Errorf("the new %s would not read back, so nothing was written: %w", path, err)
		}
		if !found || got.APIPort == nil || *got.APIPort != *want.APIPort || got.APIKey != want.APIKey || got.BindAddr != want.BindAddr || !bytes.HasPrefix(raw, []byte(existing)) {
			return fmt.Errorf("the new %s would not read back as asked, so nothing was written", path)
		}
		return nil
	}})
}
