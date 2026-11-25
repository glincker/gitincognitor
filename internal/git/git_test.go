package git_test

import (
	"testing"

	"github.com/glincker/gitincognitor/internal/git"
	"github.com/glincker/gitincognitor/internal/testutil"
)

func TestRunAndHasRef(t *testing.T) {
	testutil.Setup(t)
	dir := testutil.Project(t, "p")
	if !git.HasRef(dir, "refs/heads/main") {
		t.Fatal("expected refs/heads/main")
	}
	if git.HasRef(dir, "refs/heads/nope") {
		t.Fatal("unexpected ref")
	}
	if _, err := git.Run(dir, "not-a-command"); err == nil {
		t.Fatal("expected error")
	}
}

func TestMinVersionOK(t *testing.T) {
	if err := git.MinVersionOK(); err != nil {
		t.Fatal(err)
	}
}

func TestCommitWithoutIdentity(t *testing.T) {
	testutil.Setup(t)
	dir := testutil.Project(t, "p")
	// CI runners have no git identity, so the setup commit must carry its own.
	testutil.Git(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x")
	if err := git.Commit(dir, "y"); err == nil {
		t.Fatal("nothing to commit should error")
	}
}
