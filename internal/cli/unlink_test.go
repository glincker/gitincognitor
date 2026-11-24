package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/project"
	"github.com/glincker/gitincognito/internal/registry"
	"github.com/glincker/gitincognito/internal/store"
	"github.com/glincker/gitincognito/internal/testutil"
)

func TestUnlinkRemovesWorktreeKeepsBranch(t *testing.T) {
	home, _, proj := linked(t)
	writeNote(t, proj, "a.md", "x")
	run(t, proj, "sync")
	if code, out := run(t, proj, "unlink"); code != 0 {
		t.Fatal(out)
	}
	if _, err := os.Stat(filepath.Join(proj, ".incognito")); !os.IsNotExist(err) {
		t.Fatal("worktree should be gone")
	}
	if project.Excluded(proj, ".incognito/") {
		t.Fatal("exclude should be removed")
	}
	if testutil.Git(t, filepath.Join(home, "store"), "branch", "--list", "proj/demo") == "" {
		t.Fatal("branch must be kept")
	}
	reg, _ := registry.Load(store.RegistryPath(home))
	if reg.Find("demo") != nil {
		t.Fatal("registry entry should be removed")
	}
}

func TestUnlinkRefusesDirtyNotes(t *testing.T) {
	_, _, proj := linked(t)
	writeNote(t, proj, "a.md", "unsynced")
	if code, _ := run(t, proj, "unlink"); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	if code, out := run(t, proj, "unlink", "--force"); code != 0 {
		t.Fatal(out)
	}
}

func TestUnlinkCleansUpWhenWorktreeDeletedByHand(t *testing.T) {
	home, _, proj := linked(t)
	if err := os.RemoveAll(filepath.Join(proj, ".incognito")); err != nil {
		t.Fatal(err)
	}
	if code, out := run(t, proj, "unlink"); code != 0 {
		t.Fatal(out)
	}
	if project.Excluded(proj, ".incognito/") {
		t.Fatal("exclude should be removed")
	}
	reg, _ := registry.Load(store.RegistryPath(home))
	if reg.Find("demo") != nil {
		t.Fatal("registry entry should be removed")
	}
}

func TestUnlinkRefusesIgnoredFilesInNotes(t *testing.T) {
	_, _, proj := linked(t)
	writeNote(t, proj, ".gitignore", "*.pdf\n")
	run(t, proj, "sync")
	writeNote(t, proj, "paper.pdf", "PDF")
	if code, _ := run(t, proj, "unlink"); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(proj, ".incognito", "paper.pdf")); err != nil {
		t.Fatal("ignored file must survive")
	}
}

func TestUnlinkRefusesUnpushedCommit(t *testing.T) {
	_, _, proj := linked(t)
	writeNote(t, proj, "a.md", "x")
	run(t, proj, "sync")
	writeNote(t, proj, "b.md", "y")
	wt := filepath.Join(proj, ".incognito")
	testutil.Git(t, wt, "add", "-A")
	testutil.Git(t, wt, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "local")
	if code, _ := run(t, proj, "unlink"); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	if code, out := run(t, proj, "unlink", "--force"); code != 0 {
		t.Fatal(out)
	}
}
