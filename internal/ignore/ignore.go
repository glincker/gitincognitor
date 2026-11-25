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
	"unicode"
)

// Matcher owns one scratch repo holding the rules, reused for every batch.
type Matcher struct {
	repo, home string
	env        []string
}

// NewMatcher builds the scratch repo once. probe is an existing path whose
// filesystem decides case sensitivity; empty means case sensitive.
func NewMatcher(rules, probe string) (*Matcher, error) {
	m := &Matcher{}
	var err error
	if m.repo, err = os.MkdirTemp("", "incognito-ignore-"); err != nil {
		return nil, err
	}
	if m.home, err = os.MkdirTemp("", "incognito-ignore-home-"); err != nil {
		m.Close()
		return nil, err
	}
	m.env = isolatedEnv(m.home)
	if err := m.git("init", "-q"); err != nil {
		m.Close()
		return nil, err
	}
	fold := "false"
	if caseInsensitive(probe) {
		fold = "true"
	}
	if err := m.git("config", "core.ignorecase", fold); err != nil {
		m.Close()
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(m.repo, ".gitignore"), []byte(rules), 0o644); err != nil {
		m.Close()
		return nil, err
	}
	return m, nil
}

// Close removes the scratch directories.
func (m *Matcher) Close() {
	if m.repo != "" {
		os.RemoveAll(m.repo)
	}
	if m.home != "" {
		os.RemoveAll(m.home)
	}
}

// Match returns the subset of paths the rules ignore. Directories must carry
// a trailing slash. One git process serves the whole batch.
func (m *Matcher) Match(paths []string) (map[string]bool, error) {
	matched := map[string]bool{}
	if len(paths) == 0 {
		return matched, nil
	}
	var out bytes.Buffer
	cmd := m.cmd("check-ignore", "--no-index", "--stdin", "-z")
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	cmd.Stdout = &out
	err := cmd.Run()
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

// Match is the one-shot form of Matcher, case sensitive.
func Match(rules string, paths []string) (map[string]bool, error) {
	if len(paths) == 0 || strings.TrimSpace(rules) == "" {
		return map[string]bool{}, nil
	}
	m, err := NewMatcher(rules, "")
	if err != nil {
		return nil, err
	}
	defer m.Close()
	return m.Match(paths)
}

func (m *Matcher) cmd(args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = m.repo
	cmd.Env = m.env
	return cmd
}

func (m *Matcher) git(args ...string) error { return m.cmd(args...).Run() }

// isolatedEnv drops every inherited GIT_* variable (GIT_DIR and friends would
// redirect git to another repo) and points all config lookups at nothing, so
// no personal excludes file can change what counts as ignored.
func isolatedEnv(home string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return append(env,
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.excludesFile", "GIT_CONFIG_VALUE_0="+os.DevNull,
		"HOME="+home, "XDG_CONFIG_HOME="+home, "LC_ALL=C")
}

// caseInsensitive stats a case-flipped spelling of an existing path: if it
// resolves to the same file the filesystem folds case.
func caseInsensitive(probe string) bool {
	if probe == "" {
		return false
	}
	a, err := os.Stat(probe)
	if err != nil {
		return false
	}
	rs := []rune(probe)
	for i := len(rs) - 1; i >= 0; i-- {
		var f rune
		switch {
		case unicode.IsLower(rs[i]):
			f = unicode.ToUpper(rs[i])
		case unicode.IsUpper(rs[i]):
			f = unicode.ToLower(rs[i])
		default:
			continue
		}
		rs[i] = f
		b, err := os.Stat(string(rs))
		return err == nil && os.SameFile(a, b)
	}
	return false
}
