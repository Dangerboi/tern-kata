package data

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/Dangerboi/tern-kata/internal/guardedfile"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// ErrViewsChanged is a write finding views.toml no longer as the read it was given. Nothing was written; read again.
var ErrViewsChanged = errors.New("views.toml changed since it was read")

// ErrViewNotFound is a write naming a view the file does not have (any more).
var ErrViewNotFound = errors.New("no such saved view")

// ViewsToken is what a read of views.toml saw: the file's text, empty for a missing file. A write takes it back and refuses
// with ErrViewsChanged when the file on disk is no longer that text. Callers treat it as opaque.
type ViewsToken struct{ text string }

// ViewsRead is one read of views.toml.
type ViewsRead struct {
	// Views are the saved views the file holds that pass model.CheckSavedView, in file order.
	Views []model.SavedView
	// Problems are the entries left out of Views, each saying which [[view]] and why. They stay in the file.
	Problems []error
	// Token is for the writes that follow this read.
	Token ViewsToken
}

// ViewsStore is the views.toml beside filters.toml. It needs no Gateway, Daemon or Tern, so the CLI and the Pane use the same
// one.
type ViewsStore struct{ Path string }

// NewViewsStore is the store of the config folder (tern-kata's, from worker.DefaultDirs).
func NewViewsStore(configFolder string) ViewsStore {
	return ViewsStore{Path: filepath.Join(configFolder, "views.toml")}
}

// ViewsStore is the gateway's store, in the same folder as filters.toml.
func (g *Gateway) ViewsStore() (ViewsStore, error) {
	folder, err := g.configFolder()
	if err != nil {
		return ViewsStore{}, err
	}
	return NewViewsStore(folder), nil
}

// viewRecord is one [[view]] table as decoded. Keys the file has beyond these are ignored here and kept by the writers.
type viewRecord struct {
	Name         string `toml:"name"`
	Base         string `toml:"base"`
	ProjectUID   string `toml:"project_uid"`
	ProjectName  string `toml:"project_name"`
	Project      string `toml:"project"`
	Status       string `toml:"status"`
	Owner        string `toml:"owner"`
	Label        string `toml:"label"`
	Relationship string `toml:"relationship"`
	Search       string `toml:"search"`
}

// projectBase is the base value of a view on one Project.
const projectBase = "project"

func (r viewRecord) view() model.SavedView {
	base := model.Collection{View: r.Base}
	if r.Base == projectBase {
		base = model.ProjectCollection(r.ProjectUID, r.ProjectName)
	}
	return model.SavedView{Name: r.Name, Base: base, Filters: model.Filters{ProjectUID: r.Project, Status: r.Status, Owner: r.Owner, Label: r.Label, Relationship: r.Relationship, Search: r.Search}}
}

// owned are the keys a write sets on a [[view]] table; every other line of the table is kept.
var owned = func() []string {
	keys := []string{"name", "base", "project_uid", "project_name"}
	for _, kv := range filterPairs(model.Filters{}) {
		keys = append(keys, kv[0])
	}
	return keys
}()

// filterPairs are the filter keys of a [[view]] table with the values f gives them, in the order they are written.
func filterPairs(f model.Filters) [][2]string {
	return [][2]string{{"project", f.ProjectUID}, {"status", f.Status}, {"owner", f.Owner}, {"label", f.Label}, {"relationship", f.Relationship}, {"search", f.Search}}
}

// Read reads the file. A missing file is no views. An unreadable one is an error saying why, and no token, so nothing can
// write to it.
func (s ViewsStore) Read() (ViewsRead, error) {
	tok, err := guardedfile.Read(s.Path)
	if err != nil {
		return ViewsRead{}, fmt.Errorf("%s: %w", s.Path, err)
	}
	p, err := parseViewsText(tok.Text())
	if err != nil {
		return ViewsRead{}, fmt.Errorf("%s: %w", s.Path, err)
	}
	out := ViewsRead{Token: ViewsToken{tok.Text()}}
	// Each name is checked against every earlier entry, left-out ones included, as the writes do, so find and the list agree
	// on the first entry of a name.
	var earlier []model.SavedView
	for i, r := range p.records {
		v, err := model.CheckSavedView(r.view(), earlier, "")
		earlier = append(earlier, model.SavedView{Name: strings.TrimSpace(r.Name)})
		if err != nil {
			out.Problems = append(out.Problems, fmt.Errorf("%s: [[view]] %d (%q): %w", s.Path, i+1, r.Name, err))
			continue
		}
		out.Views = append(out.Views, v)
	}
	return out, nil
}

