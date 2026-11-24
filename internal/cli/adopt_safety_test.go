package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/testutil"
)

func adoptSetup(t *testing.T) (remote, proj string) {
	t.Helper()
	testutil.Setup(t)
	remote = testutil.BareRemote(t)
	proj = testutil.Project(t, "demo")
	if code, out := run(t, proj, "init", "--remote", remote); code != 0 {
		t.Fatal(out)
	}
	return
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAdoptRejectsNestedGit(t *testing.T) {
	_, proj := adoptSetup(t)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, "a.md"), "A")
	testutil.Git(t, docs, "init", "-q")
	if code, _ := run(t, proj, "adopt", "docs-local"); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(docs, ".git")); err != nil {
		t.Fatal("nested repo must survive")
	}
}

func TestAdoptRejectsNestedGitFile(t *testing.T) {
	_, proj := adoptSetup(t)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, "sub", ".git"), "gitdir: /elsewhere")
	if code, _ := run(t, proj, "adopt", "docs-local"); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
}

func TestAdoptRejectsSymlink(t *testing.T) {
	_, proj := adoptSetup(t)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, "a.md"), "A")
	if err := os.Symlink("a.md", filepath.Join(docs, "link.md")); err != nil {
		t.Fatal(err)
	}
	if code, _ := run(t, proj, "adopt", "docs-local"); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	if _, err := os.Lstat(filepath.Join(docs, "link.md")); err != nil {
		t.Fatal("symlink must survive")
	}
}

func TestAdoptKeepsFilesIgnoredByInnerGitignore(t *testing.T) {
	_, proj := adoptSetup(t)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, ".gitignore"), "*.pdf\n")
	mustWrite(t, filepath.Join(docs, "paper.pdf"), "PDF")
	mustWrite(t, filepath.Join(docs, "a.md"), "A")
	code, out := run(t, proj, "adopt", "docs-local")
	if code == 0 {
		t.Fatal("expected failure")
	}
	if !contains(out, "paper.pdf") {
		t.Fatalf("error should name the file: %s", out)
	}
	if b, err := os.ReadFile(filepath.Join(docs, "paper.pdf")); err != nil || string(b) != "PDF" {
		t.Fatal("ignored file must not be deleted")
	}
}

func TestAdoptRejectsUnsafeSources(t *testing.T) {
	_, proj := adoptSetup(t)
	mustWrite(t, filepath.Join(proj, "keep.md"), "K")
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "x.md"), "X")
	for _, arg := range []string{"..", ".", "/", outside} {
		if code, out := run(t, proj, "adopt", arg); code != 2 {
			t.Fatalf("adopt %q: want 2, got %d (%s)", arg, code, out)
		}
	}
	sub := filepath.Join(proj, "sub")
	_ = os.MkdirAll(sub, 0o755)
	if code, _ := run(t, sub, "adopt", ".."); code != 2 {
		t.Fatal("adopt .. from sub must be 2")
	}
	for _, f := range []string{filepath.Join(proj, "keep.md"), filepath.Join(outside, "x.md")} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("%s must be intact", f)
		}
	}
}

func TestAdoptRejectsInsideIncognito(t *testing.T) {
	_, proj := adoptSetup(t)
	if code, out := run(t, proj, "link"); code != 0 {
		t.Fatal(out)
	}
	mustWrite(t, filepath.Join(proj, ".incognito", "n", "a.md"), "A")
	for _, arg := range []string{".incognito", ".incognito/n"} {
		if code, _ := run(t, proj, "adopt", arg); code != 2 {
			t.Fatalf("adopt %s: want 2, got %d", arg, code)
		}
	}
	if _, err := os.Stat(filepath.Join(proj, ".incognito", "n", "a.md")); err != nil {
		t.Fatal("notes must be intact")
	}
}
