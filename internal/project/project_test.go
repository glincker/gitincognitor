package project_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glincker/gitincognitor/internal/git"
	"github.com/glincker/gitincognitor/internal/project"
	"github.com/glincker/gitincognitor/internal/testutil"
)

func TestExcludeIdempotentAndRemovable(t *testing.T) {
	testutil.Setup(t)
	root := testutil.Project(t, "p")
	for i := 0; i < 2; i++ {
		if err := project.AddExclude(root, ".incognito/", "CLAUDE.local.md"); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	if strings.Count(string(b), ".incognito/") != 1 {
		t.Fatalf("duplicated:\n%s", b)
	}
	if !project.Excluded(root, ".incognito/") {
		t.Fatal("Excluded false")
	}
	if err := project.RemoveExclude(root, ".incognito/"); err != nil {
		t.Fatal(err)
	}
	if project.Excluded(root, ".incognito/") {
		t.Fatal("still excluded")
	}
}

func TestPointerUpsertKeepsUserContent(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "CLAUDE.local.md")
	_ = os.WriteFile(f, []byte("my own notes\n"), 0o644)
	for i := 0; i < 2; i++ {
		if err := project.WritePointer(root); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(f)
	s := string(b)
	if !strings.Contains(s, "my own notes") || strings.Count(s, "gitincognitor:start") != 1 {
		t.Fatalf("bad pointer file:\n%s", s)
	}
	if err := project.RemovePointer(root); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(f)
	if strings.Contains(string(b), "gitincognitor") || !strings.Contains(string(b), "my own notes") {
		t.Fatalf("remove damaged file:\n%s", b)
	}
}

func TestSharesRemoteNormalizes(t *testing.T) {
	testutil.Setup(t)
	root := testutil.Project(t, "p")
	testutil.Git(t, root, "remote", "add", "origin", "git@github.com:glincker/Notes.git")
	if !project.SharesRemote(root, "https://github.com/glincker/notes") {
		t.Fatal("expected match")
	}
	if project.SharesRemote(root, "https://github.com/glincker/other") {
		t.Fatal("unexpected match")
	}
}

func TestGuardBlocksStagedNotes(t *testing.T) {
	testutil.Setup(t)
	root := testutil.Project(t, "p")
	if err := project.InstallGuard(root); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(root, ".incognito"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".incognito", "x.md"), []byte("x"), 0o644)
	testutil.Git(t, root, "add", "-f", ".incognito/x.md")
	_, err := git.Run(root, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "x")
	if err == nil {
		t.Fatal("guard should have blocked the commit")
	}
}

func TestDetectPlainFolder(t *testing.T) {
	d := t.TempDir()
	info := project.Detect(d)
	if info.IsGit || info.Root != d {
		t.Fatalf("%+v", info)
	}
}
