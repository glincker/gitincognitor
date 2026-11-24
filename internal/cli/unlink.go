package cli

import (
	"path/filepath"

	"github.com/glincker/gitincognito/internal/git"
	"github.com/glincker/gitincognito/internal/project"
	"github.com/glincker/gitincognito/internal/store"
)

func init() { commands["unlink"] = cmdUnlink }

func cmdUnlink(c *Ctx, args []string) error {
	fs := c.flags("unlink")
	force := fs.Bool("force", false, "discard unsynced notes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	reg, err := loadRegistry(c)
	if err != nil {
		return err
	}
	ts, err := targets(c, reg, false)
	if err != nil {
		return err
	}
	p := ts[0]
	if err := checkEntry(p); err != nil {
		return err
	}
	wt := filepath.Join(p.Path, ".incognito")
	if st, _ := git.Run(wt, "status", "--porcelain"); st != "" && !*force {
		return Unsafe("notes have unsynced changes, run sync or pass --force")
	}
	dir := store.CentralDir(c.Home)
	if p.Store == "dedicated" {
		dir = store.DedicatedDir(c.Home, p.Name)
	}
	if _, err := git.Run(dir, "worktree", "remove", "--force", wt); err != nil {
		return err
	}
	if info := project.Detect(p.Path); info.IsGit {
		if err := project.RemoveExclude(info.Root, ".incognito/", "CLAUDE.local.md"); err != nil {
			return err
		}
	}
	if err := project.RemovePointer(p.Path); err != nil {
		return err
	}
	reg.Remove(p.Name)
	if err := reg.Save(store.RegistryPath(c.Home)); err != nil {
		return err
	}
	if err := store.CommitRegistry(store.CentralDir(c.Home), "chore: unlink "+p.Name); err != nil {
		return err
	}
	c.Emit(map[string]any{"ok": true, "unlinked": p.Name, "branch_kept": p.Branch}, "unlinked "+p.Name+", branch "+p.Branch+" kept")
	return nil
}
