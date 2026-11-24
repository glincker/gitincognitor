package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/testutil"
)

// Whole story: init, adopt existing notes, crash, restore, doctor is green.
func TestEndToEndCrashDrill(t *testing.T) {
	home := testutil.Setup(t)
	remote := testutil.BareRemote(t)
	proj := testutil.Project(t, "demo")
	for _, args := range [][]string{{"init", "--remote", remote}} {
		if code, out := run(t, proj, args...); code != 0 {
			t.Fatal(out)
		}
	}
	_ = os.MkdirAll(filepath.Join(proj, "docs-local"), 0o755)
	_ = os.WriteFile(filepath.Join(proj, "docs-local", "idea.md"), []byte("idea"), 0o644)
	if code, out := run(t, proj, "adopt", "docs-local"); code != 0 {
		t.Fatal(out)
	}
	_ = os.RemoveAll(home)
	_ = os.RemoveAll(proj)
	_ = os.MkdirAll(proj, 0o755)
	testutil.Git(t, proj, "init", "-q", "-b", "main")
	if code, out := run(t, proj, "restore", "--all", "--remote", remote); code != 0 {
		t.Fatal(out)
	}
	if b, err := os.ReadFile(filepath.Join(proj, ".incognito", "docs-local", "idea.md")); err != nil || string(b) != "idea" {
		t.Fatalf("lost the note: %q %v", b, err)
	}
	if code, out := run(t, proj, "doctor"); code != 0 {
		t.Fatalf("doctor red after restore: %s", out)
	}
}
