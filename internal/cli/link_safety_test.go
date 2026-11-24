package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/registry"
	"github.com/glincker/gitincognito/internal/store"
	"github.com/glincker/gitincognito/internal/testutil"
)

func TestLinkRefusesTrackedPointerFile(t *testing.T) {
	_, proj := adoptSetup(t)
	mustWrite(t, filepath.Join(proj, "CLAUDE.local.md"), "team file\n")
	testutil.Git(t, proj, "add", "CLAUDE.local.md")
	testutil.Git(t, proj, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "track")
	if code, _ := run(t, proj, "link"); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	b, _ := os.ReadFile(filepath.Join(proj, "CLAUDE.local.md"))
	if string(b) != "team file\n" {
		t.Fatalf("tracked file modified: %q", b)
	}
	if _, err := os.Stat(filepath.Join(proj, ".incognito")); !os.IsNotExist(err) {
		t.Fatal("nothing may be created")
	}
}

func TestDoctorFlagsTrackedPointer(t *testing.T) {
	_, _, proj := linked(t)
	testutil.Git(t, proj, "add", "-f", "CLAUDE.local.md")
	code, out := run(t, proj, "doctor", "--json")
	if code != 2 || !contains(out, "pointer-tracked") {
		t.Fatalf("want 2 with pointer-tracked, got %d: %s", code, out)
	}
}

func TestLinkRejectsBadNames(t *testing.T) {
	_, proj := adoptSetup(t)
	for _, n := range []string{"../../x", "-x", "a/b", "a..b", ".hidden", "a b"} {
		if code, out := run(t, proj, "link", "--name", n); code != 1 {
			t.Fatalf("--name %q: want 1, got %d (%s)", n, code, out)
		}
	}
	if _, err := os.Stat(filepath.Join(proj, ".incognito")); !os.IsNotExist(err) {
		t.Fatal("nothing may be created")
	}
}

func TestLinkRejectsBadFolderName(t *testing.T) {
	testutil.Setup(t)
	remote := testutil.BareRemote(t)
	proj := testutil.Project(t, "-weird")
	run(t, proj, "init", "--remote", remote)
	if code, _ := run(t, proj, "link"); code != 1 {
		t.Fatalf("want 1, got %d", code)
	}
}

func TestRegistryBadNamesAreSkippedWithErrorRows(t *testing.T) {
	home, _, proj := linked(t)
	reg, _ := registry.Load(store.RegistryPath(home))
	reg.Projects[0].Name = "../evil"
	if err := reg.Save(store.RegistryPath(home)); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"status", "sync", "restore"} {
		code, out := run(t, proj, cmd, "--all", "--json")
		if !contains(out, "invalid project name") {
			t.Fatalf("%s: want error row, got %d: %s", cmd, code, out)
		}
	}
	if code, _ := run(t, proj, "unlink"); code != 1 {
		t.Fatalf("unlink: want 1, got %d", code)
	}
	if _, err := os.Stat(filepath.Join(proj, ".incognito")); err != nil {
		t.Fatal("unlink must not touch the worktree")
	}
}

func TestLinkRefusesDuplicateFolderName(t *testing.T) {
	_, _, _ = linked(t)
	other := testutil.Project(t, "demo")
	if code, _ := run(t, other, "link"); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	if code, out := run(t, other, "link", "--name", "demo2"); code != 0 {
		t.Fatal(out)
	}
}

func TestLinkDedicatedRefusesRemoteMismatch(t *testing.T) {
	home, _, _ := linked(t)
	proj2 := testutil.Project(t, "other")
	if code, out := run(t, proj2, "link", "--store", "dedicated", "--remote", testutil.BareRemote(t)); code != 0 {
		t.Fatal(out)
	}
	if code, _ := run(t, proj2, "link", "--store", "dedicated", "--remote", testutil.BareRemote(t)); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	if !store.Exists(store.DedicatedDir(home, "other")) {
		t.Fatal("store must remain")
	}
}
