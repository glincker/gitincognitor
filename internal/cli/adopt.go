package cli

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/glincker/gitincognito/internal/git"
	"github.com/glincker/gitincognito/internal/project"
	"github.com/glincker/gitincognito/internal/syncer"
)

func init() { commands["adopt"] = cmdAdopt }

// cmdAdopt copies a folder into the notes branch, pushes, and only then
// removes the original, so a failed push never loses files.
func cmdAdopt(c *Ctx, args []string) error {
	fs := c.flags("adopt")
	name := fs.String("name", "", "project name (default: folder name)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: git incognito adopt <folder>")
	}
	src := fs.Arg(0)
	if !filepath.IsAbs(src) {
		src = filepath.Join(c.Cwd, src)
	}
	if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
		return errors.New("adopt needs an existing folder")
	}
	info := project.Detect(c.Cwd)
	if filepath.Base(src) == ".incognito" {
		return Unsafe("cannot adopt .incognito itself")
	}
	if info.IsGit {
		if out, _ := git.Run(info.Root, "ls-files", "--", src); out != "" {
			return Unsafe("%s is tracked by the project repo, untrack it first", src)
		}
	}
	p, err := linkProject(c, linkOpts{Store: "central", Name: *name})
	if err != nil {
		return err
	}
	dest := filepath.Join(p.Path, ".incognito", filepath.Base(src))
	if _, err := os.Stat(dest); err == nil {
		return Unsafe("%s already exists in the notes", dest)
	}
	if err := os.CopyFS(dest, os.DirFS(src)); err != nil {
		return err
	}
	r := syncer.Sync(c.Home, p, 0, time.Now(), true)
	if r.Error != "" || (!r.Pushed && syncer.Ahead(filepath.Join(p.Path, ".incognito")) > 0) {
		return errors.New("kept the original folder, notes not pushed yet: " + r.Error)
	}
	if err := os.RemoveAll(src); err != nil {
		return err
	}
	c.Emit(map[string]any{"ok": true, "adopted": dest}, "adopted "+src+" -> "+dest)
	return nil
}
