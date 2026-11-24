package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/glincker/gitincognito/internal/project"
	"github.com/glincker/gitincognito/internal/registry"
	"github.com/glincker/gitincognito/internal/store"
)

func init() { commands["restore"] = cmdRestore }

type restoreRow struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func cmdRestore(c *Ctx, args []string) error {
	fs := c.flags("restore")
	remote := fs.String("remote", "", "store remote URL (needed when the store is missing)")
	all := fs.Bool("all", false, "restore every registered project")
	if err := fs.Parse(args); err != nil {
		return err
	}
	central := store.CentralDir(c.Home)
	if !store.Exists(central) {
		if *remote == "" {
			return errors.New("store missing, pass --remote <url>")
		}
		if err := store.Clone(central, *remote); err != nil {
			return err
		}
	}
	reg, err := loadRegistry(c)
	if err != nil {
		return err
	}
	ts, err := targets(c, reg, *all)
	if err != nil {
		return err
	}
	var rows []restoreRow
	var lines []string
	for _, p := range ts {
		st, err := restoreOne(c, p)
		if err != nil {
			st = "error: " + err.Error()
		}
		rows = append(rows, restoreRow{p.Name, st})
		lines = append(lines, fmt.Sprintf("%s: %s", p.Name, st))
	}
	c.Emit(rows, strings.Join(lines, "\n"))
	return nil
}

func restoreOne(c *Ctx, p registry.Project) (string, error) {
	if _, err := os.Stat(p.Path); err != nil {
		return "skipped: project path missing, clone it and rerun", nil
	}
	dir := store.CentralDir(c.Home)
	if p.Store == "dedicated" {
		dir = store.DedicatedDir(c.Home, p.Name)
		if !store.Exists(dir) {
			if err := store.Clone(dir, p.Remote); err != nil {
				return "", err
			}
		}
	}
	wt := filepath.Join(p.Path, ".incognito")
	if _, err := os.Stat(wt); err == nil {
		if _, err := os.Stat(filepath.Join(wt, ".git")); err != nil {
			return "skipped: .incognito exists and is not a notes worktree", nil
		}
	} else if err := store.AddWorktree(dir, p.Branch, wt); err != nil {
		return "", err
	}
	info := project.Detect(p.Path)
	if info.IsGit {
		if err := project.AddExclude(info.Root, ".incognito/", "CLAUDE.local.md"); err != nil {
			return "", err
		}
	}
	if err := project.WritePointer(p.Path); err != nil {
		return "", err
	}
	return "restored", nil
}
