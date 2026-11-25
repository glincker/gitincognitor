package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glincker/gitincognitor/internal/git"
	"github.com/glincker/gitincognitor/internal/project"
	"github.com/glincker/gitincognitor/internal/syncer"
)

func init() { commands["adopt"] = cmdAdopt }

// Test seams, nil in production: they let tests change the folder at the
// exact moments a concurrent edit could hurt.
var (
	adoptBeforeCopy func()
	adoptAfterSync  func()
)

// cmdAdopt copies a folder into the notes branch, pushes, and only then
// removes the original, so a failed push never loses files.
func cmdAdopt(c *Ctx, args []string) error {
	fs := c.flags("adopt")
	name := fs.String("name", "", "project name (default: folder name)")
	dry := fs.Bool("dry-run", false, "report what would be copied and skipped, write nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: git incognitor adopt <folder>")
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
	plan, err := buildAdoptPlan(src, *dry)
	if err != nil {
		return err
	}
	dest := filepath.Join(info.Root, ".incognito", filepath.Base(src))
	destExists, destMatches := destState(src, dest, plan.Files)
	if refs := adoptRefusals(info, src, dest, destExists, destMatches, plan); len(refs) > 0 {
		plan.Refusals = append(plan.Refusals, refs...)
	}
	if *dry {
		return reportDryRun(c, plan)
	}
	if len(plan.Refusals) > 0 {
		return Unsafe("%s", strings.Join(plan.Refusals, "; "))
	}
	p, err := linkProject(c, linkOpts{Store: "central", Name: *name})
	if err != nil {
		return err
	}
	dest = filepath.Join(p.Path, ".incognito", filepath.Base(src))
	if !destExists && len(plan.Files) > 0 {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		if adoptBeforeCopy != nil {
			adoptBeforeCopy()
		}
		if err := copyPlanned(src, dest, plan.Files); err != nil {
			return err
		}
	}
	// A matching copy from an earlier run still goes through sync, verify and
	// removal, so a rerun after a failed push cannot report false success.
	wt := filepath.Join(p.Path, ".incognito")
	r := syncer.Sync(c.Home, p, 0, time.Now(), true)
	if r.Error != "" || (!r.Pushed && syncer.Ahead(wt) > 0) {
		return errors.New("kept the original folder, notes not pushed yet: " + r.Error)
	}
	if err := verifyAdopted(wt, filepath.Base(src), plan.Files); err != nil {
		return err
	}
	if adoptAfterSync != nil {
		adoptAfterSync()
	}
	if err := removeAdopted(src, wt, p.Branch, filepath.Base(src), plan); err != nil {
		return err
	}
	msg := "adopted " + src + " -> " + dest
	if len(plan.IgnoredTop) > 0 {
		msg += fmt.Sprintf("\nleft %d ignored entries in place; the rules stay in %s and are also saved in the notes", len(plan.IgnoredTop), filepath.Join(src, ignoreFileName))
	}
	c.Emit(map[string]any{"ok": true, "adopted": dest, "ignored": plan.IgnoredTop}, msg)
	return nil
}

// destState reports whether the notes copy exists and matches every planned file.
func destState(src, dest string, files []string) (exists, matches bool) {
	if _, err := os.Stat(dest); err != nil {
		return false, false
	}
	return true, alreadyAdopted(src, dest, files)
}

// adoptRefusals lists what a real run would refuse before any side effect,
// so dry-run and a real run agree.
func adoptRefusals(info project.Info, src, dest string, destExists, destMatches bool, plan *adoptPlan) []string {
	var refs []string
	if info.IsGit {
		if out, _ := git.Run(info.Root, "ls-files", "--", src); out != "" {
			refs = append(refs, src+" is tracked by the project repo, untrack it first")
		}
		if project.PointerTracked(info.Root) {
			refs = append(refs, "CLAUDE.local.md is tracked by this repo, untrack it first")
		}
	}
	if destExists && !destMatches {
		rel, _ := filepath.Rel(info.Root, dest)
		refs = append(refs, fmt.Sprintf("%s already exists in the notes and does not match %s (partial or different copy); to redo it, delete %s and rerun adopt", dest, src, rel))
	}
	return refs
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

// verifyAdopted proves every file in src is committed in the notes branch and
// nothing was left ignored or untracked, so deleting src cannot lose data.
func verifyAdopted(wt, base string, files []string) error {
	out, err := git.Run(wt, "--literal-pathspecs", "ls-files", "-z", "--", base)
	if err != nil {
		return err
	}
	tracked := map[string]bool{}
	for _, f := range strings.Split(out, "\x00") {
		tracked[f] = true
	}
	var missing []string
	for _, rel := range files {
		if !tracked[base+"/"+rel] {
			missing = append(missing, rel)
		}
	}
	if st, _ := git.Run(wt, "status", "--porcelain", "--ignored"); st != "" && len(missing) == 0 {
		missing = append(missing, "(uncommitted or ignored files in notes)")
	}
	if len(missing) > 0 {
		return fmt.Errorf("kept the original folder, not committed to the notes branch: %s", strings.Join(missing, ", "))
	}
	return nil
}