// Add appends v to the file. It refuses a name another view has, with the model's message.
func (s ViewsStore) Add(token ViewsToken, v model.SavedView) error {
	return s.edit(token, func(p *viewsText) (string, error) {
		v, err := model.CheckSavedView(v, p.others(-1), "")
		if err != nil {
			return "", err
		}
		return p.insert(v), nil
	})
}

// Replace puts v in place of the view called self (ignoring case), so v may be a rename or an edit. The view keeps its place in
// the file, its comments and the keys this module does not know. A name v takes from another view is refused.
func (s ViewsStore) Replace(token ViewsToken, self string, v model.SavedView) error {
	return s.edit(token, func(p *viewsText) (string, error) {
		i := p.find(self)
		if i < 0 {
			return "", fmt.Errorf("%w: %q", ErrViewNotFound, self)
		}
		v, err := model.CheckSavedView(v, p.others(i), "")
		if err != nil {
			return "", err
		}
		return p.replace(i, v), nil
	})
}

// Delete removes the view called name (ignoring case).
func (s ViewsStore) Delete(token ViewsToken, name string) error {
	return s.edit(token, func(p *viewsText) (string, error) {
		i := p.find(name)
		if i < 0 {
			return "", fmt.Errorf("%w: %q", ErrViewNotFound, name)
		}
		return p.remove(i), nil
	})
}

// edit applies change to the text the token holds and writes the result, refusing if the file is not that text any more.
// The guard is guardedfile's.
func (s ViewsStore) edit(token ViewsToken, change func(*viewsText) (string, error)) error {
	p, err := parseViewsText(token.text)
	if err != nil {
		return fmt.Errorf("%s: %w", s.Path, err)
	}
	next, err := change(&p)
	if err != nil {
		return err
	}
	if _, err := parseViewsText(next); err != nil {
		return fmt.Errorf("%s: the new file would not read back, so nothing was written: %w", s.Path, err)
	}
	err = guardedfile.Write(s.Path, 0o755, guardedfile.TokenOf(token.text), []byte(next))
	if errors.Is(err, guardedfile.ErrChanged) {
		return ErrViewsChanged
	}
	return err
}

// viewsText is the file's text split into lines, with where each [[view]] table starts and ends.
type viewsText struct {
	lines   []string
	records []viewRecord
	blocks  [][2]int // [start, end) line numbers of each [[view]] table, the end after its last key line
	crlf    bool     // the file breaks lines with CRLF; lines are held without it
	bom     string   // the UTF-8 BOM the file starts with, kept by writes; lines are held without it
}

const utf8BOM = "\xef\xbb\xbf"

var (
	viewHeader  = regexp.MustCompile(`^\s*\[\[\s*(?:view|"view"|'view')\s*\]\]\s*(#.*)?$`)
	anyHeader   = regexp.MustCompile(`^\s*\[`)
	commentOnly = regexp.MustCompile(`^\s*(#.*)?$`)
)

