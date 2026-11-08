//go:build windows

package docbanksetup

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.kenn.io/kit/safefileio"
	"golang.org/x/sys/windows"
)

// sharedDir is a new folder whose access list lets Everyone read whatever is created in it, through an inherited grant, as the
// folder of the user who reported the bug let CodexSandboxUsers in.
func sharedDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	world, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		t.Fatal(err)
	}
	current, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := current.DACL()
	if err != nil {
		t.Fatal(err)
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.FILE_GENERIC_READ,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(world),
		},
	}}, dacl)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
	return dir
}

// katasCheck is the check kata runs on a key file on Windows before it reads it.
func katasCheck(t *testing.T, path string) error {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	return safefileio.ValidatePrivateCurrentUserFile(f)
}

// plainFile is a file written the ordinary way into dir, which takes the folder's inherited access list.
func plainFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if katasCheck(t, path) == nil {
		t.Fatal("a plain file in the shared folder passed kata's check, so the folder shares nothing")
	}
	return path
}

func TestAKeyFileWrittenIntoAFolderOthersCanReadPassesKatasCheck(t *testing.T) {
	dir := sharedDir(t)
	plainFile(t, dir, "plain", "x")
	keyPath := filepath.Join(dir, KeyFileName)
	var out strings.Builder
	if err := (Env{Out: &out}).writeKeyFile(keyPath, "k"); err != nil {
		t.Fatal(err)
	}
	if err := katasCheck(t, keyPath); err != nil {
		t.Errorf("kata would refuse the key file setup wrote: %v", err)
	}
	if !strings.Contains(out.String(), "readable only by you") || strings.Contains(out.String(), "0600") {
		t.Errorf("output does not say what was set: %q", out.String())
	}
}

func TestAKeyFileKataWouldRefuseIsWrittenAgainAndSaidSo(t *testing.T) {
	keyPath := plainFile(t, sharedDir(t), KeyFileName, "k\n")
	var out strings.Builder
	if err := (Env{Out: &out}).writeKeyFile(keyPath, "k"); err != nil {
		t.Fatal(err)
	}
	if err := katasCheck(t, keyPath); err != nil {
		t.Errorf("the key file still fails kata's check: %v", err)
	}
	if !strings.Contains(out.String(), "kata would have refused") || !strings.Contains(out.String(), "must be a private current-user file") {
		t.Errorf("output does not say the file was written again and why: %q", out.String())
	}
}

func TestSetupRemoteRefusesAKeyFileOthersCanRead(t *testing.T) {
	keyPath := plainFile(t, sharedDir(t), "shared.key", "k\n")
	e := Env{KataHome: t.TempDir(), Out: io.Discard, Runner: noRunRunner{t}}
	err := e.SetupRemote(context.Background(), Remote{URL: "https://docbank.example", KeyFile: keyPath})
	if err == nil || !strings.Contains(err.Error(), "must be a private current-user file") || !strings.Contains(err.Error(), "--key-file -") {
		t.Fatalf("SetupRemote = %v; want kata's reason and the way out", err)
	}
}
