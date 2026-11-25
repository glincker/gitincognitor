package cli

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/glincker/gitincognitor/internal/ignore"
)

const ignoreFileName = ".incognitoignore"

// adoptPlan is everything adopt will touch, decided before any write.
type adoptPlan struct {
	Files        []string // slash-separated, relative to src
	Dirs         []string // walked (not ignored) dirs, parents first, "" is src
	CopyBytes    int64
	IgnoredTop   []string
	SkippedFiles int64
	SkippedBytes int64
	Refusals     []string
	sizes        map[string]int64
}

// buildAdoptPlan walks src one directory level at a time so an ignored
// directory is never descended into, with one git call per level.
// measure also sizes the ignored subtrees (dry-run only, it can be slow).
func buildAdoptPlan(src string, measure bool) (*adoptPlan, error) {
	rules := ""
	if fi, err := os.Lstat(filepath.Join(src, ignoreFileName)); err == nil && fi.Mode().IsRegular() {
		b, err := os.ReadFile(filepath.Join(src, ignoreFileName))
		if err != nil {
			return nil, err
		}
		rules = string(b)
	}
	p := &adoptPlan{sizes: map[string]int64{}}
	queue := []string{""}
	for len(queue) > 0 {
		rel := queue[0]
		queue = queue[1:]
		p.Dirs = append(p.Dirs, rel)
		entries, err := os.ReadDir(filepath.Join(src, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = path.Join(rel, e.Name())
			if e.IsDir() {
				names[i] += "/"
			}
		}
		hit, err := ignore.Match(rules, names)
		if err != nil {
			return nil, err
		}
		for i, e := range entries {
			if err := p.visit(src, rel, e, names[i], hit[names[i]], measure, &queue); err != nil {
				return nil, err
			}
		}
	}
	sort.Strings(p.Files)
	return p, nil
}

func (p *adoptPlan) visit(src, rel string, e fs.DirEntry, name string, ignored, measure bool, queue *[]string) error {
	full := filepath.Join(src, filepath.FromSlash(name))
	switch {
	case ignored:
		if rel == "" {
			p.IgnoredTop = append(p.IgnoredTop, name)
		}
		if measure {
			return p.measure(full)
		}
	case e.Name() == ".git":
		p.Refusals = append(p.Refusals, full+" is a nested git entry, adopt would lose its history")
	case e.Type()&fs.ModeSymlink != 0:
		p.Refusals = append(p.Refusals, full+" is a symlink, replace it with the real file first")
	case e.IsDir():
		*queue = append(*queue, path.Clean(name))
	case e.Type().IsRegular():
		fi, err := e.Info()
		if err != nil {
			return err
		}
		p.Files = append(p.Files, name)
		p.sizes[name] = fi.Size()
		p.CopyBytes += fi.Size()
	default:
		p.Refusals = append(p.Refusals, full+" is not a regular file")
	}
	return nil
}

func (p *adoptPlan) measure(full string) error {
	return filepath.WalkDir(full, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil {
				p.SkippedFiles++
				p.SkippedBytes += fi.Size()
			}
		}
		return nil
	})
}

// copyPlanned copies only planned files, so ignored content never leaves src.
func copyPlanned(src, dest string, files []string) error {
	for _, rel := range files {
		from := filepath.Join(src, filepath.FromSlash(rel))
		to := filepath.Join(dest, filepath.FromSlash(rel))
		fi, err := os.Stat(from)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(to, b, fi.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

// alreadyAdopted reports whether every planned file is already in dest
// byte for byte, which makes a rerun after success a harmless no-op.
func alreadyAdopted(src, dest string, files []string) bool {
	for _, rel := range files {
		have, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(rel)))
		if err != nil {
			return false
		}
		want, err := os.ReadFile(filepath.Join(src, filepath.FromSlash(rel)))
		if err != nil || !bytes.Equal(have, want) {
			return false
		}
	}
	return true
}

// removeAdopted deletes only copied files, then empty directories. The rules
// file stays so a rerun keeps honoring it. A dir holding ignored data survives.
func removeAdopted(src string, p *adoptPlan) error {
	for _, rel := range p.Files {
		if rel == ignoreFileName {
			continue
		}
		if err := os.Remove(filepath.Join(src, filepath.FromSlash(rel))); err != nil {
			return err
		}
	}
	for i := len(p.Dirs) - 1; i >= 0; i-- {
		// os.Remove refuses a non-empty dir, which is exactly the guard we want.
		_ = os.Remove(filepath.Join(src, filepath.FromSlash(p.Dirs[i])))
	}
	return nil
}

// reportDryRun prints the plan and exits 2 when a refusal would occur.
func reportDryRun(c *Ctx, p *adoptPlan) error {
	refused := len(p.Refusals) > 0
	top, refs := p.IgnoredTop, p.Refusals
	if top == nil {
		top = []string{}
	}
	if refs == nil {
		refs = []string{}
	}
	type count struct {
		Files int64 `json:"files"`
		Bytes int64 `json:"bytes"`
	}
	human := fmt.Sprintf("would copy %d files (%d bytes), skip %d ignored files (%d bytes)",
		len(p.Files), p.CopyBytes, p.SkippedFiles, p.SkippedBytes)
	for _, t := range top {
		human += "\nignored: " + t
	}
	for _, r := range refs {
		human += "\nrefused: " + r
	}
	c.Emit(map[string]any{
		"ok": !refused, "dry_run": true,
		"copy":        count{int64(len(p.Files)), p.CopyBytes},
		"skipped":     count{p.SkippedFiles, p.SkippedBytes},
		"ignored_top": top, "refusals": refs,
	}, human)
	if refused {
		return quietExit{2}
	}
	return nil
}
