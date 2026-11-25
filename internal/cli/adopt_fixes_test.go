package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glincker/gitincognitor/internal/cli"
	"github.com/glincker/gitincognitor/internal/testutil"
)

func TestAdoptKeepsFileEditedAfterCopy(t *testing.T) {
	_, proj := adoptSetup(t)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, "a.md"), "A")
	mustWrite(t, filepath.Join(docs, "b.md"), "B")
	cli.SetAdoptAfterSync(func() { mustWrite(t, filepath.Join(docs, "a.md"), "A edited") })
	t.Cleanup(func() { cli.SetAdoptAfterSync(nil) })
	code, out := run(t, proj, "adopt", "docs-local")
	if code != 1 || !strings.Contains(out, "a.md") {
		t.Fatalf("want 1 naming a.md, got %d: %s", code, out)
	}
	if b, _ := os.ReadFile(filepath.Join(docs, "a.md")); string(b) != "A edited" {
		t.Fatalf("edited file must be kept: %q", b)
	}
	if exists(filepath.Join(docs, "b.md")) {
		t.Fatal("unchanged file should still be removed")
	}
}

func breakRemote(t *testing.T, home string) string {
	t.Helper()
	st := filepath.Join(home, "store")
	old := testutil.Git(t, st, "remote", "get-url", "origin")
	testutil.Git(t, st, "remote", "set-url", "origin", "/nonexistent/x.git")
	return old
}

func TestAdoptRerunAfterFailedPushCompletes(t *testing.T) {
	home := testutil.Setup(t)
	remote := testutil.BareRemote(t)
	proj := testutil.Project(t, "demo")
	run(t, proj, "init", "--remote", remote)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, "a.md"), "A")
	old := breakRemote(t, home)
	if code, _ := run(t, proj, "adopt", "docs-local"); code == 0 {
		t.Fatal("expected failure")
	}
	testutil.Git(t, filepath.Join(home, "store"), "remote", "set-url", "origin", old)
	if code, out := run(t, proj, "adopt", "docs-local"); code != 0 {
		t.Fatal(code, out)
	}
	if exists(filepath.Join(docs, "a.md")) {
		t.Fatal("rerun must finish the deletion")
	}
	if testutil.Git(t, remote, "branch", "--list", "proj/demo") == "" {
		t.Fatal("notes not pushed")
	}
}

func TestAdoptRerunWithBrokenRemoteDeletesNothing(t *testing.T) {
	home := testutil.Setup(t)
	remote := testutil.BareRemote(t)
	proj := testutil.Project(t, "demo")
	run(t, proj, "init", "--remote", remote)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, "a.md"), "A")
	breakRemote(t, home)
	run(t, proj, "adopt", "docs-local")
	if code, _ := run(t, proj, "adopt", "docs-local"); code == 0 {
		t.Fatal("rerun must fail while the push is broken")
	}
	if !exists(filepath.Join(docs, "a.md")) {
		t.Fatal("nothing may be deleted")
	}
}

func TestAdoptEverythingIgnoredWithUnrelatedDest(t *testing.T) {
	_, proj := adoptSetup(t)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, ".incognitoignore"), "*\n")
	mustWrite(t, filepath.Join(docs, "x.md"), "X")
	run(t, proj, "link")
	mustWrite(t, filepath.Join(proj, ".incognito", "docs-local", "other.md"), "O")
	code, out := run(t, proj, "adopt", "docs-local")
	if code != 2 || !strings.Contains(out, "rerun") {
		t.Fatalf("want 2 with cleanup hint, got %d: %s", code, out)
	}
}

func TestAdoptPartialDestIsUnsafeWithHint(t *testing.T) {
	_, proj := adoptSetup(t)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, "a.md"), "A")
	mustWrite(t, filepath.Join(docs, "b.md"), "B")
	run(t, proj, "link")
	mustWrite(t, filepath.Join(proj, ".incognito", "docs-local", "a.md"), "A")
	code, out := run(t, proj, "adopt", "docs-local")
	if code != 2 || !strings.Contains(out, ".incognito/docs-local") || !strings.Contains(out, "rerun") {
		t.Fatalf("got %d: %s", code, out)
	}
	if !exists(filepath.Join(docs, "a.md")) {
		t.Fatal("source must survive")
	}
}

func TestAdoptDryRunParity(t *testing.T) {
	t.Run("tracked path", func(t *testing.T) {
		testutil.Setup(t)
		proj := testutil.Project(t, "demo")
		mustWrite(t, filepath.Join(proj, "docs", "a.md"), "A")
		testutil.Git(t, proj, "add", "docs")
		if code, out := run(t, proj, "adopt", "--dry-run", "docs"); code != 2 || !strings.Contains(out, "tracked") {
			t.Fatal(code, out)
		}
	})
	t.Run("dest differs", func(t *testing.T) {
		testutil.Setup(t)
		proj := testutil.Project(t, "demo")
		mustWrite(t, filepath.Join(proj, "docs-local", "a.md"), "A")
		mustWrite(t, filepath.Join(proj, ".incognito", "docs-local", "a.md"), "different")
		if code, out := run(t, proj, "adopt", "--dry-run", "docs-local"); code != 2 || !strings.Contains(out, "already exists") {
			t.Fatal(code, out)
		}
	})
	t.Run("tracked pointer", func(t *testing.T) {
		testutil.Setup(t)
		proj := testutil.Project(t, "demo")
		mustWrite(t, filepath.Join(proj, "CLAUDE.local.md"), "p")
		testutil.Git(t, proj, "add", "CLAUDE.local.md")
		mustWrite(t, filepath.Join(proj, "docs-local", "a.md"), "A")
		if code, out := run(t, proj, "adopt", "--dry-run", "docs-local"); code != 2 || !strings.Contains(out, "CLAUDE.local.md") {
			t.Fatal(code, out)
		}
	})
}

func TestAdoptSymlinkSwappedBeforeCopyIsRefused(t *testing.T) {
	_, proj := adoptSetup(t)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, "a.md"), "A")
	secret := filepath.Join(t.TempDir(), "secret.txt")
	mustWrite(t, secret, "TOPSECRET")
	cli.SetAdoptBeforeCopy(func() {
		_ = os.Remove(filepath.Join(docs, "a.md"))
		if err := os.Symlink(secret, filepath.Join(docs, "a.md")); err != nil {
			t.Error(err)
		}
	})
	t.Cleanup(func() { cli.SetAdoptBeforeCopy(nil) })
	if code, out := run(t, proj, "adopt", "docs-local"); code != 2 {
		t.Fatalf("want 2, got %d: %s", code, out)
	}
	_ = filepath.WalkDir(filepath.Join(proj, ".incognito"), func(p string, d os.DirEntry, _ error) error {
		if d != nil && d.Type().IsRegular() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), "TOPSECRET") {
				t.Fatalf("outside content leaked into %s", p)
			}
		}
		return nil
	})
}

func TestAdoptIgnoresSymlinkToDirWithDirOnlyRule(t *testing.T) {
	_, proj := adoptSetup(t)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, "a.md"), "A")
	mustWrite(t, filepath.Join(docs, ".incognitoignore"), "ref/\n")
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(docs, "ref")); err != nil {
		t.Fatal(err)
	}
	if code, out := run(t, proj, "adopt", "docs-local"); code != 0 {
		t.Fatal(code, out)
	}
	if fi, err := os.Lstat(filepath.Join(docs, "ref")); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink must stay in place")
	}
}
