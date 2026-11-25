package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognitor/internal/testutil"
)

func TestAdoptMovesFolderIntoNotes(t *testing.T) {
	testutil.Setup(t)
	remote := testutil.BareRemote(t)
	proj := testutil.Project(t, "demo")
	run(t, proj, "init", "--remote", remote)
	docs := filepath.Join(proj, "docs-local")
	_ = os.MkdirAll(filepath.Join(docs, "sub"), 0o755)
	_ = os.WriteFile(filepath.Join(docs, "a.md"), []byte("A"), 0o644)
	_ = os.WriteFile(filepath.Join(docs, "sub", "b.md"), []byte("B"), 0o644)

	if code, out := run(t, proj, "adopt", "docs-local"); code != 0 {
		t.Fatal(out)
	}
	if _, err := os.Stat(docs); !os.IsNotExist(err) {
		t.Fatal("original folder should be gone after a verified push")
	}
	b, err := os.ReadFile(filepath.Join(proj, ".incognito", "docs-local", "sub", "b.md"))
	if err != nil || string(b) != "B" {
		t.Fatalf("not moved: %q %v", b, err)
	}
	if testutil.Git(t, remote, "branch", "--list", "proj/demo") == "" {
		t.Fatal("notes not pushed")
	}
}

func TestAdoptKeepsOriginalWhenPushFails(t *testing.T) {
	home := testutil.Setup(t)
	remote := testutil.BareRemote(t)
	proj := testutil.Project(t, "demo")
	run(t, proj, "init", "--remote", remote)
	testutil.Git(t, filepath.Join(home, "store"), "remote", "set-url", "origin", "/nonexistent/x.git")
	docs := filepath.Join(proj, "docs-local")
	_ = os.MkdirAll(docs, 0o755)
	_ = os.WriteFile(filepath.Join(docs, "a.md"), []byte("A"), 0o644)
	if code, _ := run(t, proj, "adopt", "docs-local"); code == 0 {
		t.Fatal("expected failure")
	}
	if _, err := os.Stat(filepath.Join(docs, "a.md")); err != nil {
		t.Fatal("original must survive a failed push")
	}
}

func TestAdoptRefusesTrackedFolder(t *testing.T) {
	testutil.Setup(t)
	remote := testutil.BareRemote(t)
	proj := testutil.Project(t, "demo")
	run(t, proj, "init", "--remote", remote)
	_ = os.MkdirAll(filepath.Join(proj, "docs"), 0o755)
	_ = os.WriteFile(filepath.Join(proj, "docs", "a.md"), []byte("A"), 0o644)
	testutil.Git(t, proj, "add", "docs")
	if code, _ := run(t, proj, "adopt", "docs"); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
}

func TestAdoptWorksInPlainWorkspaceFolder(t *testing.T) {
	testutil.Setup(t)
	remote := testutil.BareRemote(t)
	ws := t.TempDir()
	run(t, ws, "init", "--remote", remote)
	_ = os.MkdirAll(filepath.Join(ws, "docs-local"), 0o755)
	_ = os.WriteFile(filepath.Join(ws, "docs-local", "a.md"), []byte("A"), 0o644)
	if code, out := run(t, ws, "adopt", "docs-local"); code != 0 {
		t.Fatal(out)
	}
	if _, err := os.Stat(filepath.Join(ws, ".incognito", "docs-local", "a.md")); err != nil {
		t.Fatal(err)
	}
}
