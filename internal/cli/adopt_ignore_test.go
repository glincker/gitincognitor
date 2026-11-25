package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glincker/gitincognito/internal/testutil"
)

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func TestAdoptIgnoredNestedCloneStays(t *testing.T) {
	_, proj := adoptSetup(t)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, "a.md"), "A")
	mustWrite(t, filepath.Join(docs, ".incognitoignore"), "ref/\n")
	mustWrite(t, filepath.Join(docs, "ref", "r.md"), "R")
	testutil.Git(t, filepath.Join(docs, "ref"), "init", "-q")
	if code, out := run(t, proj, "adopt", "docs-local"); code != 0 {
		t.Fatal(out)
	}
	if !exists(filepath.Join(docs, "ref", ".git")) || !exists(filepath.Join(docs, "ref", "r.md")) {
		t.Fatal("ignored clone must stay untouched")
	}
	if exists(filepath.Join(docs, "a.md")) {
		t.Fatal("copied note must be removed from src")
	}
	n := filepath.Join(proj, ".incognito", "docs-local")
	if !exists(filepath.Join(n, "a.md")) || exists(filepath.Join(n, "ref")) {
		t.Fatal("note copied, ignored dir not copied")
	}
}

func TestAdoptNonIgnoredNestedGitStillRefused(t *testing.T) {
	_, proj := adoptSetup(t)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, ".incognitoignore"), "other/\n")
	mustWrite(t, filepath.Join(docs, "ref", "r.md"), "R")
	testutil.Git(t, filepath.Join(docs, "ref"), "init", "-q")
	if code, _ := run(t, proj, "adopt", "docs-local"); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	if !exists(filepath.Join(docs, "ref", ".git")) {
		t.Fatal("repo must survive")
	}
}

func TestAdoptIgnoredHugeFileNotCopied(t *testing.T) {
	_, proj := adoptSetup(t)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, "a.md"), "A")
	mustWrite(t, filepath.Join(docs, ".incognitoignore"), "big/\n")
	mustWrite(t, filepath.Join(docs, "big", "blob.bin"), strings.Repeat("x", 2<<20))
	if code, out := run(t, proj, "adopt", "docs-local"); code != 0 {
		t.Fatal(out)
	}
	if !exists(filepath.Join(docs, "big", "blob.bin")) {
		t.Fatal("ignored file must stay")
	}
	if exists(filepath.Join(proj, ".incognito", "docs-local", "big")) {
		t.Fatal("ignored dir must not be copied")
	}
}

func TestAdoptLeavesOnlyIgnoredAndRulesFile(t *testing.T) {
	_, proj := adoptSetup(t)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, ".incognitoignore"), "keepme/\n")
	mustWrite(t, filepath.Join(docs, "keepme", "k.txt"), "K")
	mustWrite(t, filepath.Join(docs, "x", "y", "n.md"), "N")
	mustWrite(t, filepath.Join(docs, "mixed", "n.md"), "N")
	mustWrite(t, filepath.Join(docs, "mixed", "keepme", "k.txt"), "K")
	if code, out := run(t, proj, "adopt", "docs-local"); code != 0 {
		t.Fatal(out)
	}
	var left []string
	_ = filepath.WalkDir(docs, func(p string, d os.DirEntry, _ error) error {
		if !d.IsDir() {
			r, _ := filepath.Rel(docs, p)
			left = append(left, r)
		}
		return nil
	})
	got := strings.Join(left, ",")
	if got != ".incognitoignore,keepme/k.txt,mixed/keepme/k.txt" {
		t.Fatalf("left: %s", got)
	}
	if exists(filepath.Join(docs, "x")) {
		t.Fatal("empty dirs must be removed")
	}
	n := filepath.Join(proj, ".incognito", "docs-local")
	if !exists(filepath.Join(n, ".incognitoignore")) {
		t.Fatal("rules file must be copied into the notes")
	}
}

func TestAdoptDryRunWritesNothing(t *testing.T) {
	home := testutil.Setup(t) // no init: dry-run must work before init
	proj := testutil.Project(t, "demo")
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, "a.md"), "AAAA")
	mustWrite(t, filepath.Join(docs, ".incognitoignore"), "big/\n")
	mustWrite(t, filepath.Join(docs, "big", "blob"), strings.Repeat("x", 1000))
	code, out := run(t, proj, "adopt", "--dry-run", "--json", "docs-local")
	if code != 0 {
		t.Fatal(code, out)
	}
	var r struct {
		Copy, Skipped struct{ Files, Bytes int64 }
		IgnoredTop    []string `json:"ignored_top"`
		Refusals      []string
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err, out)
	}
	if r.Copy.Files != 2 || r.Skipped.Files != 1 || r.Skipped.Bytes != 1000 || len(r.IgnoredTop) != 1 || r.IgnoredTop[0] != "big/" {
		t.Fatalf("%+v", r)
	}
	if exists(filepath.Join(proj, ".incognito")) || exists(filepath.Join(home, "store")) {
		t.Fatal("dry-run must not link or touch the store")
	}
	if !exists(filepath.Join(docs, "a.md")) {
		t.Fatal("src untouched")
	}
}

func TestAdoptDryRunExits2OnRefusal(t *testing.T) {
	testutil.Setup(t)
	proj := testutil.Project(t, "demo")
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, "ref", "r.md"), "R")
	testutil.Git(t, filepath.Join(docs, "ref"), "init", "-q")
	code, out := run(t, proj, "adopt", "--dry-run", "docs-local")
	if code != 2 || !strings.Contains(out, "ref") {
		t.Fatal(code, out)
	}
}

func TestAdoptRerunIsNoOp(t *testing.T) {
	_, proj := adoptSetup(t)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, "a.md"), "A")
	mustWrite(t, filepath.Join(docs, ".incognitoignore"), "ref/\n")
	mustWrite(t, filepath.Join(docs, "ref", "r.md"), "R")
	if code, out := run(t, proj, "adopt", "docs-local"); code != 0 {
		t.Fatal(out)
	}
	if code, out := run(t, proj, "adopt", "docs-local"); code != 0 {
		t.Fatal(code, out)
	}
	if b, _ := os.ReadFile(filepath.Join(docs, "ref", "r.md")); string(b) != "R" {
		t.Fatal("ignored data must survive a rerun")
	}
	if !exists(filepath.Join(proj, ".incognito", "docs-local", "a.md")) {
		t.Fatal("notes intact")
	}
}

func TestAdoptNegationPattern(t *testing.T) {
	_, proj := adoptSetup(t)
	docs := filepath.Join(proj, "docs-local")
	mustWrite(t, filepath.Join(docs, ".incognitoignore"), "*.md\n!keep.md\n")
	mustWrite(t, filepath.Join(docs, "keep.md"), "K")
	mustWrite(t, filepath.Join(docs, "skip.md"), "S")
	if code, out := run(t, proj, "adopt", "docs-local"); code != 0 {
		t.Fatal(out)
	}
	n := filepath.Join(proj, ".incognito", "docs-local")
	if !exists(filepath.Join(n, "keep.md")) || exists(filepath.Join(n, "skip.md")) {
		t.Fatal("negation not honored in copy")
	}
	if exists(filepath.Join(docs, "keep.md")) || !exists(filepath.Join(docs, "skip.md")) {
		t.Fatal("keep.md removed, skip.md stays")
	}
}
