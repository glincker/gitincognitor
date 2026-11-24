package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/glincker/gitincognito/internal/git"
	"github.com/glincker/gitincognito/internal/project"
	"github.com/glincker/gitincognito/internal/store"
)

func init() { commands["doctor"] = cmdDoctor }

type check struct {
	Project string `json:"project,omitempty"`
	Name    string `json:"name"`
	Level   string `json:"level"`
	Detail  string `json:"detail,omitempty"`
}

func cmdDoctor(c *Ctx, args []string) error {
	reg, err := loadRegistry(c)
	if err != nil {
		return err
	}
	var checks []check
	add := func(proj, name, level, detail string) {
		checks = append(checks, check{proj, name, level, detail})
	}
	if err := git.MinVersionOK(); err != nil {
		add("", "git-version", "fail", err.Error())
	} else {
		add("", "git-version", "ok", "")
	}
	for _, p := range reg.Projects {
		if err := checkEntry(p); err != nil {
			add(p.Name, "entry", "fail", err.Error())
			continue
		}
		wt := filepath.Join(p.Path, ".incognito")
		if _, err := os.Stat(p.Path); err != nil {
			add(p.Name, "path", "fail", "project path missing")
			continue
		}
		if project.Detect(p.Path).IsGit && project.PointerTracked(project.Detect(p.Path).Root) {
			add(p.Name, "pointer-tracked", "fail", "CLAUDE.local.md is tracked by the project repo")
		}
		if _, err := os.Stat(filepath.Join(wt, ".git")); err != nil {
			add(p.Name, "linked", "fail", ".incognito is not a notes worktree, run: git incognito restore")
			continue
		}
		add(p.Name, "linked", "ok", "")
		info := project.Detect(p.Path)
		if info.IsGit {
			if project.Excluded(info.Root, ".incognito/") {
				add(p.Name, "excluded", "ok", "")
			} else {
				add(p.Name, "excluded", "fail", ".incognito/ missing from .git/info/exclude")
			}
			if out, _ := git.Run(info.Root, "ls-files", ".incognito"); out != "" {
				add(p.Name, "untracked", "fail", ".incognito files are tracked by the project repo")
			} else {
				add(p.Name, "untracked", "ok", "")
			}
			dir := store.CentralDir(c.Home)
			if p.Store == "dedicated" {
				dir = store.DedicatedDir(c.Home, p.Name)
			}
			if project.SharesRemote(info.Root, store.Remote(dir)) {
				add(p.Name, "remote", "fail", "notes remote is also a remote of the project")
			} else {
				add(p.Name, "remote", "ok", "")
			}
		}
		_ = filepath.WalkDir(wt, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() && d.Name() == ".git" {
				return filepath.SkipDir
			}
			if n := d.Name(); !d.IsDir() && (n == ".env" || strings.HasPrefix(n, ".env.")) {
				add(p.Name, "secrets", "warn", "env-like file in notes: "+n)
			}
			return nil
		})
	}
	failed := 0
	var lines []string
	for _, k := range checks {
		if k.Level == "fail" {
			failed++
		}
		lines = append(lines, fmt.Sprintf("[%s] %s %s %s", k.Level, k.Project, k.Name, k.Detail))
	}
	c.Emit(checks, strings.Join(lines, "\n"))
	if failed > 0 {
		return Unsafe("doctor found %d problem(s)", failed)
	}
	return nil
}
