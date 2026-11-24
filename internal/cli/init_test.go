package cli_test

import (
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/testutil"
)

func TestInitRequiresRemote(t *testing.T) {
	testutil.Setup(t)
	if code, _ := run(t, t.TempDir(), "init"); code != 1 {
		t.Fatalf("code %d", code)
	}
}

func TestInitCreatesStore(t *testing.T) {
	home := testutil.Setup(t)
	remote := testutil.BareRemote(t)
	code, out := run(t, t.TempDir(), "init", "--remote", remote, "--json")
	if code != 0 {
		t.Fatal(out)
	}
	testutil.Git(t, filepath.Join(home, "store"), "rev-parse", "--git-dir")
}
