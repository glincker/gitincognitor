package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/git"
)

// Setup isolates git config and points GITINCOGNITO_HOME at a temp dir.
func Setup(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("GITINCOGNITO_HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GITINCOGNITO_SYNC_DEBOUNCE_SECS", "0")
	return home
}

func Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := git.Run(dir, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func BareRemote(t *testing.T) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), "r.git")
	Git(t, "", "init", "-q", "--bare", "-b", "main", d)
	return d
}

// Project makes a git repo with one commit, path resolved through symlinks.
func Project(t *testing.T, name string) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	d, _ = filepath.EvalSymlinks(d)
	Git(t, d, "init", "-q", "-b", "main")
	Git(t, d, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init")
	return d
}
