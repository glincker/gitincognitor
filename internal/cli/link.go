package cli

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/glincker/gitincognito/internal/project"
	"github.com/glincker/gitincognito/internal/provider"
	"github.com/glincker/gitincognito/internal/registry"
	"github.com/glincker/gitincognito/internal/store"
)

func init() { commands["link"] = cmdLink }

type linkOpts struct {
	Store, Remote, Name string
	Guard               bool
}

func cmdLink(c *Ctx, args []string) error {
	fs := c.flags("link")
	var o linkOpts
	fs.StringVar(&o.Store, "store", "central", "central or dedicated")
	fs.StringVar(&o.Remote, "remote", "", "private remote URL (dedicated only)")
	fs.StringVar(&o.Name, "name", "", "project name (default: folder name)")
	fs.BoolVar(&o.Guard, "guard", false, "install pre-commit and pre-push guards")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := linkProject(c, o)
	if err != nil {
		return err
	}
	c.Emit(map[string]any{"ok": true, "project": p}, "linked "+p.Name+" -> "+filepath.Join(p.Path, ".incognito"))
	return nil
}

// linkProject is shared by link and adopt.
func linkProject(c *Ctx, o linkOpts) (registry.Project, error) {
	info := project.Detect(c.Cwd)
	name := o.Name
	if name == "" {
		name = info.Name
	}
	if err := project.ValidName(name); err != nil {
		return registry.Project{}, err
	}
	if info.IsGit && project.PointerTracked(info.Root) {
		return registry.Project{}, Unsafe("CLAUDE.local.md is tracked by this repo, untrack it first")
	}
	reg, err := registry.Load(store.RegistryPath(c.Home))
	if err != nil {
		return registry.Project{}, err
	}
	if e := reg.Find(name); e != nil && e.Path != info.Root {
		return registry.Project{}, Unsafe("a project named %s is already linked at %s, pass --name", name, e.Path)
	}
	var dir, notesRemote string
	switch o.Store {
	case "central":
		dir = store.CentralDir(c.Home)
		if !store.Exists(dir) {
			return registry.Project{}, errors.New("no store yet, run: git incognito init --remote <url>")
		}
		notesRemote = store.Remote(dir)
	case "dedicated":
		if o.Remote == "" {
			return registry.Project{}, errors.New("--remote is required with --store dedicated")
		}
		if err := store.CheckRemote(o.Remote); err != nil {
			return registry.Project{}, err
		}
		if err := provider.CheckPrivate(o.Remote); err != nil {
			return registry.Project{}, Unsafe("%v", err)
		}
		dir = store.DedicatedDir(c.Home, name)
		notesRemote = o.Remote
	default:
		return registry.Project{}, errors.New("--store must be central or dedicated")
	}
	if info.IsGit && project.SharesRemote(info.Root, notesRemote) {
		return registry.Project{}, Unsafe("notes remote %s is also a remote of this project", notesRemote)
	}
	if o.Store == "dedicated" {
		if store.Exists(dir) && store.Remote(dir) != o.Remote {
			return registry.Project{}, Unsafe("dedicated store for %s already uses remote %s", name, store.Remote(dir))
		}
		if err := store.Init(dir, o.Remote, ".gitincognito", "dedicated notes store\n"); err != nil {
			return registry.Project{}, err
		}
	}
	wt := filepath.Join(info.Root, ".incognito")
	if _, err := os.Stat(wt); err == nil {
		if _, err := os.Stat(filepath.Join(wt, ".git")); err != nil {
			return registry.Project{}, Unsafe(".incognito exists and is not a notes worktree, use: git incognito adopt")
		}
	} else if err := store.AddWorktree(dir, store.Branch(name), wt, true); err != nil {
		return registry.Project{}, err
	}
	if info.IsGit {
		if err := project.AddExclude(info.Root, ".incognito/", "CLAUDE.local.md"); err != nil {
			return registry.Project{}, err
		}
		if o.Guard {
			if err := project.InstallGuard(info.Root); err != nil {
				return registry.Project{}, err
			}
		}
	}
	if err := project.WritePointer(info.Root); err != nil {
		return registry.Project{}, err
	}
	p := registry.Project{Name: name, Path: info.Root, Store: o.Store, Branch: store.Branch(name), Remote: o.Remote}
	reg.Upsert(p)
	if err := reg.Save(store.RegistryPath(c.Home)); err != nil {
		return p, err
	}
	return p, store.CommitRegistry(store.CentralDir(c.Home), "chore: link "+name)
}
