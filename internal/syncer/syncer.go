package syncer

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/glincker/gitincognito/internal/git"
	"github.com/glincker/gitincognito/internal/registry"
)

type Result struct {
	Name      string `json:"name"`
	Committed bool   `json:"committed"`
	Pushed    bool   `json:"pushed"`
	Skipped   string `json:"skipped,omitempty"`
	Error     string `json:"error,omitempty"`
}

// DebounceFromEnv reads the minimum gap between syncs. Default 60 seconds.
func DebounceFromEnv() time.Duration {
	secs := 60
	if v := os.Getenv("GITINCOGNITO_SYNC_DEBOUNCE_SECS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			secs = n
		}
	}
	return time.Duration(secs) * time.Second
}

func statePath(home, name string) string { return filepath.Join(home, "state", name+".last") }

func LastSync(home, name string) (time.Time, bool) {
	b, err := os.ReadFile(statePath(home, name))
	if err != nil {
		return time.Time{}, false
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(ts, 0), true
}

// Ahead counts local commits not on the remote (all commits when no upstream).
func Ahead(wt string) int {
	out, err := git.Run(wt, "rev-list", "--count", "@{u}..HEAD")
	if err != nil {
		out, err = git.Run(wt, "rev-list", "--count", "HEAD")
		if err != nil {
			return 0
		}
	}
	n, _ := strconv.Atoi(out)
	return n
}

// Sync commits notes and pushes. A push error leaves the commit local so the
// next run retries. The state file is written only on a clean run.
func Sync(home string, p registry.Project, debounce time.Duration, now time.Time, force bool) Result {
	r := Result{Name: p.Name}
	wt := filepath.Join(p.Path, ".incognito")
	if _, err := os.Stat(filepath.Join(wt, ".git")); err != nil {
		r.Skipped = "not linked"
		return r
	}
	if last, ok := LastSync(home, p.Name); ok && !force && now.Sub(last) < debounce {
		r.Skipped = "debounced"
		return r
	}
	st, err := git.Run(wt, "status", "--porcelain")
	if err != nil {
		r.Error = err.Error()
		return r
	}
	if st != "" {
		if _, err := git.Run(wt, "add", "-A"); err != nil {
			r.Error = err.Error()
			return r
		}
		if err := git.Commit(wt, "notes: sync "+now.UTC().Format(time.RFC3339)); err != nil {
			r.Error = err.Error()
			return r
		}
		r.Committed = true
	}
	if Ahead(wt) > 0 {
		if _, err := git.Run(wt, "push", "-q", "-u", "origin", p.Branch); err != nil {
			r.Error = err.Error()
			return r
		}
		r.Pushed = true
	}
	_ = os.MkdirAll(filepath.Dir(statePath(home, p.Name)), 0o755)
	_ = os.WriteFile(statePath(home, p.Name), []byte(strconv.FormatInt(now.Unix(), 10)), 0o644)
	return r
}
