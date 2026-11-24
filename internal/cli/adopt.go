package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
	src, err := checkAdoptSource(src, info)
	if err != nil {
		return err
	}
	if err := checkAdoptTree(src); err != nil {
		return err
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
	wt := filepath.Join(p.Path, ".incognito")
	r := syncer.Sync(c.Home, p, 0, time.Now(), true)
	if r.Error != "" || (!r.Pushed && syncer.Ahead(wt) > 0) {
		return errors.New("kept the original folder, notes not pushed yet: " + r.Error)
	}
	if err := verifyAdopted(wt, filepath.Base(src), src); err != nil {
		return err
	}
	if err := os.RemoveAll(src); err != nil {
		return err
	}
	c.Emit(map[string]any{"ok": true, "adopted": dest}, "adopted "+src+" -> "+dest)
	return nil
}

func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// checkAdoptSource resolves symlinks and refuses sources whose removal could
// take the project, its notes worktree or anything outside the project with it.
func checkAdoptSource(src string, info project.Info) (string, error) {
	real, err := filepath.EvalSymlinks(src)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(info.Root)
	if err != nil {
		return "", err
	}
	notes := filepath.Join(root, ".incognito")
	switch {
	case real == string(filepath.Separator):
		return "", Unsafe("refusing to adopt /")
	case within(real, root):
		return "", Unsafe("%s is the project or contains it", real)
	case within(notes, real) || within(real, notes):
		return "", Unsafe("%s overlaps the notes folder", real)
	case info.IsGit && !within(root, real):
		return "", Unsafe("%s is outside the project", real)
	}
	return real, nil
}

// checkAdoptTree refuses trees git cannot carry whole: nested repos and symlinks.
func checkAdoptTree(src string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch {
		case d.Name() == ".git":
			return Unsafe("%s is a nested git entry, adopt would lose its history", path)
		case d.Type()&fs.ModeSymlink != 0:
			return Unsafe("%s is a symlink, replace it with the real file first", path)
		}
		return nil
	})
}

// verifyAdopted proves every file in src is committed in the notes branch and
// nothing was left ignored or untracked, so deleting src cannot lose data.
func verifyAdopted(wt, base, src string) error {
	out, err := git.Run(wt, "--literal-pathspecs", "ls-files", "-z", "--", base)
	if err != nil {
		return err
	}
	tracked := map[string]bool{}
	for _, f := range strings.Split(out, "\x00") {
		tracked[f] = true
	}
	var missing []string
	_ = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			rel, _ := filepath.Rel(src, path)
			if !tracked[filepath.ToSlash(filepath.Join(base, rel))] {
				missing = append(missing, rel)
			}
		}
		return nil
	})
	if st, _ := git.Run(wt, "status", "--porcelain", "--ignored"); st != "" && len(missing) == 0 {
		missing = append(missing, "(uncommitted or ignored files in notes)")
	}
	if len(missing) > 0 {
		return fmt.Errorf("kept the original folder, not committed to the notes branch: %s", strings.Join(missing, ", "))
	}
	return nil
}
