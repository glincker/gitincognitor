package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/project"
	"github.com/glincker/gitincognito/internal/testutil"
)

// Crash drill: lose the store and the project's .git, then get the notes back.
func TestRestoreAfterTotalLocalLoss(t *testing.T) {
	home, remote, proj := linked(t)
	writeNote(t, proj, "plan.md", "keep me")
	if code, out := run(t, proj, "sync"); code != 0 {
		t.Fatal(out)
	}
	if err := os.RemoveAll(home); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(proj); err != nil {
		t.Fatal(err)
	}
	// A fresh clone of the project appears at the same path.
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Git(t, proj, "init", "-q", "-b", "main")

	if code, out := run(t, proj, "restore", "--all", "--remote", remote); code != 0 {
		t.Fatal(out)
	}
	b, err := os.ReadFile(filepath.Join(proj, ".incognito", "plan.md"))
	if err != nil || string(b) != "keep me" {
		t.Fatalf("note not restored: %q %v", b, err)
	}
	if !project.Excluded(proj, ".incognito/") {
		t.Fatal("exclude not restored")
	}
	if got := testutil.Git(t, proj, "status", "--porcelain"); got != "" {
		t.Fatalf("outer repo dirty: %q", got)
	}
}

func TestRestoreSkipsMissingProjectPath(t *testing.T) {
	home, remote, proj := linked(t)
	if err := os.RemoveAll(proj); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(home, "store", ".git", "worktrees")); err != nil {
		t.Fatal(err)
	}
	code, out := run(t, t.TempDir(), "restore", "--all", "--remote", remote, "--json")
	if code != 0 {
		t.Fatal(out)
	}
	if !contains(out, "path missing") {
		t.Fatalf("want skip message: %s", out)
	}
}
