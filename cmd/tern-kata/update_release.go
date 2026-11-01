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
	"strings"

	"golang.org/x/mod/semver"
)

// maxDownload bounds one downloaded file, so a wrong server cannot fill the disk.
const maxDownload = 512 << 20

// unpackedRelease is a downloaded, checked and unpacked release. Dir holds what the archive holds: the binary, LICENSE and
// plugin/.
type unpackedRelease struct {
	Version string
	Dir     string
	Binary  string
}

// binaryName is the file the archive holds the binary as.
func (e updateEnv) binaryName() string {
	if e.GOOS == "windows" {
		return "tern-kata.exe"
	}
	return "tern-kata"
}

// archiveName is the OS archive, as scripts/release.sh names it.
func (e updateEnv) archiveName() string {
	if e.GOOS == "windows" {
		return "tern-kata_" + e.GOOS + "_" + e.GOARCH + ".zip"
	}
	return "tern-kata_" + e.GOOS + "_" + e.GOARCH + ".tar.gz"
}

// versionURL is version.txt of the latest release, or of the dist/ TERN_KATA_BASE_URL names.
func (e updateEnv) versionURL() string {
	if e.BaseURL != "" {
		return strings.TrimRight(e.BaseURL, "/") + "/version.txt"
	}
	return latestBase + "/version.txt"
}

// fileURL is name in the folder of release version, or in the dist/ TERN_KATA_BASE_URL names.
func (e updateEnv) fileURL(version, name string) string {
	if e.BaseURL != "" {
		return strings.TrimRight(e.BaseURL, "/") + "/" + name
	}
	return releaseBase + "/" + version + "/" + name
}

// fetchLatestVersion reads the latest release's version from version.txt, as the semver "vX.Y.Z".
func fetchLatestVersion(ctx context.Context, env updateEnv) (string, error) {
	var text strings.Builder
	if err := download(ctx, env.Client, env.versionURL(), &text, 1<<10); err != nil {
		return "", fmt.Errorf("cannot read the latest version: %w", err)
	}
	latest := "v" + strings.TrimPrefix(strings.TrimSpace(text.String()), "v")
	if !semver.IsValid(latest) {
		return "", fmt.Errorf("%s holds %q, which is not a version number", env.versionURL(), strings.TrimSpace(text.String()))
	}
	return latest, nil
}

// downloadRelease downloads release version's archive for this OS and its checksums.txt into work, checks the archive's
// SHA-256 against its checksums.txt line and unpacks it into work/unpacked.
func downloadRelease(ctx context.Context, env updateEnv, version, work string) (unpackedRelease, error) {
	name := env.archiveName()
	archive := filepath.Join(work, name)
	if err := downloadFile(ctx, env, env.fileURL(version, name), archive); err != nil {
		return unpackedRelease{}, err
	}
	var sums strings.Builder
	if err := download(ctx, env.Client, env.fileURL(version, "checksums.txt"), &sums, 1<<20); err != nil {
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
	return unpackedRelease{Version: version, Dir: dir, Binary: binary}, nil
}

// download writes the body of url to w, refusing more than limit bytes and any status but 200.
func download(ctx context.Context, client *http.Client, url string, w io.Writer, limit int64) error {
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
	return download(ctx, env.Client, url, file, maxDownload)
}

// verifyChecksum checks that file's SHA-256 is the one sums (the text of a checksums.txt) lists for name.
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
