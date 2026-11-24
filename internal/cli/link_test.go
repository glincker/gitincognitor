package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glincker/gitincognito/internal/project"
	"github.com/glincker/gitincognito/internal/registry"
	"github.com/glincker/gitincognito/internal/store"
	"github.com/glincker/gitincognito/internal/testutil"
)

func TestLinkHidesNotesAndRegisters(t *testing.T) {
	home, _, proj := linked(t)
	if got := testutil.Git(t, proj, "status", "--porcelain"); got != "" {
		t.Fatalf("outer repo not clean: %q", got)
	}
	if _, err := os.Stat(filepath.Join(proj, ".incognito", ".git")); err != nil {
		t.Fatal("worktree missing")
	}
	if !project.Excluded(proj, ".incognito/") || !project.Excluded(proj, "CLAUDE.local.md") {
		t.Fatal("exclude entries missing")
	}
	b, _ := os.ReadFile(filepath.Join(proj, "CLAUDE.local.md"))
	if !strings.Contains(string(b), "gitincognito:start") {
		t.Fatal("pointer missing")
	}
	reg, _ := registry.Load(store.RegistryPath(home))
	if p := reg.Find("demo"); p == nil || p.Branch != "proj/demo" || p.Path != proj {
		t.Fatalf("registry: %+v", reg.Projects)
	}
}

func TestLinkIsIdempotent(t *testing.T) {
	_, _, proj := linked(t)
	if code, out := run(t, proj, "link"); code != 0 {
		t.Fatal(out)
	}
}

func TestLinkRefusesWhenNotesRemoteIsProjectRemote(t *testing.T) {
	testutil.Setup(t)
	remote := testutil.BareRemote(t)
	proj := testutil.Project(t, "demo")
	testutil.Git(t, proj, "remote", "add", "origin", remote)
	if code, out := run(t, proj, "init", "--remote", remote); code != 0 {
		t.Fatal(out)
	}
	if code, _ := run(t, proj, "link"); code != 2 {
		t.Fatalf("want exit 2, got %d", code)
	}
}

func TestLinkRefusesExistingPlainFolder(t *testing.T) {
	home := testutil.Setup(t)
	_ = home
	remote := testutil.BareRemote(t)
	proj := testutil.Project(t, "demo")
	run(t, proj, "init", "--remote", remote)
	_ = os.MkdirAll(filepath.Join(proj, ".incognito"), 0o755)
	if code, _ := run(t, proj, "link"); code != 2 {
		t.Fatalf("want exit 2, got %d", code)
	}
}

func TestLinkDedicatedStore(t *testing.T) {
	home, _, _ := linked(t)
	proj2 := testutil.Project(t, "other")
	remote2 := testutil.BareRemote(t)
	if code, out := run(t, proj2, "link", "--store", "dedicated", "--remote", remote2); code != 0 {
		t.Fatal(out)
	}
	if !store.Exists(store.DedicatedDir(home, "other")) {
		t.Fatal("dedicated store missing")
	}
	reg, _ := registry.Load(store.RegistryPath(home))
	if p := reg.Find("other"); p == nil || p.Store != "dedicated" || p.Remote != remote2 {
		t.Fatalf("registry: %+v", reg.Projects)
	}
}
