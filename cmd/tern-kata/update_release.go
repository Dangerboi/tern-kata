package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/Dangerboi/tern-kata/internal/progress"
)

// maxDownload bounds one downloaded file, so a wrong server cannot fill the disk.
const maxDownload = 512 << 20

type unpackedRelease struct {
	Dir    string
	Binary string
}

func (e updateEnv) binaryName() string {
	if runtime.GOOS == "windows" {
		return "tern-kata.exe"
	}
	return "tern-kata"
}

// archiveName is the OS archive, as scripts/release.sh names it.
func (e updateEnv) archiveName() string {
	if runtime.GOOS == "windows" {
		return "tern-kata_" + runtime.GOOS + "_" + runtime.GOARCH + ".zip"
	}
	return "tern-kata_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
}

func (e updateEnv) versionURL() string {
	if e.BaseURL != "" {
		return strings.TrimRight(e.BaseURL, "/") + "/version.txt"
	}
	return latestBase + "/version.txt"
}

func (e updateEnv) fileURL(version, name string) string {
	if e.BaseURL != "" {
		return strings.TrimRight(e.BaseURL, "/") + "/" + name
	}
	return releaseBase + "/" + version + "/" + name
}

func fetchLatestVersion(ctx context.Context, env updateEnv) (string, error) {
	var text strings.Builder
	if err := download(ctx, env.Client, env.versionURL(), &text, 1<<10, nil); err != nil {
		return "", fmt.Errorf("cannot read the latest version: %w", err)
	}
	latest := "v" + strings.TrimPrefix(strings.TrimSpace(text.String()), "v")
	if !semver.IsValid(latest) {
		return "", fmt.Errorf("%s holds %q, which is not a version number", env.versionURL(), strings.TrimSpace(text.String()))
	}
	return latest, nil
}

func downloadRelease(ctx context.Context, env updateEnv, version, work string) (unpackedRelease, error) {
	name := env.archiveName()
	archive := filepath.Join(work, name)
	if err := downloadFile(ctx, env, env.fileURL(version, name), archive); err != nil {
		return unpackedRelease{}, err
	}
	env.Progress.Working(progress.Unknown) // the checksum, unpacking and the swap have no size
	var sums strings.Builder
	if err := download(ctx, env.Client, env.fileURL(version, "checksums.txt"), &sums, 1<<20, nil); err != nil {
		return unpackedRelease{}, err
	}
	if err := verifyChecksum(archive, name, sums.String()); err != nil {
		return unpackedRelease{}, err
	}
	dir := filepath.Join(work, "unpacked")
	if err := unpack(archive, dir, env.binaryName()); err != nil {
		return unpackedRelease{}, fmt.Errorf("cannot unpack %s: %w", name, err)
	}
	binary := filepath.Join(dir, env.binaryName())
	if info, err := os.Stat(binary); err != nil || !info.Mode().IsRegular() {
		return unpackedRelease{}, fmt.Errorf("%s holds no %s", name, env.binaryName())
	}
	return unpackedRelease{Dir: dir, Binary: binary}, nil
}

// download copies url to w, at most limit bytes. onBytes, when not nil, is told the bytes copied so far and the size the server
// announced (0 when it did not).
func download(ctx context.Context, client *http.Client, url string, w io.Writer, limit int64, onBytes func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", url, resp.Status)
	}
	if onBytes != nil {
		w = &countingWriter{w: w, total: max(resp.ContentLength, 0), onBytes: onBytes}
	}
	n, err := io.Copy(w, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return fmt.Errorf("reading %s: %w", url, err)
	}
	if n > limit {
		return fmt.Errorf("%s is larger than %d bytes", url, limit)
	}
	return nil
}

func downloadFile(ctx context.Context, env updateEnv, url, path string) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	env.Progress.Working(progress.Unknown) // until the server's answer says how big the file is
	return download(ctx, env.Client, url, file, maxDownload, func(done, total int64) {
		if total > 0 {
			env.Progress.Working(int(min(done, total) * 100 / total))
		}
	})
}

// countingWriter tells onBytes how much has been written after every write.
type countingWriter struct {
	w           io.Writer
	done, total int64
	onBytes     func(done, total int64)
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.done += int64(n)
	c.onBytes(c.done, c.total)
	return n, err
}

func verifyChecksum(file, name, sums string) error {
	want := ""
	for line := range strings.SplitSeq(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			want = strings.ToLower(fields[0])
		}
	}
	if want == "" {
		return fmt.Errorf("checksums.txt has no line for %s", name)
	}
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("the downloaded %s does not match its checksum in checksums.txt", name)
	}
	return nil
}

// unpack extracts every file of the .zip or .tar.gz archive into dir; the binary gets mode 0755, the rest 0644. A path that
// leaves dir is refused.
func unpack(archive, dir, binary string) error {
	write := func(name string, r io.Reader) error {
		if !filepath.IsLocal(filepath.FromSlash(name)) {
			return fmt.Errorf("%q is outside the archive's folder", name)
		}
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if name == binary {
			mode = 0o755
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, r)
		return errors.Join(err, f.Close())
	}
	if strings.HasSuffix(archive, ".zip") {
		zr, err := zip.OpenReader(archive)
		if err != nil {
			return err
		}
		defer func() { _ = zr.Close() }()
		for _, entry := range zr.File {
			if entry.FileInfo().IsDir() {
				continue
			}
			r, err := entry.Open()
			if err != nil {
				return err
			}
			err = write(entry.Name, r)
			_ = r.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if err := write(header.Name, tr); err != nil {
			return err
		}
	}
}
