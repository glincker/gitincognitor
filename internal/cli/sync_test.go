package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognitor/internal/testutil"
)

func writeNote(t *testing.T, proj, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(proj, ".incognito", name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSyncCommitsAndPushes(t *testing.T) {
	_, remote, proj := linked(t)
	writeNote(t, proj, "a.md", "one")
	if code, out := run(t, proj, "sync"); code != 0 {
		t.Fatal(out)
	}
	if testutil.Git(t, remote, "branch", "--list", "proj/demo") == "" {
		t.Fatal("branch not on remote")
	}
	if got := testutil.Git(t, filepath.Join(proj, ".incognito"), "status", "--porcelain"); got != "" {
		t.Fatalf("not clean: %q", got)
	}
}

func TestSyncNoopWhenClean(t *testing.T) {
	_, _, proj := linked(t)
	if code, out := run(t, proj, "sync"); code != 0 {
		t.Fatal(out)
	}
}

func TestSyncDebounce(t *testing.T) {
	_, _, proj := linked(t)
	t.Setenv("GITINCOGNITOR_SYNC_DEBOUNCE_SECS", "3600")
	writeNote(t, proj, "a.md", "one")
	run(t, proj, "sync")
	writeNote(t, proj, "b.md", "two")
	_, out := run(t, proj, "sync", "--json")
	if want := "debounced"; !contains(out, want) {
		t.Fatalf("want %q in %s", want, out)
	}
	if got := testutil.Git(t, filepath.Join(proj, ".incognito"), "status", "--porcelain"); got == "" {
		t.Fatal("b.md should still be uncommitted")
	}
	if code, out := run(t, proj, "sync", "--force"); code != 0 {
		t.Fatal(out)
	}
}

func TestQuietSyncSurvivesBrokenRemote(t *testing.T) {
	home, _, proj := linked(t)
	testutil.Git(t, filepath.Join(home, "store"), "remote", "set-url", "origin", "/nonexistent/x.git")
	writeNote(t, proj, "a.md", "one")
	if code, out := run(t, proj, "sync", "--quiet"); code != 0 {
		t.Fatalf("quiet must exit 0: %d %s", code, out)
	}
	if got := testutil.Git(t, filepath.Join(proj, ".incognito"), "log", "--oneline"); got == "" {
		t.Fatal("note should be committed locally")
	}
	if code, _ := run(t, proj, "sync"); code != 1 {
		t.Fatalf("non-quiet should exit 1 on push error, got %d", code)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool { return indexOf(s, sub) >= 0 })()
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
