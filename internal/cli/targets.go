package cli

import (
	"fmt"

	"github.com/glincker/gitincognito/internal/project"
	"github.com/glincker/gitincognito/internal/registry"
	"github.com/glincker/gitincognito/internal/store"
)

func loadRegistry(c *Ctx) (*registry.Registry, error) {
	return registry.Load(store.RegistryPath(c.Home))
}

// targets returns every project with all=true, else the one containing cwd.
func targets(c *Ctx, reg *registry.Registry, all bool) ([]registry.Project, error) {
	if all {
		return reg.Projects, nil
	}
	info := project.Detect(c.Cwd)
	for _, p := range reg.Projects {
		if p.Path == info.Root {
			return []registry.Project{p}, nil
		}
	}
	return nil, fmt.Errorf("%s is not linked, run: git incognito link", info.Root)
}
