package cli

import (
	"errors"

	"github.com/glincker/gitincognitor/internal/provider"
	"github.com/glincker/gitincognitor/internal/store"
)

func init() { commands["init"] = cmdInit }

func cmdInit(c *Ctx, args []string) error {
	fs := c.flags("init")
	remote := fs.String("remote", "", "private remote URL for the store (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *remote == "" {
		return errors.New("--remote is required")
	}
	if err := store.CheckRemote(*remote); err != nil {
		return err
	}
	if err := provider.CheckPrivate(*remote); err != nil {
		return Unsafe("%v", err)
	}
	dir := store.CentralDir(c.Home)
	if err := store.Init(dir, *remote, "registry.yml", "version: 1\nprojects: []\n"); err != nil {
		return err
	}
	c.Emit(map[string]any{"ok": true, "store": dir, "remote": *remote}, "store ready at "+dir)
	return nil
}
