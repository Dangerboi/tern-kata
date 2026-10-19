package data

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func primaryCheckout(t *testing.T, workspace string) string {
	t.Helper()
	got, err := PrimaryCheckoutRoot(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func sameDir(t *testing.T, got, want string) {
	t.Helper()
	g, _ := filepath.EvalSymlinks(got)
	w, _ := filepath.EvalSymlinks(want)
	if g != w {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestPrimaryCheckoutOfALinkedWorktreeIsTheMainWorktree(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	worktree := filepath.Join(t.TempDir(), "linked")
	git(t, repo, "worktree", "add", "-q", worktree)
	sameDir(t, primaryCheckout(t, worktree), repo)
	sub := filepath.Join(worktree, "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	sameDir(t, primaryCheckout(t, sub), repo)
}

func TestPrimaryCheckoutOutsideGitIsTheDirectoryHoldingTheBinding(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, workspace, ".kata.toml", "")
	sub := filepath.Join(workspace, "deep", "er")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	sameDir(t, primaryCheckout(t, sub), workspace)
	if _, err := PrimaryCheckoutRoot(context.Background(), t.TempDir()); err == nil {
		t.Fatal("a folder with no repository and no binding has no primary checkout")
	}
}

func TestPrimaryCheckoutOfBareRepositoryWorktreesIsTheirParent(t *testing.T) {
	root := t.TempDir()
	seed := t.TempDir()
	git(t, seed, "init", "-q")
	git(t, seed, "commit", "-q", "--allow-empty", "-m", "init")
	bare := filepath.Join(root, ".bare")
	git(t, root, "clone", "-q", "--bare", seed, bare)
	main, other := filepath.Join(root, "main"), filepath.Join(root, "other")
	git(t, bare, "worktree", "add", "-q", main)
	git(t, bare, "worktree", "add", "-q", "-b", "side", other)
	for _, workspace := range []string{main, other} {
		sameDir(t, primaryCheckout(t, workspace), root)
	}
}

func TestPrimaryCheckoutOfASubmoduleIsItsOwnTopLevel(t *testing.T) {
	inner := t.TempDir()
	git(t, inner, "init", "-q")
	git(t, inner, "commit", "-q", "--allow-empty", "-m", "init")
	outer := t.TempDir()
	git(t, outer, "init", "-q")
	git(t, outer, "-c", "protocol.file.allow=always", "submodule", "add", "-q", inner, "sub")
	sub := filepath.Join(outer, "sub")
	sameDir(t, primaryCheckout(t, sub), sub)
}
