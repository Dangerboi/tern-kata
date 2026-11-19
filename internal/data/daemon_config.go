package data

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/guardedfile"
)

// The Daemons Page manages the [[daemon]] entries of kata's client config. kata has no command for it, and a TOML library that
// decodes and encodes the file would drop its comments and unrelated keys, so the file is edited as text: only the lines of the
// entry being changed are touched, every other line stays as it is, and the result is checked with kata's own rules before it
// replaces the file.

// DaemonSpec is an entry as the Daemons Page writes it: a name, either Local or a URL, at most one of a token's environment
// variable and an inline token, and whether an unencrypted URL is allowed.
type DaemonSpec struct {
	Name          string
	Local         bool
	URL           string
	TokenEnv      string
	Token         string
	AllowInsecure bool
	// KeepToken leaves the entry's token lines as they are, for an edit that does not retype the secret.
	KeepToken bool
}

var (
	daemonTableHeader = regexp.MustCompile(`^\s*\[\[\s*daemon\s*\]\]\s*(#.*)?$`)
	// tableHeader is a table header line. The character class leaves out quotes and commas, so a line of a multi-line array does
	// not read as one.
	tableHeader     = regexp.MustCompile(`^\s*\[\[?[A-Za-z0-9_. \-]+\]\]?\s*(#.*)?$`)
	tomlCommentLine = regexp.MustCompile(`^\s*#`)
	managedKey      = regexp.MustCompile(`^\s*(name|local|url|token|token_env|allow_insecure)\s*=`)
	tokenKeyLine    = regexp.MustCompile(`^\s*(token|token_env)\s*=`)
)

// tomlString is s as a TOML basic string. Control characters are refused by the caller, so quoting only escapes quotes and
// backslashes.
func tomlString(s string) string { return strconv.Quote(s) }

func (d DaemonSpec) check() error {
	for _, field := range []struct{ name, value string }{{"name", d.Name}, {"URL", d.URL}, {"token variable", d.TokenEnv}, {"token", d.Token}} {
		if strings.ContainsFunc(field.value, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
			return fmt.Errorf("the %s holds a control character", field.name)
		}
	}
	if d.Local && d.URL != "" {
		return errors.New("exactly one of local or url is required")
	}
	if d.TokenEnv != "" && d.Token != "" {
		return errors.New("token and token_env are mutually exclusive")
	}
	return nil
}

// lines are the entry's managed keys in kata's order. Empty values are left out.
func (d DaemonSpec) lines() []string {
	out := []string{"name = " + tomlString(d.Name)}
	if d.Local {
		out = append(out, "local = true")
	} else {
		out = append(out, "url = "+tomlString(d.URL))
	}
	switch {
	case d.KeepToken:
	case d.TokenEnv != "":
		out = append(out, "token_env = "+tomlString(d.TokenEnv))
	case d.Token != "":
		out = append(out, "token = "+tomlString(d.Token))
	}
	if d.AllowInsecure {
		out = append(out, "allow_insecure = true")
	}
	return out
}

// block is the span of file lines [start, end) of one [[daemon]] entry: its header, the comment lines right above the header, and
// everything up to where the next table's own comment lines begin.
type block struct{ header, start, end int }

func daemonBlocks(lines []string) []block {
	var blocks []block
	var headers []int
	for i, line := range lines {
		if tableHeader.MatchString(line) {
			headers = append(headers, i)
		}
	}
	// start walks up over the comment lines directly above a header, which belong to it.
	start := func(h int) int {
		for h > 0 && tomlCommentLine.MatchString(lines[h-1]) {
			h--
		}
		return h
	}
	for n, h := range headers {
		if !daemonTableHeader.MatchString(lines[h]) {
			continue
		}
		end := len(lines)
		if n+1 < len(headers) {
			end = start(headers[n+1])
		}
		blocks = append(blocks, block{header: h, start: start(h), end: end})
	}
	return blocks
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// entryBlock is the block of the entry called name, from the order the decoded catalog lists the entries in.
func entryBlock(path, text, name string) (block, []string, error) {
	cat, err := parseCatalog(path, []byte(text))
	if err != nil {
		return block{}, nil, err
	}
	lines := splitLines(text)
	blocks := daemonBlocks(lines)
	for i, e := range cat.Daemons {
		if e.Name == name && i < len(blocks) {
			return blocks[i], lines, nil
		}
	}
	return block{}, nil, fmt.Errorf("daemon %q is not in the daemon catalog", name)
}

// addDaemonText is text with the entry appended as a new [[daemon]] table after a blank line.
func addDaemonText(text string, d DaemonSpec) string {
	lines := splitLines(text)
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		lines = append(lines, "")
	}
	lines = append(lines, "[[daemon]]")
	lines = append(lines, d.lines()...)
	return joinLines(lines)
}

// replaceDaemonText is text with the managed keys of the entry called old replaced by d's. Comments, home, instance_uid and any
// other line of the entry stay, and so does the entry's token when d.KeepToken.
func replaceDaemonText(path, text, old string, d DaemonSpec) (string, error) {
	b, lines, err := entryBlock(path, text, old)
	if err != nil {
		return "", err
	}
	kept := make([]string, 0, b.end-b.start)
	for _, line := range lines[b.start:b.end] {
		switch {
		case managedKey.MatchString(line) && !(d.KeepToken && tokenKeyLine.MatchString(line)):
		default:
			kept = append(kept, line)
		}
	}
	// The new keys go right under the header, which sits after the entry's leading comments.
	at := b.header - b.start
	next := append([]string{}, kept[:at+1]...)
	next = append(next, d.lines()...)
	next = append(next, kept[at+1:]...)
	out := append([]string{}, lines[:b.start]...)
	out = append(out, next...)
	out = append(out, lines[b.end:]...)
	return joinLines(out), nil
}

// removeDaemonText is text without the entry called name: its header, its comments and its keys.
func removeDaemonText(path, text, name string) (string, error) {
	b, lines, err := entryBlock(path, text, name)
	if err != nil {
		return "", err
	}
	out := append([]string{}, lines[:b.start]...)
	out = append(out, lines[b.end:]...)
	return joinLines(out), nil
}

// updateCatalog reads <KATA_HOME>/config.toml again, lets change return its next text, refuses a text that kata's own validation
// would refuse, and writes it only while the file is still what was read.
func updateCatalog(change func(path, text string) (string, error)) error {
	home, err := kataHome()
	if err != nil {
		return err
	}
	path := filepath.Join(home, "config.toml")
	return guardedfile.Update(path, 0o700, func(token guardedfile.Token) ([]byte, error) {
		next, err := change(path, token.Text())
		if err != nil {
			return nil, err
		}
		if _, err := parseCatalog(path, []byte(next)); err != nil {
			return nil, errors.New(strings.TrimPrefix(err.Error(), path+": "))
		}
		return []byte(next), nil
	})
}

// AddDaemon appends the entry to the catalog.
func AddDaemon(d DaemonSpec) error {
	if err := d.check(); err != nil {
		return err
	}
	return updateCatalog(func(_, text string) (string, error) { return addDaemonText(text, d), nil })
}

// UpdateDaemon rewrites the entry called old as d.
func UpdateDaemon(old string, d DaemonSpec) error {
	if err := d.check(); err != nil {
		return err
	}
	return updateCatalog(func(path, text string) (string, error) { return replaceDaemonText(path, text, old, d) })
}

// RemoveDaemon deletes the entry called name. A file whose active_daemon names it is refused by the validation that follows.
func RemoveDaemon(name string) error {
	return updateCatalog(func(path, text string) (string, error) { return removeDaemonText(path, text, name) })
}
