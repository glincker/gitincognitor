package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/store"
	"github.com/glincker/gitincognito/internal/testutil"
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
	if err := store.AddWorktree(dir, "proj/a", wt); err != nil {
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
	if err := store.AddWorktree(dir2, "proj/a", wt2); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(wt2, "n.md")); err != nil || string(b) != "hi" {
		t.Fatalf("restore failed: %q %v", b, err)
	}
}
