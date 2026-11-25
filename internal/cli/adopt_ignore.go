package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

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
	var m *ignore.Matcher
	if strings.TrimSpace(rules) != "" {
		var err error
		if m, err = ignore.NewMatcher(rules, src); err != nil {
			return nil, err
		}
		defer m.Close()
	}
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
		var queries []string
		for i, e := range entries {
			names[i] = path.Join(rel, e.Name())
			if e.IsDir() {
				names[i] += "/"
			}
			queries = append(queries, names[i])
			// A symlink to a directory is also asked about as a directory, so a
			// dir-only rule like "ref/" can ignore it.
			if e.Type()&fs.ModeSymlink != 0 {
				if fi, err := os.Stat(filepath.Join(src, filepath.FromSlash(names[i]))); err == nil && fi.IsDir() {
					queries = append(queries, names[i]+"/")
				}
			}
		}
		hit := map[string]bool{}
		if m != nil {
			if hit, err = m.Match(queries); err != nil {
				return nil, err
			}
		}
		for i, e := range entries {
			if err := p.visit(src, rel, e, names[i], hit[names[i]] || hit[names[i]+"/"], measure, &queue); err != nil {
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
// Each file is re-checked and opened without following symlinks, because the
// folder can change between the plan and the copy.
func copyPlanned(src, dest string, files []string) error {
	for _, rel := range files {
		from := filepath.Join(src, filepath.FromSlash(rel))
		to := filepath.Join(dest, filepath.FromSlash(rel))
		if err := copyOne(from, to); err != nil {
			return err
		}
	}
	return nil
}

func copyOne(from, to string) error {
	in, fi, err := openRegular(from)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// openRegular opens path only if it is still the regular file it was planned as.
func openRegular(path string) (*os.File, os.FileInfo, error) {
	lfi, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !lfi.Mode().IsRegular() {
		return nil, nil, Unsafe("%s changed after planning and is no longer a regular file, nothing was removed", path)
	}
	f, err := os.OpenFile(path, os.O_RDONLY|openNoFollow, 0)
	if err != nil {
		return nil, nil, Unsafe("%s changed after planning (%v), nothing was removed", path, err)
	}
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() || !os.SameFile(lfi, fi) {
		f.Close()
		return nil, nil, Unsafe("%s changed after planning, nothing was removed", path)
	}
	return f, fi, nil
}

// alreadyAdopted reports whether every planned file is already in dest
// byte for byte, which makes a rerun after success a harmless no-op.
func alreadyAdopted(src, dest string, files []string) bool {
	if len(files) == 0 {
		return false // nothing to compare, so a dest cannot be proven ours
	}
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

// removeAdopted deletes only copied files whose bytes equal what the notes
// branch committed, then empty directories. A file edited after the copy is
// kept and named in the error. The rules file stays so a rerun honors it.
// A dir holding ignored or kept data survives.
func removeAdopted(src, wt, branch, base string, p *adoptPlan) error {
	var kept []string
	for _, rel := range p.Files {
		if rel == ignoreFileName {
			continue
		}
		from := filepath.Join(src, filepath.FromSlash(rel))
		same, err := sameAsCommitted(from, wt, branch+":"+base+"/"+rel)
		if err != nil || !same {
			kept = append(kept, rel)
			continue
		}
		if err := os.Remove(from); err != nil {
			return err
		}
	}
	for i := len(p.Dirs) - 1; i >= 0; i-- {
		// os.Remove refuses a non-empty dir, which is exactly the guard we want.
		_ = os.Remove(filepath.Join(src, filepath.FromSlash(p.Dirs[i])))
	}
	if len(kept) > 0 {
		return fmt.Errorf("kept %d file(s) in %s that differ from the notes copy (edited after copying): %s; rerun adopt after cleaning up",
			len(kept), src, strings.Join(kept, ", "))
	}
	return nil
}

// sameAsCommitted streams the file and the committed blob and compares them.
func sameAsCommitted(from, wt, spec string) (bool, error) {
	f, _, err := openRegular(from)
	if err != nil {
		return false, err
	}
	defer f.Close()
	cmd := exec.Command("git", "cat-file", "blob", spec)
	cmd.Dir = wt
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return false, err
	}
	if err := cmd.Start(); err != nil {
		return false, err
	}
	same, cmpErr := equalStreams(f, out)
	_, _ = io.Copy(io.Discard, out)
	if err := cmd.Wait(); err != nil {
		return false, err
	}
	return same, cmpErr
}

func equalStreams(a, b io.Reader) (bool, error) {
	ba, bb := make([]byte, 32<<10), make([]byte, 32<<10)
	for {
		na, ea := io.ReadFull(a, ba)
		nb, eb := io.ReadFull(b, bb)
		if na != nb || !bytes.Equal(ba[:na], bb[:nb]) {
			return false, nil
		}
		endA := ea == io.EOF || ea == io.ErrUnexpectedEOF
		endB := eb == io.EOF || eb == io.ErrUnexpectedEOF
		if (ea != nil && !endA) || (eb != nil && !endB) {
			return false, errors.Join(ea, eb)
		}
		if endA || endB {
			return endA && endB, nil
		}
	}
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
