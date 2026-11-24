package project

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/glincker/gitincognito/internal/git"
)

type Info struct {
	Root  string
	IsGit bool
	Name  string
}

// Detect treats a plain folder as a project too (no exclude or guard needed).
func Detect(cwd string) Info {
	if out, err := git.Run(cwd, "rev-parse", "--show-toplevel"); err == nil {
		return Info{Root: out, IsGit: true, Name: filepath.Base(out)}
	}
	return Info{Root: cwd, Name: filepath.Base(cwd)}
}

func gitPath(root, rel string) (string, error) {
	out, err := git.Run(root, "rev-parse", "--git-path", rel)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}
	return out, nil
}

func AddExclude(root string, lines ...string) error {
	p, err := gitPath(root, "info/exclude")
	if err != nil {
		return err
	}
	b, _ := os.ReadFile(p)
	have := map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		have[strings.TrimSpace(l)] = true
	}
	var add []string
	for _, l := range lines {
		if !have[l] {
			add = append(add, l)
		}
	}
	if len(add) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	prefix := ""
	if len(b) > 0 && !strings.HasSuffix(string(b), "\n") {
		prefix = "\n"
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(prefix + strings.Join(add, "\n") + "\n")
	return err
}

func Excluded(root, line string) bool {
	p, err := gitPath(root, "info/exclude")
	if err != nil {
		return false
	}
	b, _ := os.ReadFile(p)
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) == line {
			return true
		}
	}
	return false
}

func RemoveExclude(root string, lines ...string) error {
	p, err := gitPath(root, "info/exclude")
	if err != nil {
		return err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	drop := map[string]bool{}
	for _, l := range lines {
		drop[l] = true
	}
	var keep []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if !drop[strings.TrimSpace(l)] {
			keep = append(keep, l)
		}
	}
	return os.WriteFile(p, []byte(strings.Join(keep, "\n")+"\n"), 0o644)
}

const (
	markStart   = "<!-- gitincognito:start -->"
	markEnd     = "<!-- gitincognito:end -->"
	pointerBody = "Private notes live in `.incognito/` (not tracked by this repo).\n" +
		"Read them for context and write new notes there, then run `git incognito sync`.\n" +
		"Other commands: `git incognito status`, `git incognito doctor` (all accept --json).\n"
)

func pointerBlock() string { return markStart + "\n" + pointerBody + markEnd + "\n" }

func splitPointer(s string) (before, after string, found bool) {
	i := strings.Index(s, markStart)
	j := strings.Index(s, markEnd)
	if i < 0 || j < i {
		return s, "", false
	}
	return s[:i], s[j+len(markEnd):], true
}

func WritePointer(root string) error {
	f := filepath.Join(root, "CLAUDE.local.md")
	b, _ := os.ReadFile(f)
	before, after, found := splitPointer(string(b))
	var out string
	if found {
		out = before + pointerBlock() + strings.TrimPrefix(after, "\n")
	} else {
		out = string(b)
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += pointerBlock()
	}
	return os.WriteFile(f, []byte(out), 0o644)
}

func RemovePointer(root string) error {
	f := filepath.Join(root, "CLAUDE.local.md")
	b, err := os.ReadFile(f)
	if err != nil {
		return nil
	}
	before, after, found := splitPointer(string(b))
	if !found {
		return nil
	}
	rest := before + strings.TrimPrefix(after, "\n")
	if strings.TrimSpace(rest) == "" {
		return os.Remove(f)
	}
	return os.WriteFile(f, []byte(rest), 0o644)
}

const guardMark = "# gitincognito-guard"

const preCommitBody = guardMark + `
if git diff --cached --name-only | grep -q '^\.incognito/'; then
  echo 'gitincognito: refusing to commit files under .incognito/' >&2
  exit 1
fi
`

const prePushBody = guardMark + `
if [ -n "$(git ls-files .incognito)" ]; then
  echo 'gitincognito: .incognito/ is tracked by this repo, refusing to push' >&2
  exit 1
fi
`

func InstallGuard(root string) error {
	dir, err := gitPath(root, "hooks")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, body := range map[string]string{"pre-commit": preCommitBody, "pre-push": prePushBody} {
		p := filepath.Join(dir, name)
		b, err := os.ReadFile(p)
		switch {
		case err != nil:
			if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
				return err
			}
		case !strings.Contains(string(b), guardMark):
			if err := os.WriteFile(p, []byte(string(b)+"\n"+body), 0o755); err != nil {
				return err
			}
		}
	}
	return nil
}

var (
	schemeRe = regexp.MustCompile(`^[a-z+]+://`)
	userRe   = regexp.MustCompile(`^[^@/]+@`)
)

func normalize(u string) string {
	u = strings.ToLower(strings.TrimSpace(u))
	u = strings.TrimSuffix(strings.TrimSuffix(u, "/"), ".git")
	u = schemeRe.ReplaceAllString(u, "")
	u = userRe.ReplaceAllString(u, "")
	return strings.Replace(u, ":", "/", 1)
}

// SharesRemote reports whether any remote of the project equals notesRemote.
func SharesRemote(root, notesRemote string) bool {
	out, err := git.Run(root, "remote", "-v")
	if err != nil {
		return false
	}
	want := normalize(notesRemote)
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && normalize(f[1]) == want {
			return true
		}
	}
	return false
}
