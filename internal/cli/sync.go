package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/glincker/gitincognito/internal/syncer"
)

func init() { commands["sync"] = cmdSync }

func cmdSync(c *Ctx, args []string) error {
	fs := c.flags("sync")
	all := fs.Bool("all", false, "sync every linked project")
	quiet := fs.Bool("quiet", false, "always exit 0 (for hooks)")
	force := fs.Bool("force", false, "ignore the debounce window")
	if err := fs.Parse(args); err != nil {
		return err
	}
	reg, err := loadRegistry(c)
	if err != nil {
		return swallow(*quiet, err)
	}
	ts, err := targets(c, reg, *all)
	if err != nil {
		return swallow(*quiet, err)
	}
	var results []syncer.Result
	var lines []string
	failed := false
	for _, p := range ts {
		r := syncer.Sync(c.Home, p, syncer.DebounceFromEnv(), time.Now(), *force)
		results = append(results, r)
		lines = append(lines, fmt.Sprintf("%s: committed=%v pushed=%v %s%s", r.Name, r.Committed, r.Pushed, r.Skipped, r.Error))
		failed = failed || r.Error != ""
	}
	if !*quiet {
		c.Emit(results, strings.Join(lines, "\n"))
	}
	if failed && !*quiet {
		return errors.New("sync finished with errors")
	}
	return nil
}

func swallow(quiet bool, err error) error {
	if quiet {
		return nil
	}
	return err
}
