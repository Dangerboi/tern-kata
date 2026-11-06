package docbanksetup

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Dangerboi/tern-kata/internal/worker"
)

// ManualFileName is the supervision marker's name in tern-kata's config folder, the one place it is spelled. The Luau host half
// (host.luau) holds no copy of the rule: it asks `tern-kata docbank supervised`, which answers from Env.Supervised.
const ManualFileName = "docbank.manual"

// ManualPath is the supervision marker.
func (e Env) ManualPath() string { return filepath.Join(e.ConfigDir, ManualFileName) }

// Supervised is whether the plugin keeps Docbank running, and when it does not, why (one sentence naming what is missing).
// It is the one place the rule is decided; the host half asks `tern-kata docbank supervised`.
func (e Env) Supervised() (on bool, why string) {
	if e.ConfigDir == "" {
		return false, "cannot tell where tern-kata's config folder is"
	}
	if e.DefaultKataHome == "" {
		return false, "cannot tell where the default kata daemon's home is"
	}
	if _, err := os.Stat(e.ManualPath()); err == nil {
		return false, ManualFileName + " exists in " + e.ConfigDir
	}
	kataPath := filepath.Join(e.DefaultKataHome, "config.toml")
	kataText, err := readConfig(kataPath)
	if err != nil {
		return false, err.Error()
	}
	section, found, err := existingSection(kataPath, kataText)
	if err != nil {
		return false, err.Error()
	}
	if !found || section.URL == "" {
		return false, kataPath + " has no [docbank] url"
	}
	u, err := url.Parse(section.URL)
	if err != nil || !isLoopback(u.Hostname()) {
		return false, "kata's [docbank] url " + section.URL + " is not on this machine"
	}
	urlPort, err := portOf(u)
	if err != nil {
		return false, "kata's [docbank] url " + section.URL + " names no usable port"
	}
	if e.DocbankHome == "" {
		return false, "cannot tell where Docbank's home is"
	}
	docPath := filepath.Join(e.DocbankHome, "config.toml")
	docText, err := readConfig(docPath)
	if err != nil {
		return false, err.Error()
	}
	server, found, err := existingServer(docPath, docText)
	if err != nil {
		return false, err.Error()
	}
	if !found || server.APIPort == nil || *server.APIPort != urlPort {
		return false, fmt.Sprintf("Docbank's [server] api_port in %s is not %d, the port of kata's [docbank] url", docPath, urlPort)
	}
	return true, ""
}

// portOf is the port a URL names, its scheme's default when it names none.
func portOf(u *url.URL) (int, error) {
	if p := u.Port(); p != "" {
		return strconv.Atoi(p)
	}
	switch u.Scheme {
	case "http":
		return 80, nil
	case "https":
		return 443, nil
	}
	return 0, errors.New("no port")
}

// SetSupervision turns the plugin's supervision of Docbank off by creating the marker (and the config folder when it is missing)
// or on by removing it; either, repeated, is no error.
func (e Env) SetSupervision(on bool) error {
	if e.ConfigDir == "" {
		return errors.New("cannot tell where tern-kata's config folder is")
	}
	return worker.SetMarker(e.ConfigDir, e.ManualPath(), on)
}

// reportSupervision is the line local setup ends with, saying whether the plugin keeps this Docbank running. The rule covers only
// the default kata daemon, which is the one the host half asks about, so a setup for a profile or a --home daemon says that
// instead of claiming the plugin keeps its Docbank running.
func (e Env) reportSupervision() error {
	if e.ConfigDir == "" {
		return nil
	}
	if e.DefaultKataHome == "" {
		_, err := fmt.Fprintln(e.Out, "The plugin keeps Docbank running only for the default kata daemon, and that daemon cannot be resolved here (is KATA_SERVER set?). To keep this Docbank running, start it with `docbank daemon start` when it is down, or run it as a service.")
		return err
	}
	if filepath.Clean(e.DefaultKataHome) != filepath.Clean(e.KataHome) {
		_, err := fmt.Fprintf(e.Out, "The plugin keeps Docbank running only for the default kata daemon (%s), not for this one. To keep this Docbank running, start it with `docbank daemon start` when it is down, or run it as a service.\n", e.DefaultKataHome)
		return err
	}
	if _, err := os.Stat(e.ManualPath()); err == nil {
		_, err = fmt.Fprintf(e.Out, "The plugin does not keep Docbank running: %s exists. `tern-kata docbank supervise on` lets it.\n", e.ManualPath())
		return err
	}
	if on, why := e.Supervised(); !on {
		_, err := fmt.Fprintf(e.Out, "The plugin does not keep Docbank running: %s.\n", why)
		return err
	}
	_, err := fmt.Fprintf(e.Out, "The plugin keeps Docbank running: its host half starts it with `docbank daemon start` when it is not running. `tern-kata docbank supervise off` stops that (creates %s).\n", e.ManualPath())
	return err
}
