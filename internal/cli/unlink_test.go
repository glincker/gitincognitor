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
