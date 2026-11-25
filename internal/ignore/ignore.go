// Package ignore matches paths against gitignore rules by asking git itself,
// so pattern semantics (negation, anchoring, dir-only) never drift from git's.
package ignore

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Match returns the subset of paths that rules ignore. Directories must carry
// a trailing slash. One git process serves the whole batch.
func Match(rules string, paths []string) (map[string]bool, error) {
	matched := map[string]bool{}
	if len(paths) == 0 || strings.TrimSpace(rules) == "" {
		return matched, nil
	}
	dir, err := os.MkdirTemp("", "incognito-ignore-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err := git(dir, "init", "-q"); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(rules), 0o644); err != nil {
		return nil, err
	}
	in := strings.Join(paths, "\x00") + "\x00"
	var out bytes.Buffer
	cmd := gitCmd(dir, "check-ignore", "--no-index", "--stdin", "-z")
	cmd.Stdin = strings.NewReader(in)
	cmd.Stdout = &out
	err = cmd.Run()
	var ee *exec.ExitError
	if err != nil && !(errors.As(err, &ee) && ee.ExitCode() == 1) {
		return nil, err
	}
	for _, p := range strings.Split(out.String(), "\x00") {
		if p != "" {
			matched[p] = true
		}
	}
	return matched, nil
}

// gitCmd isolates git from the user's global config so a personal
// core.excludesFile cannot change what counts as ignored.
func gitCmd(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "LC_ALL=C")
	return cmd
}

func git(dir string, args ...string) error {
	return gitCmd(dir, args...).Run()
}
