package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glincker/gitincognito/internal/git"
	"github.com/glincker/gitincognito/internal/syncer"
)

func init() { commands["status"] = cmdStatus }

type statusRow struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Linked   bool   `json:"linked"`
	Dirty    bool   `json:"dirty"`
	Ahead    int    `json:"ahead"`
	LastSync string `json:"last_sync,omitempty"`
}

func cmdStatus(c *Ctx, args []string) error {
	fs := c.flags("status")
	all := fs.Bool("all", false, "all projects")
	if err := fs.Parse(args); err != nil {
		return err
	}
	reg, err := loadRegistry(c)
	if err != nil {
		return err
	}
	ts, err := targets(c, reg, *all)
	if err != nil {
		return err
	}
	rows := make([]statusRow, 0, len(ts))
	lines := make([]string, 0, len(ts))
	for _, p := range ts {
		wt := filepath.Join(p.Path, ".incognito")
		r := statusRow{Name: p.Name, Path: p.Path}
		if _, err := os.Stat(filepath.Join(wt, ".git")); err == nil {
			r.Linked = true
			st, _ := git.Run(wt, "status", "--porcelain")
			r.Dirty = st != ""
			r.Ahead = syncer.Ahead(wt)
		}
		if t, ok := syncer.LastSync(c.Home, p.Name); ok {
			r.LastSync = t.UTC().Format(time.RFC3339)
		}
		rows = append(rows, r)
		lines = append(lines, fmt.Sprintf("%s: linked=%v dirty=%v ahead=%d last=%s", r.Name, r.Linked, r.Dirty, r.Ahead, r.LastSync))
	}
	c.Emit(rows, strings.Join(lines, "\n"))
	return nil
}
