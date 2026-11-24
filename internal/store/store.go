package store

import (
	"os"
	"path/filepath"

	"github.com/glincker/gitincognito/internal/git"
)

func CentralDir(home string) string { return filepath.Join(home, "store") }

func DedicatedDir(home, name string) string { return filepath.Join(home, "dedicated", name) }

func RegistryPath(home string) string { return filepath.Join(CentralDir(home), "registry.yml") }

func Branch(name string) string { return "proj/" + name }

func Exists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func Remote(dir string) string {
	out, _ := git.Run(dir, "remote", "get-url", "origin")
	return out
}

// Init creates a store with a seed commit on main and pushes it, so the
// remote's HEAD never dangles. Safe to call again.
func Init(dir, remote, seedFile, seedBody string) error {
	if Exists(dir) {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if _, err := git.Run(dir, "init", "-q", "-b", "main"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, seedFile), []byte(seedBody), 0o644); err != nil {
		return err
	}
	if _, err := git.Run(dir, "add", "-A"); err != nil {
		return err
	}
	if err := git.Commit(dir, "chore: init store"); err != nil {
		return err
	}
	if _, err := git.Run(dir, "remote", "add", "origin", remote); err != nil {
		return err
	}
	_, err := git.Run(dir, "push", "-q", "-u", "origin", "main")
	return err
}

func Clone(dir, remote string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	_, err := git.Run("", "clone", "-q", remote, dir)
	return err
}

// AddWorktree checks out branch at path: local branch first, then the remote
// branch (restore), else a fresh orphan branch (first link).
func AddWorktree(storeDir, branch, path string) error {
	_, _ = git.Run(storeDir, "worktree", "prune")
	_, _ = git.Run(storeDir, "fetch", "-q", "origin")
	var err error
	switch {
	case git.HasRef(storeDir, "refs/heads/"+branch):
		_, err = git.Run(storeDir, "worktree", "add", "-q", path, branch)
	case git.HasRef(storeDir, "refs/remotes/origin/"+branch):
		_, err = git.Run(storeDir, "worktree", "add", "-q", "--track", "-b", branch, path, "origin/"+branch)
	default:
		_, err = git.Run(storeDir, "worktree", "add", "-q", "--orphan", "-b", branch, path)
	}
	return err
}

// CommitRegistry commits registry.yml on main and pushes best effort.
func CommitRegistry(dir, msg string) error {
	if _, err := git.Run(dir, "add", "registry.yml"); err != nil {
		return err
	}
	if _, err := git.Run(dir, "diff", "--cached", "--quiet"); err == nil {
		return nil
	}
	if err := git.Commit(dir, msg); err != nil {
		return err
	}
	_, _ = git.Run(dir, "push", "-q", "origin", "main")
	return nil
}
