package data

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// attachmentview.luau tells an open of an Attachment by the folder attachmentCacheDir downloads into; this reads the pattern from
// the plugin source so a change to either side fails here.
func TestTheWindowPluginRecognisesTheFolderAttachmentsAreDownloadedInto(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "attachmentview.luau"))
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`path:find\("((?:[^"\\]|\\.)*)"\)`).FindSubmatch(src)
	if found == nil {
		t.Fatal(`attachmentview.luau no longer tests the path with path:find("...")`)
	}
	lua := strings.ReplaceAll(string(found[1]), `\\`, `\`)
	var pattern strings.Builder
	for i := 0; i < len(lua); i++ {
		switch {
		case lua[i] == '%' && i+1 < len(lua):
			i++
			pattern.WriteString(regexp.QuoteMeta(lua[i : i+1]))
		case lua[i] == '\\':
			pattern.WriteString(`\\`)
		default:
			pattern.WriteByte(lua[i])
		}
	}
	re, err := regexp.Compile(pattern.String())
	if err != nil {
		t.Fatalf("pattern %q from %q: %v", pattern.String(), lua, err)
	}

	dir, err := attachmentCacheDir()
	if err != nil {
		t.Skip(err)
	}
	path := filepath.Join(dir, "01JABCDEFGHJKMNPQRSTVWXYZ0", "A1", "3f2a", "shot.png")
	for _, p := range []string{path, filepath.ToSlash(path), strings.ReplaceAll(path, "/", `\`)} {
		if !re.MatchString(p) {
			t.Errorf("the plugin's pattern %q does not match %q, a file the Pane downloads", lua, p)
		}
	}
	if re.MatchString(filepath.Join(filepath.Dir(dir), "other", "shot.png")) {
		t.Errorf("the plugin's pattern %q matches a folder that is not the download folder", lua)
	}
}