// parseViewsText decodes text and locates its [[view]] tables. It fails if text is not TOML, or if a [[view]] is written in a way
// the line scan cannot find.
func parseViewsText(text string) (viewsText, error) {
	bom := ""
	if strings.HasPrefix(text, utf8BOM) {
		bom, text = utf8BOM, text[len(utf8BOM):]
	}
	var doc struct {
		View []viewRecord `toml:"view"`
	}
	if _, err := toml.Decode(text, &doc); err != nil {
		return viewsText{}, err
	}
	p := viewsText{records: doc.View, bom: bom}
	if text != "" {
		p.crlf = strings.Contains(text, "\r\n")
		for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
			p.lines = append(p.lines, strings.TrimSuffix(line, "\r"))
		}
	}
	for i := 0; i < len(p.lines); i++ {
		if !viewHeader.MatchString(p.lines[i]) {
			continue
		}
		end, next := i+1, i+1
		for ; next < len(p.lines) && !anyHeader.MatchString(p.lines[next]); next++ {
			if !commentOnly.MatchString(p.lines[next]) {
				end = next + 1
			}
		}
		p.blocks = append(p.blocks, [2]int{i, end})
		i = next - 1
	}
	if len(p.blocks) != len(p.records) {
		return viewsText{}, errors.New("a [[view]] table is written in a form this program cannot edit safely")
	}
	return p, nil
}

func (p *viewsText) find(name string) int {
	name = strings.TrimSpace(name)
	for i, r := range p.records {
		if strings.EqualFold(strings.TrimSpace(r.Name), name) {
			return i
		}
	}
	return -1
}

// others are the views of the file except the one at skip and the entries of its name, which a read leaves out as its
// duplicates, for the name check; entries that fail the check count too, since a name is taken either way. Names are trimmed
// as Read trims them.
func (p *viewsText) others(skip int) []model.SavedView {
	var out []model.SavedView
	for i, r := range p.records {
		if i != skip && (skip < 0 || !strings.EqualFold(strings.TrimSpace(r.Name), strings.TrimSpace(p.records[skip].Name))) {
			out = append(out, model.SavedView{Name: strings.TrimSpace(r.Name)})
		}
	}
	return out
}

// text joins lines with the file's own line break, so a write into a CRLF file stays CRLF throughout, and keeps its BOM.
func (p *viewsText) text(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	eol := "\n"
	if p.crlf {
		eol = "\r\n"
	}
	return p.bom + strings.Join(lines, eol) + eol
}

func (p *viewsText) insert(v model.SavedView) string {
	lines := append([]string(nil), p.lines...)
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		lines = append(lines, "")
	}
	lines = append(lines, "[[view]]")
	lines = append(lines, viewLines(v)...)
	return p.text(lines)
}

func (p *viewsText) replace(i int, v model.SavedView) string {
	start, end := p.blocks[i][0], p.blocks[i][1]
	kept := []string{}
	for _, line := range p.lines[start+1 : end] {
		if !ownedLine(line) {
			kept = append(kept, line)
		}
	}
	lines := append([]string(nil), p.lines[:start+1]...)
	lines = append(lines, viewLines(v)...)
	lines = append(lines, kept...)
	lines = append(lines, p.lines[end:]...)
	return p.text(lines)
}

func (p *viewsText) remove(i int) string {
	start, end := p.blocks[i][0], p.blocks[i][1]
	if start > 0 && strings.TrimSpace(p.lines[start-1]) == "" && (end >= len(p.lines) || strings.TrimSpace(p.lines[end]) == "") {
		start--
	}
	lines := append(append([]string(nil), p.lines[:start]...), p.lines[end:]...)
	return p.text(lines)
}

func ownedLine(line string) bool {
	key, _, found := strings.Cut(line, "=")
	if !found {
		return false
	}
	key = strings.Trim(strings.TrimSpace(key), `"'`)
	for _, k := range owned {
		if key == k {
			return true
		}
	}
	return false
}

// viewLines are the owned key lines of v; an empty filter is not written.
func viewLines(v model.SavedView) []string {
	out := []string{"name = " + quote(v.Name)}
	if v.Base.IsProject() {
		out = append(out, "base = "+quote(projectBase), "project_uid = "+quote(v.Base.ProjectUID), "project_name = "+quote(v.Base.Title))
	} else {
		out = append(out, "base = "+quote(v.Base.View))
	}
	for _, kv := range filterPairs(v.Filters) {
		if kv[1] != "" {
			out = append(out, kv[0]+" = "+quote(kv[1]))
		}
	}
	return out
}

// quote is s as a TOML basic string.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
