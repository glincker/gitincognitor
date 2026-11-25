package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glincker/gitincognitor/internal/store"
	"github.com/glincker/gitincognitor/internal/testutil"
)

func TestInitPushesSeedMainAndIsIdempotent(t *testing.T) {
	testutil.Setup(t)
	remote := testutil.BareRemote(t)
	dir := filepath.Join(t.TempDir(), "store")
	for i := 0; i < 2; i++ {
		if err := store.Init(dir, remote, "registry.yml", "version: 1\n"); err != nil {
			t.Fatal(err)
		}
	}
	if got := testutil.Git(t, remote, "branch", "--list", "main"); got == "" {
		t.Fatal("seed main not pushed")
	}
}

func TestAddWorktreeOrphanThenRestoreFromRemote(t *testing.T) {
	testutil.Setup(t)
	remote := testutil.BareRemote(t)
	dir := filepath.Join(t.TempDir(), "store")
	if err := store.Init(dir, remote, "registry.yml", "version: 1\n"); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(t.TempDir(), "wt")
	if err := store.AddWorktree(dir, "proj/a", wt, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "n.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Git(t, wt, "add", "-A")
	testutil.Git(t, wt, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "n")
	testutil.Git(t, wt, "push", "-q", "-u", "origin", "proj/a")

	// Simulate a new machine: clone, then add the worktree from the remote branch.
	dir2 := filepath.Join(t.TempDir(), "store2")
	if err := store.Clone(dir2, remote); err != nil {
		t.Fatal(err)
	}
	wt2 := filepath.Join(t.TempDir(), "wt2")
	if err := store.AddWorktree(dir2, "proj/a", wt2, false); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(wt2, "n.md")); err != nil || string(b) != "hi" {
		t.Fatalf("restore failed: %q %v", b, err)
	}
}

func TestRemotesStartingWithDashAreRejected(t *testing.T) {
	testutil.Setup(t)
	dir := filepath.Join(t.TempDir(), "store")
	if err := store.Init(dir, "--upload-pack=touch /tmp/x", "registry.yml", "v\n"); err == nil {
		t.Fatal("Init must reject a dash remote")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("nothing may be created")
	}
	if err := store.Clone(filepath.Join(t.TempDir(), "c"), "-x"); err == nil {
		t.Fatal("Clone must reject a dash remote")
	}
}

func TestAddWorktreeNeverInventsMissingBranch(t *testing.T) {
	testutil.Setup(t)
	remote := testutil.BareRemote(t)
	dir := filepath.Join(t.TempDir(), "store")
	if err := store.Init(dir, remote, "registry.yml", "v\n"); err != nil {
		t.Fatal(err)
	}
	// Reachable remote, branch absent: restore must not create a fresh orphan.
	if err := store.AddWorktree(dir, "proj/none", filepath.Join(t.TempDir(), "w1"), false); err == nil {
		t.Fatal("want error for missing branch")
	}
	// Unreachable remote: link may still start a new branch, restore may not.
	testutil.Git(t, dir, "remote", "set-url", "origin", "/nonexistent/x.git")
	err := store.AddWorktree(dir, "proj/none", filepath.Join(t.TempDir(), "w2"), false)
	if err == nil || !strings.Contains(err.Error(), "cannot reach the store remote and branch is missing") {
		t.Fatalf("want unreachable error, got %v", err)
	}
	if err := store.AddWorktree(dir, "proj/none", filepath.Join(t.TempDir(), "w3"), true); err != nil {
		t.Fatalf("new branch allowed offline: %v", err)
	}
}
