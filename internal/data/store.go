package data

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/atomicfile"
	"github.com/Dangerboi/tern-kata/internal/model"
)

// attachmentStoreName is the Attachment store's folder in the primary checkout.
const attachmentStoreName = ".kata-attachments"

// attachmentStore is the Attachment store of the primary checkout at root: one folder per Issue UID, kept out of git by its own
// .gitignore. It knows nothing about kata.
type attachmentStore struct{ root string }

func (s attachmentStore) dir() string { return filepath.Join(s.root, attachmentStoreName) }

func (s attachmentStore) issueDir(uid string) string { return filepath.Join(s.dir(), uid) }

// ensure makes the Issue's folder and writes the store's .gitignore with "*" when it is missing or empty; a non-empty one is the
// user's and stays. The file is renamed into place, so a failed write never leaves an empty .gitignore behind. Two attaches that
// start together both find it missing; Windows refuses the second rename while the first holds the file, and that attach goes
// on once the .gitignore the other wrote is there.
func (s attachmentStore) ensure(uid string) error {
	if err := os.MkdirAll(s.issueDir(uid), 0o755); err != nil {
		return err
	}
	ignore := filepath.Join(s.dir(), ".gitignore")
	if info, err := os.Stat(ignore); err == nil && info.Size() > 0 {
		return nil
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	err := atomicfile.Write(ignore, []byte("*\n"), atomicfile.Options{Mode: 0o600})
	var lost *atomicfile.RenameError
	if errors.As(err, &lost) {
		if info, serr := os.Stat(ignore); serr == nil && info.Size() > 0 {
			return nil
		}
	}
	return err
}

// put copies source into the Issue's folder under the lowest free name for base that is not in taken, never overwriting a file,
// and returns that name and the copy's path. A failed copy leaves nothing behind.
func (s attachmentStore) put(ctx context.Context, uid, source, base string, taken []string) (name, path string, err error) {
	if err := s.ensure(uid); err != nil {
		return "", "", err
	}
	for n := 1; ; n++ {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		name = numberedName(base, n)
		if slices.Contains(taken, name) {
			continue
		}
		path = filepath.Join(s.issueDir(uid), name)
		if err := copyNew(source, path); errors.Is(err, fs.ErrExist) {
			continue
		} else if err != nil {
			return "", "", err
		}
		return name, path, nil
	}
}

// remove deletes the copy at path, saying what became of it. Only a file inside this Issue's folder is touched, since a
// pointer is data any writer can set; anything else is left.
func (s attachmentStore) remove(uid, path string) (model.FileFate, error) {
	rel, err := filepath.Rel(s.issueDir(uid), path)
	if err != nil || !filepath.IsAbs(path) || rel == "." || !filepath.IsLocal(rel) {
		return model.FileLeft, nil
	}
	switch err := os.Remove(path); {
	case err == nil:
		return model.FileDeleted, nil
	case errors.Is(err, fs.ErrNotExist):
		return model.FileGone, nil
	default:
		return model.FileLeft, err
	}
}

// numberedName is name for n == 1, else <stem>-n<ext>.
func numberedName(name string, n int) string {
	if n == 1 {
		return name
	}
	ext := filepath.Ext(name)
	if ext == name { // a dotfile has no stem
		ext = ""
	}
	return fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, ext), n, ext)
}

// copyNew copies source to a new file at dst, failing with fs.ErrExist if dst exists, and leaves nothing at dst on failure.
func copyNew(source, dst string) (err error) {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err = out.Close(); err != nil {
		_ = os.Remove(dst)
	}
	return err
}
