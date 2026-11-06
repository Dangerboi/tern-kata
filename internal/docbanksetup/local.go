package docbanksetup

import (
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
)

// DefaultPort is the port Docbank is given when --port names none.
const DefaultPort = 8080

// DocbankRepo is where Docbank is installed from, named when it is not on PATH.
const DocbankRepo = "https://github.com/kenn-io/docbank"

// Local is `docbank setup` without --url: the flags that set Docbank up on this machine and point kata at it.
type Local struct {
	Common
	// Port is the fixed port Docbank listens on; zero means DefaultPort, or the port an existing [server] section already has.
	Port int
}

// docbankServer is the [server] section of Docbank's config.toml as setup reads and writes it.
type docbankServer struct {
	BindAddr string `toml:"bind_addr"`
	APIPort  *int   `toml:"api_port"`
	APIKey   string `toml:"api_key"`
}

// SetupLocal gives Docbank on this machine a fixed address and key, restarts it so it reads them, and then points kata at it exactly
// as SetupRemote does. Nothing is written until docbank and kata are on PATH, kata's [docbank] section is compatible, and the port is
// free (or already this Docbank's).
func (e Env) SetupLocal(ctx context.Context, l Local) error {
	if err := e.setupLocal(ctx, l); err != nil {
		return err
	}
	return e.reportSupervision()
}

func (e Env) setupLocal(ctx context.Context, l Local) error {
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
		return errors.New(kataMissing)
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
	} else if port = l.Port; port == 0 {
		port = DefaultPort
	}

	keyPath := filepath.Join(e.KataHome, KeyFileName)
	diskKey, err := readKeyFile(keyPath)
	if err != nil {
		return err
	}
	switch {
	case found && diskKey != "" && diskKey != key:
		return fmt.Errorf("%s already holds a different key than Docbank's, so nothing was changed; remove it or give Docbank that key", keyPath)
	case !found && diskKey != "":
		key = diskKey // a key already stored for kata becomes Docbank's, so no second key is made
	case !found:
		if key, err = newKey(); err != nil {
			return err
		}
	}
	keyOnDisk := diskKey != ""

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
	remote := Remote{Common: l.Common, URL: base, KeyFile: keyPath}
	if found && keyOnDisk && haveKata {
		return e.SetupRemote(ctx, remote) // finished: says so
	}

	status, err := e.docbankStatus(ctx)
	if err != nil {
		return err
	}
	running := statusNames(status, addr)
	if !running {
		ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("port %d on 127.0.0.1 is in use by something other than this Docbank, so nothing was written; free it or pick another with --port", port)
		}
		_ = ln.Close()
	}

	write := !found
	if write {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writeServer(docPath, docText, docbankServer{BindAddr: "127.0.0.1", APIPort: &port, APIKey: key}); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(e.Out, "Wrote the [server] section of %s: 127.0.0.1, port %d, an API key.\n", docPath, port); err != nil {
			return err
		}
	}
	if !keyOnDisk {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.writeKeyFile(keyPath, key); err != nil {
			return err
		}
	}
	if write || !running {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.restartDocbank(ctx, addr); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(e.Out, "Docbank is up at %s.\nDocbank reports usage unless its daemon's environment sets DOCBANK_TELEMETRY_ENABLED=0.\n", addr); err != nil {
		return err
	}
	return e.SetupRemote(ctx, remote)
}

// readKeyFile is the key the file at path holds, empty when the file is missing or blank.
func readKeyFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read the key file: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}

// docbankStatus is what `docbank daemon status` prints; a Docbank that is not running exits non-zero, which is an answer too. A
// status cut short by a cancelled context or its deadline is no answer, and is an error.
func (e Env) docbankStatus(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, checkDeadline)
	defer cancel()
	out, _ := e.Runner.Run(ctx, nil, "docbank", "daemon", "status")
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("`docbank daemon status` did not finish: %w", err)
	}
	return string(out), nil
}

// restartDocbank runs `docbank daemon restart` and then requires `docbank daemon status` to name addr.
func (e Env) restartDocbank(ctx context.Context, addr string) error {
	ctx, cancel := context.WithTimeout(ctx, checkDeadline)
	defer cancel()
	if out, err := e.Runner.Run(ctx, nil, "docbank", "daemon", "restart"); err != nil {
		return fmt.Errorf("`docbank daemon restart` failed: %w\n%s\nkata's config was not touched", err, strings.TrimSpace(string(out)))
	}
	status, err := e.docbankStatus(ctx)
	if err != nil {
		return err
	}
	if !statusNames(status, addr) {
		return fmt.Errorf("Docbank did not come up at %s, so kata's config was not touched; `docbank daemon status` says:\n%s", addr, strings.TrimSpace(status))
	}
	_, err = fmt.Fprintf(e.Out, "Restarted Docbank (`docbank daemon restart`).\n")
	return err
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
func existingServer(path, text string) (docbankServer, bool, error) {
	return decodeSection[docbankServer](path, text, "server")
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
	return appendSection(path, existing, "server", serverText(want), func(got docbankServer) bool {
		return got.APIPort != nil && *got.APIPort == *want.APIPort && got.APIKey == want.APIKey && got.BindAddr == want.BindAddr
	})
}
