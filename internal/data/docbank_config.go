package data

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// DocbankSection is the [docbank] table of a kata daemon's config.toml, as far as telling its setups apart needs: unknown keys are
// ignored, so a key a later kata adds never fails the read.
type DocbankSection struct {
	URL    string `toml:"url"`
	Folder string `toml:"folder"`
	Local  bool   `toml:"local"`
}

// DocbankForm says which setup a [docbank] section is.
type DocbankForm uint8

const (
	// DocbankNotSet is no section, or one that names neither a url nor local = true.
	DocbankNotSet DocbankForm = iota
	// DocbankLocal is `kata docbank setup` without --url: local = true, with kata finding and starting the Docbank on this machine.
	DocbankLocal
	// DocbankAtURL is a Docbank at a url that tern-kata's old setup did not write.
	DocbankAtURL
	// DocbankPinned is the setup tern-kata 0.10 wrote: a url on this machine whose port is the [server] api_port it gave Docbank.
	// kata does not start a Docbank it is given by url.
	DocbankPinned
)

// ReadDocbankSection reads the [docbank] section of the config.toml in kataHome. A missing file has no section; a file that
// cannot be read or parsed is an error naming its path.
func ReadDocbankSection(kataHome string) (DocbankSection, error) {
	path := filepath.Join(kataHome, "config.toml")
	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return DocbankSection{}, err
	}
	return parseDocbankSection(path, string(raw))
}

// parseDocbankSection decodes the [docbank] section of the config.toml text raw, read from path.
func parseDocbankSection(path, raw string) (DocbankSection, error) {
	var file struct {
		Docbank DocbankSection `toml:"docbank"`
	}
	if _, err := toml.Decode(raw, &file); err != nil {
		return DocbankSection{}, fmt.Errorf("%s: %w", path, err)
	}
	file.Docbank.URL = strings.TrimSpace(file.Docbank.URL)
	return file.Docbank, nil
}

// ClassifyDocbank is the one rule that tells the forms apart. A url wins over local = true, as it does in kata. A url is the old
// pinned form when its host is this machine and its port is [server] api_port in <docbankHome>/config.toml, which is where
// tern-kata's old setup pinned Docbank.
func ClassifyDocbank(s DocbankSection, docbankHome string) DocbankForm {
	switch {
	case s.URL != "":
		if pinnedToDocbank(s.URL, docbankHome) {
			return DocbankPinned
		}
		return DocbankAtURL
	case s.Local:
		return DocbankLocal
	}
	return DocbankNotSet
}

// DocbankHome is Docbank's home: DOCBANK_HOME, else ~/.docbank.
func DocbankHome() (string, error) {
	if home := os.Getenv("DOCBANK_HOME"); home != "" {
		return filepath.Abs(home)
	}
	user, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(user, ".docbank"), nil
}

func pinnedToDocbank(rawURL, docbankHome string) bool {
	if docbankHome == "" {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "127.0.0.1", "localhost", "::1":
	default:
		return false
	}
	port := u.Port()
	if port == "" {
		return false
	}
	want, err := strconv.Atoi(port)
	if err != nil {
		return false
	}
	var file struct {
		Server struct {
			APIPort *int `toml:"api_port"`
		} `toml:"server"`
	}
	if _, err := toml.DecodeFile(filepath.Join(docbankHome, "config.toml"), &file); err != nil {
		return false
	}
	return file.Server.APIPort != nil && *file.Server.APIPort == want
}

// docbankRow is the Docbank row: the [docbank] section of the default kata daemon's config.toml, whichever daemon the Pane is
// switched to. A file that cannot be read shows the reader's error like the other rows.
func (g *Gateway) docbankRow() model.ConfigFile {
	const title = "Docbank (the [docbank] section of kata's config.toml)"
	home, err := kataHome()
	if err != nil {
		return model.ConfigFile{Key: model.ConfigDocbank, Title: title, Status: model.ConfigBroken, Err: err.Error()}
	}
	pinned := false
	row := configRow(model.ConfigDocbank, title, filepath.Join(home, "config.toml"), "not set up: `kata docbank setup` sets Docbank up", nil, func(string) (string, error) {
		section, err := ReadDocbankSection(home)
		if err != nil {
			return "", err
		}
		docbankHome, _ := DocbankHome() // without a home no url is the old form
		switch ClassifyDocbank(section, docbankHome) {
		case DocbankLocal:
			return "the Docbank on this machine: kata starts it when needed and follows its port and key", nil
		case DocbankAtURL:
			return "Docbank at " + section.URL, nil
		case DocbankPinned:
			pinned = true
			return "pinned to " + section.URL + " by tern-kata's old setup: kata does not start it", nil
		}
		return "not set up: `kata docbank setup` sets Docbank up", nil
	})
	if pinned {
		row.Actions = []model.ConfigAction{model.ConfigSwitchDocbank}
	}
	return row
}
