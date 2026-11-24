# gitincognito v0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the `git-incognito` CLI from `docs/v0-spec.md`: private notes hidden inside any repo, backed up to a private remote, restorable after a crash.

**Architecture:** One Go binary shelling out to `git`. A central store repo holds one orphan branch per project, checked out as `<project>/.incognito/` via `git worktree add --orphan`. Hiding uses `.git/info/exclude`. Commands live in `internal/cli`, one file each, registered through a map. Domain logic lives in small packages (`git`, `registry`, `store`, `project`, `syncer`).

**Tech Stack:** Go 1.23+ (`os.CopyFS`), git 2.42+, `gopkg.in/yaml.v3`, stdlib `flag` and `testing`. No network in tests.

**Working rules for this repo:** no em or en dashes anywhere, commit format `<type>: <description>`, no co-author or AI attribution lines, never push `main`, work on branch `feat/v0-engine` in its own git worktree (remove the worktree when done).

---

## File structure

| File | Responsibility |
| --- | --- |
| `cmd/git-incognito/main.go` | Entry point, calls `cli.Run` |
| `internal/git/git.go` | `Run`, `HasRef`, `Commit`, `Identity`, `MinVersionOK` |
| `internal/testutil/testutil.go` | Test env, temp repos and bare remotes |
| `internal/registry/registry.go` | Registry model, load, save, `~` handling |
| `internal/provider/provider.go` | GitHub slug parse and best-effort private check |
| `internal/store/store.go` | Store init, clone, worktree add, registry commit |
| `internal/project/project.go` | Detect project, exclude, pointer, guard, remote collision |
| `internal/syncer/syncer.go` | Debounced commit and push |
| `internal/cli/cli.go` | `Ctx`, dispatcher, JSON, exit codes, helpers |
| `internal/cli/{init,link,sync,restore,status,doctor,adopt,unlink,hooks}.go` | One command each |
| `internal/cli/*_test.go` | Command tests (package `cli_test`) |

---

### Task 1: Scaffold, git runner, test helpers

**Files:**
- Create: `go.mod`, `.gitignore`, `internal/git/git.go`, `internal/git/git_test.go`, `internal/testutil/testutil.go`

- [ ] **Step 1: Create the branch and worktree, then init the module**

```bash
cd /Users/gdsks/G-Development/GLINRV6/gitincognito
git worktree add ../gitincognito-wt -b feat/v0-engine docs/v0-spec
cd ../gitincognito-wt
go version   # expect go1.23 or newer
go mod init github.com/glincker/gitincognito
printf 'bin/\n' > .gitignore
```

- [ ] **Step 2: Write the failing test** `internal/git/git_test.go`

```go
package git_test

import (
	"testing"

	"github.com/glincker/gitincognito/internal/git"
	"github.com/glincker/gitincognito/internal/testutil"
)

func TestRunAndHasRef(t *testing.T) {
	testutil.Setup(t)
	dir := testutil.Project(t, "p")
	if !git.HasRef(dir, "refs/heads/main") {
		t.Fatal("expected refs/heads/main")
	}
	if git.HasRef(dir, "refs/heads/nope") {
		t.Fatal("unexpected ref")
	}
	if _, err := git.Run(dir, "not-a-command"); err == nil {
		t.Fatal("expected error")
	}
}

func TestMinVersionOK(t *testing.T) {
	if err := git.MinVersionOK(); err != nil {
		t.Fatal(err)
	}
}

func TestCommitWithoutIdentity(t *testing.T) {
	testutil.Setup(t)
	dir := testutil.Project(t, "p")
	testutil.Git(t, dir, "commit", "-q", "--allow-empty", "-m", "x")
	if err := git.Commit(dir, "y"); err == nil {
		t.Fatal("nothing to commit should error")
	}
}
```

- [ ] **Step 3: Run it to verify it fails**

Run: `go test ./internal/git/ -v`
Expected: FAIL, package `git` and `testutil` do not exist.

- [ ] **Step 4: Implement** `internal/git/git.go`

```go
package git

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type Error struct {
	Args []string
	Out  string
	Err  error
}

func (e *Error) Error() string {
	return fmt.Sprintf("git %s: %v: %s", strings.Join(e.Args, " "), e.Err, e.Out)
}

// Run executes git in dir (may be empty) and returns trimmed combined output.
func Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, &Error{Args: args, Out: s, Err: err}
	}
	return s, nil
}

func HasRef(dir, ref string) bool {
	_, err := Run(dir, "show-ref", "--verify", "--quiet", ref)
	return err == nil
}

var verRe = regexp.MustCompile(`(\d+)\.(\d+)`)

// MinVersionOK enforces git 2.42, the first release with `worktree add --orphan`.
func MinVersionOK() error {
	out, err := Run("", "--version")
	if err != nil {
		return fmt.Errorf("git not found: %w", err)
	}
	m := verRe.FindStringSubmatch(out)
	if m == nil {
		return fmt.Errorf("cannot parse git version %q", out)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	if major < 2 || (major == 2 && minor < 42) {
		return fmt.Errorf("git 2.42 or newer required, found %q", out)
	}
	return nil
}

// Identity returns -c flags so commits work on machines with no git identity.
func Identity(dir string) []string {
	if e, _ := Run(dir, "config", "user.email"); e != "" {
		return nil
	}
	return []string{"-c", "user.name=gitincognito", "-c", "user.email=gitincognito@localhost"}
}

func Commit(dir, msg string) error {
	args := append(Identity(dir), "commit", "-q", "-m", msg)
	_, err := Run(dir, args...)
	return err
}
```

- [ ] **Step 5: Implement** `internal/testutil/testutil.go`

```go
package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/git"
)

// Setup isolates git config and points GITINCOGNITO_HOME at a temp dir.
func Setup(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("GITINCOGNITO_HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GITINCOGNITO_SYNC_DEBOUNCE_SECS", "0")
	return home
}

func Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := git.Run(dir, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func BareRemote(t *testing.T) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), "r.git")
	Git(t, "", "init", "-q", "--bare", "-b", "main", d)
	return d
}

// Project makes a git repo with one commit, path resolved through symlinks.
func Project(t *testing.T, name string) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	d, _ = filepath.EvalSymlinks(d)
	Git(t, d, "init", "-q", "-b", "main")
	Git(t, d, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init")
	return d
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/git/ -v`
Expected: PASS (3 tests).

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "feat: scaffold module, git runner and test helpers"
```

---

### Task 2: Registry

**Files:**
- Create: `internal/registry/registry.go`, `internal/registry/registry_test.go`
- Modify: `docs/v0-spec.md` (registry location line)

- [ ] **Step 1: Write the failing test** `internal/registry/registry_test.go`

```go
package registry_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glincker/gitincognito/internal/registry"
)

func TestRoundTripAndHomeShortening(t *testing.T) {
	home, _ := os.UserHomeDir()
	p := filepath.Join(t.TempDir(), "registry.yml")
	r := &registry.Registry{Version: 1}
	r.Upsert(registry.Project{Name: "a", Path: filepath.Join(home, "x", "a"), Store: "central", Branch: "proj/a"})
	if err := r.Save(p); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if !strings.Contains(string(raw), "~/x/a") {
		t.Fatalf("home not shortened:\n%s", raw)
	}
	got, err := registry.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Find("a").Path != filepath.Join(home, "x", "a") {
		t.Fatalf("home not expanded: %q", got.Find("a").Path)
	}
}

func TestMissingFileIsEmpty(t *testing.T) {
	r, err := registry.Load(filepath.Join(t.TempDir(), "none.yml"))
	if err != nil || r.Version != 1 || len(r.Projects) != 0 {
		t.Fatalf("got %+v %v", r, err)
	}
}

func TestUpsertAndRemove(t *testing.T) {
	r := &registry.Registry{Version: 1}
	r.Upsert(registry.Project{Name: "a", Path: "/1"})
	r.Upsert(registry.Project{Name: "a", Path: "/2"})
	if len(r.Projects) != 1 || r.Find("a").Path != "/2" {
		t.Fatalf("upsert failed: %+v", r.Projects)
	}
	r.Remove("a")
	if r.Find("a") != nil {
		t.Fatal("remove failed")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/registry/ -v`
Expected: FAIL, package missing.

- [ ] **Step 3: Implement** `internal/registry/registry.go`

```go
package registry

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Project struct {
	Name   string `yaml:"name"`
	Path   string `yaml:"path"`
	Store  string `yaml:"store"`
	Branch string `yaml:"branch"`
	Remote string `yaml:"remote,omitempty"`
}

type Registry struct {
	Version  int       `yaml:"version"`
	Projects []Project `yaml:"projects"`
}

func ExpandPath(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && (p == "~" || strings.HasPrefix(p, "~/")) {
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}

func ShortPath(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && (p == home || strings.HasPrefix(p, home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func Load(path string) (*Registry, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Registry{Version: 1}, nil
	}
	if err != nil {
		return nil, err
	}
	var r Registry
	if err := yaml.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	if r.Version == 0 {
		r.Version = 1
	}
	for i := range r.Projects {
		r.Projects[i].Path = ExpandPath(r.Projects[i].Path)
	}
	return &r, nil
}

func (r *Registry) Save(path string) error {
	out := Registry{Version: r.Version, Projects: make([]Project, len(r.Projects))}
	copy(out.Projects, r.Projects)
	for i := range out.Projects {
		out.Projects[i].Path = ShortPath(out.Projects[i].Path)
	}
	b, err := yaml.Marshal(&out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func (r *Registry) Find(name string) *Project {
	for i := range r.Projects {
		if r.Projects[i].Name == name {
			return &r.Projects[i]
		}
	}
	return nil
}

func (r *Registry) Upsert(p Project) {
	if e := r.Find(p.Name); e != nil {
		*e = p
		return
	}
	r.Projects = append(r.Projects, p)
}

func (r *Registry) Remove(name string) {
	out := r.Projects[:0]
	for _, p := range r.Projects {
		if p.Name != name {
			out = append(out, p)
		}
	}
	r.Projects = out
}
```

- [ ] **Step 4: Add the dependency, run tests**

Run: `go get gopkg.in/yaml.v3 && go test ./internal/registry/ -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Fix the spec's registry location**

In `docs/v0-spec.md`, replace ``` `~/.incognito/registry.yml`, also committed to the store's `main`: ``` with ``` `~/.incognito/store/registry.yml`, committed on the store's `main` (the store checkout is its only copy): ```.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: add registry with home path shortening"
```

---

### Task 3: Provider check, store, and `init`

**Files:**
- Create: `internal/provider/provider.go`, `internal/provider/provider_test.go`, `internal/store/store.go`, `internal/store/store_test.go`, `internal/cli/cli.go`, `internal/cli/init.go`, `internal/cli/helpers_test.go`, `internal/cli/init_test.go`, `cmd/git-incognito/main.go`

- [ ] **Step 1: Write failing tests**

`internal/provider/provider_test.go`:

```go
package provider_test

import (
	"testing"

	"github.com/glincker/gitincognito/internal/provider"
)

func TestGitHubSlug(t *testing.T) {
	cases := map[string]string{
		"git@github.com:glincker/notes.git":     "glincker/notes",
		"https://github.com/glincker/notes":     "glincker/notes",
		"https://github.com/glincker/notes.git": "glincker/notes",
	}
	for in, want := range cases {
		got, ok := provider.GitHubSlug(in)
		if !ok || got != want {
			t.Errorf("%s: got %q %v", in, got, ok)
		}
	}
	if _, ok := provider.GitHubSlug("/tmp/r.git"); ok {
		t.Error("local path must not match")
	}
}
```

`internal/store/store_test.go`:

```go
package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/store"
	"github.com/glincker/gitincognito/internal/testutil"
)

func TestInitPushesSeedMainAndIsIdempotent(t *testing.T) {
	testutil.Setup(t)
	remote := testutil.BareRemote(t)
	dir := filepath.Join(t.TempDir(), "store")
	for i := 0; i < 2; i++ {
		if err := store.Init(dir, remote, "registry.yml", "version: 1\n"); err != nil {
			t.Fatal(err)
		}
	}
	if got := testutil.Git(t, remote, "branch", "--list", "main"); got == "" {
		t.Fatal("seed main not pushed")
	}
}

func TestAddWorktreeOrphanThenRestoreFromRemote(t *testing.T) {
	testutil.Setup(t)
	remote := testutil.BareRemote(t)
	dir := filepath.Join(t.TempDir(), "store")
	if err := store.Init(dir, remote, "registry.yml", "version: 1\n"); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(t.TempDir(), "wt")
	if err := store.AddWorktree(dir, "proj/a", wt); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "n.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Git(t, wt, "add", "-A")
	testutil.Git(t, wt, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "n")
	testutil.Git(t, wt, "push", "-q", "-u", "origin", "proj/a")

	// Simulate a new machine: clone, then add the worktree from the remote branch.
	dir2 := filepath.Join(t.TempDir(), "store2")
	if err := store.Clone(dir2, remote); err != nil {
		t.Fatal(err)
	}
	wt2 := filepath.Join(t.TempDir(), "wt2")
	if err := store.AddWorktree(dir2, "proj/a", wt2); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(wt2, "n.md")); err != nil || string(b) != "hi" {
		t.Fatalf("restore failed: %q %v", b, err)
	}
}
```

`internal/cli/helpers_test.go`:

```go
package cli_test

import (
	"bytes"
	"testing"

	"github.com/glincker/gitincognito/internal/cli"
	"github.com/glincker/gitincognito/internal/testutil"
)

func run(t *testing.T, cwd string, args ...string) (int, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := cli.RunIn(cwd, args, &out, &errb)
	return code, out.String() + errb.String()
}

// linked returns an initialised store and a project already linked to it.
func linked(t *testing.T) (home, remote, proj string) {
	t.Helper()
	home = testutil.Setup(t)
	remote = testutil.BareRemote(t)
	proj = testutil.Project(t, "demo")
	if code, out := run(t, proj, "init", "--remote", remote); code != 0 {
		t.Fatal(out)
	}
	if code, out := run(t, proj, "link"); code != 0 {
		t.Fatal(out)
	}
	return
}
```

`internal/cli/init_test.go`:

```go
package cli_test

import (
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/testutil"
)

func TestInitRequiresRemote(t *testing.T) {
	testutil.Setup(t)
	if code, _ := run(t, t.TempDir(), "init"); code != 1 {
		t.Fatalf("code %d", code)
	}
}

func TestInitCreatesStore(t *testing.T) {
	home := testutil.Setup(t)
	remote := testutil.BareRemote(t)
	code, out := run(t, t.TempDir(), "init", "--remote", remote, "--json")
	if code != 0 {
		t.Fatal(out)
	}
	testutil.Git(t, filepath.Join(home, "store"), "rev-parse", "--git-dir")
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/... -v 2>&1 | tail -20`
Expected: FAIL, packages missing.

- [ ] **Step 3: Implement** `internal/provider/provider.go`

```go
package provider

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var ghRe = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(?:\.git)?/?$`)

func GitHubSlug(remote string) (string, bool) {
	m := ghRe.FindStringSubmatch(remote)
	if m == nil {
		return "", false
	}
	return m[1] + "/" + m[2], true
}

// CheckPrivate fails only when it can prove the remote is public. It cannot
// verify non-GitHub remotes or hosts without `gh`, and then it allows them.
func CheckPrivate(remote string) error {
	if os.Getenv("GITINCOGNITO_SKIP_PRIVATE_CHECK") == "1" {
		return nil
	}
	slug, ok := GitHubSlug(remote)
	if !ok {
		return nil
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return nil
	}
	out, err := exec.Command("gh", "repo", "view", slug, "--json", "isPrivate", "-q", ".isPrivate").Output()
	if err != nil {
		return nil
	}
	if strings.TrimSpace(string(out)) == "false" {
		return fmt.Errorf("remote %s is public", slug)
	}
	return nil
}
```

- [ ] **Step 4: Implement** `internal/store/store.go`

```go
package store

import (
	"os"
	"path/filepath"

	"github.com/glincker/gitincognito/internal/git"
)

func CentralDir(home string) string { return filepath.Join(home, "store") }

func DedicatedDir(home, name string) string { return filepath.Join(home, "dedicated", name) }

func RegistryPath(home string) string { return filepath.Join(CentralDir(home), "registry.yml") }

func Branch(name string) string { return "proj/" + name }

func Exists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func Remote(dir string) string {
	out, _ := git.Run(dir, "remote", "get-url", "origin")
	return out
}

// Init creates a store with a seed commit on main and pushes it, so the
// remote's HEAD never dangles. Safe to call again.
func Init(dir, remote, seedFile, seedBody string) error {
	if Exists(dir) {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if _, err := git.Run(dir, "init", "-q", "-b", "main"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, seedFile), []byte(seedBody), 0o644); err != nil {
		return err
	}
	if _, err := git.Run(dir, "add", "-A"); err != nil {
		return err
	}
	if err := git.Commit(dir, "chore: init store"); err != nil {
		return err
	}
	if _, err := git.Run(dir, "remote", "add", "origin", remote); err != nil {
		return err
	}
	_, err := git.Run(dir, "push", "-q", "-u", "origin", "main")
	return err
}

func Clone(dir, remote string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	_, err := git.Run("", "clone", "-q", remote, dir)
	return err
}

// AddWorktree checks out branch at path: local branch first, then the remote
// branch (restore), else a fresh orphan branch (first link).
func AddWorktree(storeDir, branch, path string) error {
	_, _ = git.Run(storeDir, "worktree", "prune")
	_, _ = git.Run(storeDir, "fetch", "-q", "origin")
	var err error
	switch {
	case git.HasRef(storeDir, "refs/heads/"+branch):
		_, err = git.Run(storeDir, "worktree", "add", "-q", path, branch)
	case git.HasRef(storeDir, "refs/remotes/origin/"+branch):
		_, err = git.Run(storeDir, "worktree", "add", "-q", "--track", "-b", branch, path, "origin/"+branch)
	default:
		_, err = git.Run(storeDir, "worktree", "add", "-q", "--orphan", "-b", branch, path)
	}
	return err
}

// CommitRegistry commits registry.yml on main and pushes best effort.
func CommitRegistry(dir, msg string) error {
	if _, err := git.Run(dir, "add", "registry.yml"); err != nil {
		return err
	}
	if _, err := git.Run(dir, "diff", "--cached", "--quiet"); err == nil {
		return nil
	}
	if err := git.Commit(dir, msg); err != nil {
		return err
	}
	_, _ = git.Run(dir, "push", "-q", "origin", "main")
	return nil
}
```

- [ ] **Step 5: Implement** `internal/cli/cli.go`

```go
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/glincker/gitincognito/internal/git"
)

type Ctx struct {
	Out, Err io.Writer
	JSON     bool
	Home     string
	Cwd      string
}

type cmdFunc func(c *Ctx, args []string) error

var commands = map[string]cmdFunc{}

// UnsafeError maps to exit code 2: a state that needs a human.
type UnsafeError struct{ Msg string }

func (e *UnsafeError) Error() string { return e.Msg }

func Unsafe(format string, a ...any) error { return &UnsafeError{Msg: fmt.Sprintf(format, a...)} }

func Home() string {
	if h := os.Getenv("GITINCOGNITO_HOME"); h != "" {
		return h
	}
	u, _ := os.UserHomeDir()
	return filepath.Join(u, ".incognito")
}

func Run(args []string, out, errw io.Writer) int {
	cwd, _ := os.Getwd()
	return RunIn(cwd, args, out, errw)
}

func RunIn(cwd string, args []string, out, errw io.Writer) int {
	c := &Ctx{Out: out, Err: errw, Home: Home(), Cwd: cwd}
	rest := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--json" {
			c.JSON = true
			continue
		}
		rest = append(rest, a)
	}
	if len(rest) == 0 {
		return c.usage()
	}
	fn, ok := commands[rest[0]]
	if !ok {
		return c.usage()
	}
	if err := git.MinVersionOK(); err != nil {
		return c.fail(err)
	}
	if err := fn(c, rest[1:]); err != nil {
		return c.fail(err)
	}
	return 0
}

func (c *Ctx) usage() int {
	names := make([]string, 0, len(commands))
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Fprintf(c.Err, "usage: git incognito <command> [--json]\ncommands: %v\n", names)
	return 1
}

func (c *Ctx) fail(err error) int {
	code := 1
	var u *UnsafeError
	if errors.As(err, &u) {
		code = 2
	}
	if c.JSON {
		_ = json.NewEncoder(c.Out).Encode(map[string]any{"ok": false, "error": err.Error(), "code": code})
	} else {
		fmt.Fprintln(c.Err, "error:", err)
	}
	return code
}

// Emit prints v as JSON in --json mode, else the human line.
func (c *Ctx) Emit(v any, human string) {
	if c.JSON {
		_ = json.NewEncoder(c.Out).Encode(v)
		return
	}
	fmt.Fprintln(c.Out, human)
}

func (c *Ctx) flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(c.Err)
	return fs
}
```

- [ ] **Step 6: Implement** `internal/cli/init.go`

```go
package cli

import (
	"errors"

	"github.com/glincker/gitincognito/internal/provider"
	"github.com/glincker/gitincognito/internal/store"
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
```

- [ ] **Step 7: Implement** `cmd/git-incognito/main.go`

```go
package main

import (
	"os"

	"github.com/glincker/gitincognito/internal/cli"
)

func main() { os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr)) }
```

- [ ] **Step 8: Run tests**

Run: `go vet ./... && go test ./... -v 2>&1 | tail -30`
Expected: all PASS.

- [ ] **Step 9: Commit**

```bash
git add -A && git commit -m "feat: add store, private-remote check and init command"
```

---

### Task 4: Project helpers and `link`

**Files:**
- Create: `internal/project/project.go`, `internal/project/project_test.go`, `internal/cli/link.go`, `internal/cli/link_test.go`

- [ ] **Step 1: Write failing tests**

`internal/project/project_test.go`:

```go
package project_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glincker/gitincognito/internal/git"
	"github.com/glincker/gitincognito/internal/project"
	"github.com/glincker/gitincognito/internal/testutil"
)

func TestExcludeIdempotentAndRemovable(t *testing.T) {
	testutil.Setup(t)
	root := testutil.Project(t, "p")
	for i := 0; i < 2; i++ {
		if err := project.AddExclude(root, ".incognito/", "CLAUDE.local.md"); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	if strings.Count(string(b), ".incognito/") != 1 {
		t.Fatalf("duplicated:\n%s", b)
	}
	if !project.Excluded(root, ".incognito/") {
		t.Fatal("Excluded false")
	}
	if err := project.RemoveExclude(root, ".incognito/"); err != nil {
		t.Fatal(err)
	}
	if project.Excluded(root, ".incognito/") {
		t.Fatal("still excluded")
	}
}

func TestPointerUpsertKeepsUserContent(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "CLAUDE.local.md")
	_ = os.WriteFile(f, []byte("my own notes\n"), 0o644)
	for i := 0; i < 2; i++ {
		if err := project.WritePointer(root); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(f)
	s := string(b)
	if !strings.Contains(s, "my own notes") || strings.Count(s, "gitincognito:start") != 1 {
		t.Fatalf("bad pointer file:\n%s", s)
	}
	if err := project.RemovePointer(root); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(f)
	if strings.Contains(string(b), "gitincognito") || !strings.Contains(string(b), "my own notes") {
		t.Fatalf("remove damaged file:\n%s", b)
	}
}

func TestSharesRemoteNormalizes(t *testing.T) {
	testutil.Setup(t)
	root := testutil.Project(t, "p")
	testutil.Git(t, root, "remote", "add", "origin", "git@github.com:glincker/Notes.git")
	if !project.SharesRemote(root, "https://github.com/glincker/notes") {
		t.Fatal("expected match")
	}
	if project.SharesRemote(root, "https://github.com/glincker/other") {
		t.Fatal("unexpected match")
	}
}

func TestGuardBlocksStagedNotes(t *testing.T) {
	testutil.Setup(t)
	root := testutil.Project(t, "p")
	if err := project.InstallGuard(root); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(root, ".incognito"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".incognito", "x.md"), []byte("x"), 0o644)
	testutil.Git(t, root, "add", "-f", ".incognito/x.md")
	_, err := git.Run(root, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "x")
	if err == nil {
		t.Fatal("guard should have blocked the commit")
	}
}

func TestDetectPlainFolder(t *testing.T) {
	d := t.TempDir()
	info := project.Detect(d)
	if info.IsGit || info.Root != d {
		t.Fatalf("%+v", info)
	}
}
```

`internal/cli/link_test.go`:

```go
package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glincker/gitincognito/internal/project"
	"github.com/glincker/gitincognito/internal/registry"
	"github.com/glincker/gitincognito/internal/store"
	"github.com/glincker/gitincognito/internal/testutil"
)

func TestLinkHidesNotesAndRegisters(t *testing.T) {
	home, _, proj := linked(t)
	if got := testutil.Git(t, proj, "status", "--porcelain"); got != "" {
		t.Fatalf("outer repo not clean: %q", got)
	}
	if _, err := os.Stat(filepath.Join(proj, ".incognito", ".git")); err != nil {
		t.Fatal("worktree missing")
	}
	if !project.Excluded(proj, ".incognito/") || !project.Excluded(proj, "CLAUDE.local.md") {
		t.Fatal("exclude entries missing")
	}
	b, _ := os.ReadFile(filepath.Join(proj, "CLAUDE.local.md"))
	if !strings.Contains(string(b), "gitincognito:start") {
		t.Fatal("pointer missing")
	}
	reg, _ := registry.Load(store.RegistryPath(home))
	if p := reg.Find("demo"); p == nil || p.Branch != "proj/demo" || p.Path != proj {
		t.Fatalf("registry: %+v", reg.Projects)
	}
}

func TestLinkIsIdempotent(t *testing.T) {
	_, _, proj := linked(t)
	if code, out := run(t, proj, "link"); code != 0 {
		t.Fatal(out)
	}
}

func TestLinkRefusesWhenNotesRemoteIsProjectRemote(t *testing.T) {
	testutil.Setup(t)
	remote := testutil.BareRemote(t)
	proj := testutil.Project(t, "demo")
	testutil.Git(t, proj, "remote", "add", "origin", remote)
	if code, out := run(t, proj, "init", "--remote", remote); code != 0 {
		t.Fatal(out)
	}
	if code, _ := run(t, proj, "link"); code != 2 {
		t.Fatalf("want exit 2, got %d", code)
	}
}

func TestLinkRefusesExistingPlainFolder(t *testing.T) {
	home := testutil.Setup(t)
	_ = home
	remote := testutil.BareRemote(t)
	proj := testutil.Project(t, "demo")
	run(t, proj, "init", "--remote", remote)
	_ = os.MkdirAll(filepath.Join(proj, ".incognito"), 0o755)
	if code, _ := run(t, proj, "link"); code != 2 {
		t.Fatalf("want exit 2, got %d", code)
	}
}

func TestLinkDedicatedStore(t *testing.T) {
	home, _, proj := linked(t)
	proj2 := testutil.Project(t, "other")
	remote2 := testutil.BareRemote(t)
	if code, out := run(t, proj2, "link", "--store", "dedicated", "--remote", remote2); code != 0 {
		t.Fatal(out)
	}
	if !store.Exists(store.DedicatedDir(home, "other")) {
		t.Fatal("dedicated store missing")
	}
	reg, _ := registry.Load(store.RegistryPath(home))
	if p := reg.Find("other"); p == nil || p.Store != "dedicated" || p.Remote != remote2 {
		t.Fatalf("registry: %+v", reg.Projects)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/project/ ./internal/cli/ 2>&1 | tail -15`
Expected: FAIL, package `project` and `link` missing.

- [ ] **Step 3: Implement** `internal/project/project.go`

```go
package project

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/glincker/gitincognito/internal/git"
)

type Info struct {
	Root  string
	IsGit bool
	Name  string
}

// Detect treats a plain folder as a project too (no exclude or guard needed).
func Detect(cwd string) Info {
	if out, err := git.Run(cwd, "rev-parse", "--show-toplevel"); err == nil {
		return Info{Root: out, IsGit: true, Name: filepath.Base(out)}
	}
	return Info{Root: cwd, Name: filepath.Base(cwd)}
}

func gitPath(root, rel string) (string, error) {
	out, err := git.Run(root, "rev-parse", "--git-path", rel)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}
	return out, nil
}

func AddExclude(root string, lines ...string) error {
	p, err := gitPath(root, "info/exclude")
	if err != nil {
		return err
	}
	b, _ := os.ReadFile(p)
	have := map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		have[strings.TrimSpace(l)] = true
	}
	var add []string
	for _, l := range lines {
		if !have[l] {
			add = append(add, l)
		}
	}
	if len(add) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	prefix := ""
	if len(b) > 0 && !strings.HasSuffix(string(b), "\n") {
		prefix = "\n"
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(prefix + strings.Join(add, "\n") + "\n")
	return err
}

func Excluded(root, line string) bool {
	p, err := gitPath(root, "info/exclude")
	if err != nil {
		return false
	}
	b, _ := os.ReadFile(p)
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) == line {
			return true
		}
	}
	return false
}

func RemoveExclude(root string, lines ...string) error {
	p, err := gitPath(root, "info/exclude")
	if err != nil {
		return err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	drop := map[string]bool{}
	for _, l := range lines {
		drop[l] = true
	}
	var keep []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if !drop[strings.TrimSpace(l)] {
			keep = append(keep, l)
		}
	}
	return os.WriteFile(p, []byte(strings.Join(keep, "\n")+"\n"), 0o644)
}

const (
	markStart   = "<!-- gitincognito:start -->"
	markEnd     = "<!-- gitincognito:end -->"
	pointerBody = "Private notes live in `.incognito/` (not tracked by this repo).\n" +
		"Read them for context and write new notes there, then run `git incognito sync`.\n" +
		"Other commands: `git incognito status`, `git incognito doctor` (all accept --json).\n"
)

func pointerBlock() string { return markStart + "\n" + pointerBody + markEnd + "\n" }

func splitPointer(s string) (before, after string, found bool) {
	i := strings.Index(s, markStart)
	j := strings.Index(s, markEnd)
	if i < 0 || j < i {
		return s, "", false
	}
	return s[:i], s[j+len(markEnd):], true
}

func WritePointer(root string) error {
	f := filepath.Join(root, "CLAUDE.local.md")
	b, _ := os.ReadFile(f)
	before, after, found := splitPointer(string(b))
	var out string
	if found {
		out = before + pointerBlock() + strings.TrimPrefix(after, "\n")
	} else {
		out = string(b)
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += pointerBlock()
	}
	return os.WriteFile(f, []byte(out), 0o644)
}

func RemovePointer(root string) error {
	f := filepath.Join(root, "CLAUDE.local.md")
	b, err := os.ReadFile(f)
	if err != nil {
		return nil
	}
	before, after, found := splitPointer(string(b))
	if !found {
		return nil
	}
	rest := before + strings.TrimPrefix(after, "\n")
	if strings.TrimSpace(rest) == "" {
		return os.Remove(f)
	}
	return os.WriteFile(f, []byte(rest), 0o644)
}

const guardMark = "# gitincognito-guard"

const preCommitBody = guardMark + `
if git diff --cached --name-only | grep -q '^\.incognito/'; then
  echo 'gitincognito: refusing to commit files under .incognito/' >&2
  exit 1
fi
`

const prePushBody = guardMark + `
if [ -n "$(git ls-files .incognito)" ]; then
  echo 'gitincognito: .incognito/ is tracked by this repo, refusing to push' >&2
  exit 1
fi
`

func InstallGuard(root string) error {
	dir, err := gitPath(root, "hooks")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, body := range map[string]string{"pre-commit": preCommitBody, "pre-push": prePushBody} {
		p := filepath.Join(dir, name)
		b, err := os.ReadFile(p)
		switch {
		case err != nil:
			if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
				return err
			}
		case !strings.Contains(string(b), guardMark):
			if err := os.WriteFile(p, []byte(string(b)+"\n"+body), 0o755); err != nil {
				return err
			}
		}
	}
	return nil
}

var (
	schemeRe = regexp.MustCompile(`^[a-z+]+://`)
	userRe   = regexp.MustCompile(`^[^@/]+@`)
)

func normalize(u string) string {
	u = strings.ToLower(strings.TrimSpace(u))
	u = strings.TrimSuffix(strings.TrimSuffix(u, "/"), ".git")
	u = schemeRe.ReplaceAllString(u, "")
	u = userRe.ReplaceAllString(u, "")
	return strings.Replace(u, ":", "/", 1)
}

// SharesRemote reports whether any remote of the project equals notesRemote.
func SharesRemote(root, notesRemote string) bool {
	out, err := git.Run(root, "remote", "-v")
	if err != nil {
		return false
	}
	want := normalize(notesRemote)
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && normalize(f[1]) == want {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Implement** `internal/cli/link.go`

```go
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
		if err := store.Init(dir, o.Remote, ".gitincognito", "dedicated notes store\n"); err != nil {
			return registry.Project{}, err
		}
	}
	wt := filepath.Join(info.Root, ".incognito")
	if _, err := os.Stat(wt); err == nil {
		if _, err := os.Stat(filepath.Join(wt, ".git")); err != nil {
			return registry.Project{}, Unsafe(".incognito exists and is not a notes worktree, use: git incognito adopt")
		}
	} else if err := store.AddWorktree(dir, store.Branch(name), wt); err != nil {
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
	reg, err := registry.Load(store.RegistryPath(c.Home))
	if err != nil {
		return p, err
	}
	reg.Upsert(p)
	if err := reg.Save(store.RegistryPath(c.Home)); err != nil {
		return p, err
	}
	return p, store.CommitRegistry(store.CentralDir(c.Home), "chore: link "+name)
}
```

- [ ] **Step 5: Run tests**

Run: `go vet ./... && go test ./... 2>&1 | tail -20`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: add project helpers, guard and link command"
```

---

### Task 5: `sync` with debounce and quiet mode

**Files:**
- Create: `internal/syncer/syncer.go`, `internal/syncer/syncer_test.go`, `internal/cli/targets.go`, `internal/cli/sync.go`, `internal/cli/sync_test.go`

- [ ] **Step 1: Write failing tests**

`internal/cli/sync_test.go`:

```go
package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/testutil"
)

func writeNote(t *testing.T, proj, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(proj, ".incognito", name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSyncCommitsAndPushes(t *testing.T) {
	_, remote, proj := linked(t)
	writeNote(t, proj, "a.md", "one")
	if code, out := run(t, proj, "sync"); code != 0 {
		t.Fatal(out)
	}
	if testutil.Git(t, remote, "branch", "--list", "proj/demo") == "" {
		t.Fatal("branch not on remote")
	}
	if got := testutil.Git(t, filepath.Join(proj, ".incognito"), "status", "--porcelain"); got != "" {
		t.Fatalf("not clean: %q", got)
	}
}

func TestSyncNoopWhenClean(t *testing.T) {
	_, _, proj := linked(t)
	if code, out := run(t, proj, "sync"); code != 0 {
		t.Fatal(out)
	}
}

func TestSyncDebounce(t *testing.T) {
	_, _, proj := linked(t)
	t.Setenv("GITINCOGNITO_SYNC_DEBOUNCE_SECS", "3600")
	writeNote(t, proj, "a.md", "one")
	run(t, proj, "sync")
	writeNote(t, proj, "b.md", "two")
	_, out := run(t, proj, "sync", "--json")
	if want := "debounced"; !contains(out, want) {
		t.Fatalf("want %q in %s", want, out)
	}
	if got := testutil.Git(t, filepath.Join(proj, ".incognito"), "status", "--porcelain"); got == "" {
		t.Fatal("b.md should still be uncommitted")
	}
	if code, out := run(t, proj, "sync", "--force"); code != 0 {
		t.Fatal(out)
	}
}

func TestQuietSyncSurvivesBrokenRemote(t *testing.T) {
	home, _, proj := linked(t)
	testutil.Git(t, filepath.Join(home, "store"), "remote", "set-url", "origin", "/nonexistent/x.git")
	writeNote(t, proj, "a.md", "one")
	if code, out := run(t, proj, "sync", "--quiet"); code != 0 {
		t.Fatalf("quiet must exit 0: %d %s", code, out)
	}
	if got := testutil.Git(t, filepath.Join(proj, ".incognito"), "log", "--oneline"); got == "" {
		t.Fatal("note should be committed locally")
	}
	if code, _ := run(t, proj, "sync"); code != 1 {
		t.Fatalf("non-quiet should exit 1 on push error, got %d", code)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (func() bool { return indexOf(s, sub) >= 0 })() }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/cli/ -run Sync 2>&1 | tail -10`
Expected: FAIL, `sync` command unknown (exit 1 usage).

- [ ] **Step 3: Implement** `internal/syncer/syncer.go`

```go
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
```

- [ ] **Step 4: Implement** `internal/cli/targets.go`

```go
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
```

- [ ] **Step 5: Implement** `internal/cli/sync.go`

```go
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
```

Note: `TestSyncDebounce` reads the JSON output; `--json` with `--force` absent prints `"skipped":"debounced"`, which the `contains` helper finds.

- [ ] **Step 6: Run tests**

Run: `go vet ./... && go test ./... 2>&1 | tail -20`
Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add -A && git commit -m "feat: add debounced sync with quiet mode"
```

---

### Task 6: `restore`

**Files:**
- Create: `internal/cli/restore.go`, `internal/cli/restore_test.go`

- [ ] **Step 1: Write the failing test** `internal/cli/restore_test.go`

```go
package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/project"
	"github.com/glincker/gitincognito/internal/testutil"
)

// Crash drill: lose the store and the project's .git, then get the notes back.
func TestRestoreAfterTotalLocalLoss(t *testing.T) {
	home, remote, proj := linked(t)
	writeNote(t, proj, "plan.md", "keep me")
	if code, out := run(t, proj, "sync"); code != 0 {
		t.Fatal(out)
	}
	if err := os.RemoveAll(home); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(proj); err != nil {
		t.Fatal(err)
	}
	// A fresh clone of the project appears at the same path.
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Git(t, proj, "init", "-q", "-b", "main")

	if code, out := run(t, proj, "restore", "--all", "--remote", remote); code != 0 {
		t.Fatal(out)
	}
	b, err := os.ReadFile(filepath.Join(proj, ".incognito", "plan.md"))
	if err != nil || string(b) != "keep me" {
		t.Fatalf("note not restored: %q %v", b, err)
	}
	if !project.Excluded(proj, ".incognito/") {
		t.Fatal("exclude not restored")
	}
	if got := testutil.Git(t, proj, "status", "--porcelain"); got != "" {
		t.Fatalf("outer repo dirty: %q", got)
	}
}

func TestRestoreSkipsMissingProjectPath(t *testing.T) {
	home, remote, proj := linked(t)
	if err := os.RemoveAll(proj); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(home, "store", ".git", "worktrees")); err != nil {
		t.Fatal(err)
	}
	code, out := run(t, t.TempDir(), "restore", "--all", "--remote", remote, "--json")
	if code != 0 {
		t.Fatal(out)
	}
	if !contains(out, "path missing") {
		t.Fatalf("want skip message: %s", out)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/cli/ -run Restore 2>&1 | tail -10`
Expected: FAIL, unknown command.

- [ ] **Step 3: Implement** `internal/cli/restore.go`

```go
package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/glincker/gitincognito/internal/project"
	"github.com/glincker/gitincognito/internal/registry"
	"github.com/glincker/gitincognito/internal/store"
)

func init() { commands["restore"] = cmdRestore }

type restoreRow struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func cmdRestore(c *Ctx, args []string) error {
	fs := c.flags("restore")
	remote := fs.String("remote", "", "store remote URL (needed when the store is missing)")
	all := fs.Bool("all", false, "restore every registered project")
	if err := fs.Parse(args); err != nil {
		return err
	}
	central := store.CentralDir(c.Home)
	if !store.Exists(central) {
		if *remote == "" {
			return errors.New("store missing, pass --remote <url>")
		}
		if err := store.Clone(central, *remote); err != nil {
			return err
		}
	}
	reg, err := loadRegistry(c)
	if err != nil {
		return err
	}
	ts, err := targets(c, reg, *all)
	if err != nil {
		return err
	}
	var rows []restoreRow
	var lines []string
	for _, p := range ts {
		st, err := restoreOne(c, p)
		if err != nil {
			st = "error: " + err.Error()
		}
		rows = append(rows, restoreRow{p.Name, st})
		lines = append(lines, fmt.Sprintf("%s: %s", p.Name, st))
	}
	c.Emit(rows, strings.Join(lines, "\n"))
	return nil
}

func restoreOne(c *Ctx, p registry.Project) (string, error) {
	if _, err := os.Stat(p.Path); err != nil {
		return "skipped: project path missing, clone it and rerun", nil
	}
	dir := store.CentralDir(c.Home)
	if p.Store == "dedicated" {
		dir = store.DedicatedDir(c.Home, p.Name)
		if !store.Exists(dir) {
			if err := store.Clone(dir, p.Remote); err != nil {
				return "", err
			}
		}
	}
	wt := filepath.Join(p.Path, ".incognito")
	if _, err := os.Stat(wt); err == nil {
		if _, err := os.Stat(filepath.Join(wt, ".git")); err != nil {
			return "skipped: .incognito exists and is not a notes worktree", nil
		}
	} else if err := store.AddWorktree(dir, p.Branch, wt); err != nil {
		return "", err
	}
	info := project.Detect(p.Path)
	if info.IsGit {
		if err := project.AddExclude(info.Root, ".incognito/", "CLAUDE.local.md"); err != nil {
			return "", err
		}
	}
	if err := project.WritePointer(p.Path); err != nil {
		return "", err
	}
	return "restored", nil
}
```

Note: in `TestRestoreSkipsMissingProjectPath` the central store is already present (only the project path is gone), so `restore` reaches `restoreOne` and reports the skip.

- [ ] **Step 4: Run tests**

Run: `go vet ./... && go test ./... 2>&1 | tail -20`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add restore command"
```

---

### Task 7: `status` and `doctor`

**Files:**
- Create: `internal/cli/status.go`, `internal/cli/doctor.go`, `internal/cli/health_test.go`

- [ ] **Step 1: Write failing tests** `internal/cli/health_test.go`

```go
package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/project"
)

func TestStatusReportsDirtyAndAhead(t *testing.T) {
	_, _, proj := linked(t)
	writeNote(t, proj, "a.md", "x")
	code, out := run(t, proj, "status", "--json")
	if code != 0 || !contains(out, `"dirty":true`) || !contains(out, `"linked":true`) {
		t.Fatalf("%d %s", code, out)
	}
	run(t, proj, "sync")
	_, out = run(t, proj, "status", "--json")
	if !contains(out, `"dirty":false`) || !contains(out, `"ahead":0`) {
		t.Fatalf("after sync: %s", out)
	}
}

func TestDoctorPassesOnHealthyProject(t *testing.T) {
	_, _, proj := linked(t)
	if code, out := run(t, proj, "doctor"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
}

func TestDoctorFailsWhenExcludeRemoved(t *testing.T) {
	_, _, proj := linked(t)
	if err := project.RemoveExclude(proj, ".incognito/"); err != nil {
		t.Fatal(err)
	}
	if code, out := run(t, proj, "doctor"); code != 2 {
		t.Fatalf("want 2, got %d: %s", code, out)
	}
}

func TestDoctorWarnsOnEnvFile(t *testing.T) {
	_, _, proj := linked(t)
	if err := os.WriteFile(filepath.Join(proj, ".incognito", ".env"), []byte("K=v"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out := run(t, proj, "doctor", "--json")
	if code != 0 || !contains(out, `"level":"warn"`) {
		t.Fatalf("%d %s", code, out)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/cli/ -run 'Status|Doctor' 2>&1 | tail -10`
Expected: FAIL, unknown commands.

- [ ] **Step 3: Implement** `internal/cli/status.go`

```go
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
```

- [ ] **Step 4: Implement** `internal/cli/doctor.go`

```go
package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/glincker/gitincognito/internal/git"
	"github.com/glincker/gitincognito/internal/project"
	"github.com/glincker/gitincognito/internal/store"
)

func init() { commands["doctor"] = cmdDoctor }

type check struct {
	Project string `json:"project,omitempty"`
	Name    string `json:"name"`
	Level   string `json:"level"`
	Detail  string `json:"detail,omitempty"`
}

func cmdDoctor(c *Ctx, args []string) error {
	reg, err := loadRegistry(c)
	if err != nil {
		return err
	}
	var checks []check
	add := func(proj, name, level, detail string) {
		checks = append(checks, check{proj, name, level, detail})
	}
	if err := git.MinVersionOK(); err != nil {
		add("", "git-version", "fail", err.Error())
	} else {
		add("", "git-version", "ok", "")
	}
	for _, p := range reg.Projects {
		wt := filepath.Join(p.Path, ".incognito")
		if _, err := os.Stat(p.Path); err != nil {
			add(p.Name, "path", "fail", "project path missing")
			continue
		}
		if _, err := os.Stat(filepath.Join(wt, ".git")); err != nil {
			add(p.Name, "linked", "fail", ".incognito is not a notes worktree, run: git incognito restore")
			continue
		}
		add(p.Name, "linked", "ok", "")
		info := project.Detect(p.Path)
		if info.IsGit {
			if project.Excluded(info.Root, ".incognito/") {
				add(p.Name, "excluded", "ok", "")
			} else {
				add(p.Name, "excluded", "fail", ".incognito/ missing from .git/info/exclude")
			}
			if out, _ := git.Run(info.Root, "ls-files", ".incognito"); out != "" {
				add(p.Name, "untracked", "fail", ".incognito files are tracked by the project repo")
			} else {
				add(p.Name, "untracked", "ok", "")
			}
			dir := store.CentralDir(c.Home)
			if p.Store == "dedicated" {
				dir = store.DedicatedDir(c.Home, p.Name)
			}
			if project.SharesRemote(info.Root, store.Remote(dir)) {
				add(p.Name, "remote", "fail", "notes remote is also a remote of the project")
			} else {
				add(p.Name, "remote", "ok", "")
			}
		}
		_ = filepath.WalkDir(wt, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() && d.Name() == ".git" {
				return filepath.SkipDir
			}
			if n := d.Name(); !d.IsDir() && (n == ".env" || strings.HasPrefix(n, ".env.")) {
				add(p.Name, "secrets", "warn", "env-like file in notes: "+n)
			}
			return nil
		})
	}
	failed := 0
	var lines []string
	for _, k := range checks {
		if k.Level == "fail" {
			failed++
		}
		lines = append(lines, fmt.Sprintf("[%s] %s %s %s", k.Level, k.Project, k.Name, k.Detail))
	}
	c.Emit(checks, strings.Join(lines, "\n"))
	if failed > 0 {
		return Unsafe("doctor found %d problem(s)", failed)
	}
	return nil
}
```

Note: when `doctor` fails in `--json` mode the output contains two JSON documents (the checks array, then the error object). That is intentional, parse the first for details.

- [ ] **Step 5: Run tests**

Run: `go vet ./... && go test ./... 2>&1 | tail -20`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add -A && git commit -m "feat: add status and doctor commands"
```

---

### Task 8: `adopt`

**Files:**
- Create: `internal/cli/adopt.go`, `internal/cli/adopt_test.go`

- [ ] **Step 1: Write failing tests** `internal/cli/adopt_test.go`

```go
package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/testutil"
)

func TestAdoptMovesFolderIntoNotes(t *testing.T) {
	testutil.Setup(t)
	remote := testutil.BareRemote(t)
	proj := testutil.Project(t, "demo")
	run(t, proj, "init", "--remote", remote)
	docs := filepath.Join(proj, "docs-local")
	_ = os.MkdirAll(filepath.Join(docs, "sub"), 0o755)
	_ = os.WriteFile(filepath.Join(docs, "a.md"), []byte("A"), 0o644)
	_ = os.WriteFile(filepath.Join(docs, "sub", "b.md"), []byte("B"), 0o644)

	if code, out := run(t, proj, "adopt", "docs-local"); code != 0 {
		t.Fatal(out)
	}
	if _, err := os.Stat(docs); !os.IsNotExist(err) {
		t.Fatal("original folder should be gone after a verified push")
	}
	b, err := os.ReadFile(filepath.Join(proj, ".incognito", "docs-local", "sub", "b.md"))
	if err != nil || string(b) != "B" {
		t.Fatalf("not moved: %q %v", b, err)
	}
	if testutil.Git(t, remote, "branch", "--list", "proj/demo") == "" {
		t.Fatal("notes not pushed")
	}
}

func TestAdoptKeepsOriginalWhenPushFails(t *testing.T) {
	home := testutil.Setup(t)
	remote := testutil.BareRemote(t)
	proj := testutil.Project(t, "demo")
	run(t, proj, "init", "--remote", remote)
	testutil.Git(t, filepath.Join(home, "store"), "remote", "set-url", "origin", "/nonexistent/x.git")
	docs := filepath.Join(proj, "docs-local")
	_ = os.MkdirAll(docs, 0o755)
	_ = os.WriteFile(filepath.Join(docs, "a.md"), []byte("A"), 0o644)
	if code, _ := run(t, proj, "adopt", "docs-local"); code == 0 {
		t.Fatal("expected failure")
	}
	if _, err := os.Stat(filepath.Join(docs, "a.md")); err != nil {
		t.Fatal("original must survive a failed push")
	}
}

func TestAdoptRefusesTrackedFolder(t *testing.T) {
	testutil.Setup(t)
	remote := testutil.BareRemote(t)
	proj := testutil.Project(t, "demo")
	run(t, proj, "init", "--remote", remote)
	_ = os.MkdirAll(filepath.Join(proj, "docs"), 0o755)
	_ = os.WriteFile(filepath.Join(proj, "docs", "a.md"), []byte("A"), 0o644)
	testutil.Git(t, proj, "add", "docs")
	if code, _ := run(t, proj, "adopt", "docs"); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
}

func TestAdoptWorksInPlainWorkspaceFolder(t *testing.T) {
	testutil.Setup(t)
	remote := testutil.BareRemote(t)
	ws := t.TempDir()
	run(t, ws, "init", "--remote", remote)
	_ = os.MkdirAll(filepath.Join(ws, "docs-local"), 0o755)
	_ = os.WriteFile(filepath.Join(ws, "docs-local", "a.md"), []byte("A"), 0o644)
	if code, out := run(t, ws, "adopt", "docs-local"); code != 0 {
		t.Fatal(out)
	}
	if _, err := os.Stat(filepath.Join(ws, ".incognito", "docs-local", "a.md")); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/cli/ -run Adopt 2>&1 | tail -10`
Expected: FAIL, unknown command.

- [ ] **Step 3: Implement** `internal/cli/adopt.go`

```go
package cli

import (
	"errors"
	"os"
	"path/filepath"
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
	if filepath.Base(src) == ".incognito" {
		return Unsafe("cannot adopt .incognito itself")
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
	r := syncer.Sync(c.Home, p, 0, time.Now(), true)
	if r.Error != "" || (!r.Pushed && syncer.Ahead(filepath.Join(p.Path, ".incognito")) > 0) {
		return errors.New("kept the original folder, notes not pushed yet: " + r.Error)
	}
	if err := os.RemoveAll(src); err != nil {
		return err
	}
	c.Emit(map[string]any{"ok": true, "adopted": dest}, "adopted "+src+" -> "+dest)
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go vet ./... && go test ./... 2>&1 | tail -20`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add adopt command"
```

---

### Task 9: `unlink`

**Files:**
- Create: `internal/cli/unlink.go`, `internal/cli/unlink_test.go`

- [ ] **Step 1: Write failing tests** `internal/cli/unlink_test.go`

```go
package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/project"
	"github.com/glincker/gitincognito/internal/registry"
	"github.com/glincker/gitincognito/internal/store"
	"github.com/glincker/gitincognito/internal/testutil"
)

func TestUnlinkRemovesWorktreeKeepsBranch(t *testing.T) {
	home, _, proj := linked(t)
	writeNote(t, proj, "a.md", "x")
	run(t, proj, "sync")
	if code, out := run(t, proj, "unlink"); code != 0 {
		t.Fatal(out)
	}
	if _, err := os.Stat(filepath.Join(proj, ".incognito")); !os.IsNotExist(err) {
		t.Fatal("worktree should be gone")
	}
	if project.Excluded(proj, ".incognito/") {
		t.Fatal("exclude should be removed")
	}
	if testutil.Git(t, filepath.Join(home, "store"), "branch", "--list", "proj/demo") == "" {
		t.Fatal("branch must be kept")
	}
	reg, _ := registry.Load(store.RegistryPath(home))
	if reg.Find("demo") != nil {
		t.Fatal("registry entry should be removed")
	}
}

func TestUnlinkRefusesDirtyNotes(t *testing.T) {
	_, _, proj := linked(t)
	writeNote(t, proj, "a.md", "unsynced")
	if code, _ := run(t, proj, "unlink"); code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	if code, out := run(t, proj, "unlink", "--force"); code != 0 {
		t.Fatal(out)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/cli/ -run Unlink 2>&1 | tail -10`
Expected: FAIL, unknown command.

- [ ] **Step 3: Implement** `internal/cli/unlink.go`

```go
package cli

import (
	"path/filepath"

	"github.com/glincker/gitincognito/internal/git"
	"github.com/glincker/gitincognito/internal/project"
	"github.com/glincker/gitincognito/internal/store"
)

func init() { commands["unlink"] = cmdUnlink }

func cmdUnlink(c *Ctx, args []string) error {
	fs := c.flags("unlink")
	force := fs.Bool("force", false, "discard unsynced notes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	reg, err := loadRegistry(c)
	if err != nil {
		return err
	}
	ts, err := targets(c, reg, false)
	if err != nil {
		return err
	}
	p := ts[0]
	wt := filepath.Join(p.Path, ".incognito")
	if st, _ := git.Run(wt, "status", "--porcelain"); st != "" && !*force {
		return Unsafe("notes have unsynced changes, run sync or pass --force")
	}
	dir := store.CentralDir(c.Home)
	if p.Store == "dedicated" {
		dir = store.DedicatedDir(c.Home, p.Name)
	}
	if _, err := git.Run(dir, "worktree", "remove", "--force", wt); err != nil {
		return err
	}
	if info := project.Detect(p.Path); info.IsGit {
		if err := project.RemoveExclude(info.Root, ".incognito/", "CLAUDE.local.md"); err != nil {
			return err
		}
	}
	if err := project.RemovePointer(p.Path); err != nil {
		return err
	}
	reg.Remove(p.Name)
	if err := reg.Save(store.RegistryPath(c.Home)); err != nil {
		return err
	}
	if err := store.CommitRegistry(store.CentralDir(c.Home), "chore: unlink "+p.Name); err != nil {
		return err
	}
	c.Emit(map[string]any{"ok": true, "unlinked": p.Name, "branch_kept": p.Branch}, "unlinked "+p.Name+", branch "+p.Branch+" kept")
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go vet ./... && go test ./... 2>&1 | tail -20`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add unlink command"
```

---

### Task 10: `hooks install`

**Files:**
- Create: `internal/cli/hooks.go`, `internal/cli/hooks_test.go`

Tests only ever pass `--settings` and `--plist` temp paths. Never run this against the real `~/.claude/settings.json` in tests.

- [ ] **Step 1: Write failing tests** `internal/cli/hooks_test.go`

```go
package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glincker/gitincognito/internal/testutil"
)

func TestHooksInstallMergesStopHookIdempotently(t *testing.T) {
	testutil.Setup(t)
	d := t.TempDir()
	settings := filepath.Join(d, "settings.json")
	_ = os.WriteFile(settings, []byte(`{"theme":"dark","hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo hi"}]}]}}`), 0o644)
	for i := 0; i < 2; i++ {
		code, out := run(t, d, "hooks", "install", "--claude", "--settings", settings)
		if code != 0 {
			t.Fatal(out)
		}
	}
	b, _ := os.ReadFile(settings)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["theme"] != "dark" {
		t.Fatal("existing settings lost")
	}
	stop := m["hooks"].(map[string]any)["Stop"].([]any)
	if len(stop) != 2 || strings.Count(string(b), "sync --quiet --all") != 1 {
		t.Fatalf("want 2 Stop entries, one ours:\n%s", b)
	}
	if _, err := os.Stat(settings + ".bak"); err != nil {
		t.Fatal("backup missing")
	}
}

func TestHooksInstallPrintDoesNotWrite(t *testing.T) {
	testutil.Setup(t)
	d := t.TempDir()
	settings := filepath.Join(d, "settings.json")
	if code, out := run(t, d, "hooks", "install", "--claude", "--print", "--settings", settings); code != 0 {
		t.Fatal(out)
	}
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		t.Fatal("--print must not write")
	}
}

func TestHooksInstallWritesPlist(t *testing.T) {
	testutil.Setup(t)
	d := t.TempDir()
	plist := filepath.Join(d, "x.plist")
	if code, out := run(t, d, "hooks", "install", "--launchd", "--plist", plist); code != 0 {
		t.Fatal(out)
	}
	b, _ := os.ReadFile(plist)
	if !strings.Contains(string(b), "<string>--quiet</string>") || !strings.Contains(string(b), "StartInterval") {
		t.Fatalf("bad plist:\n%s", b)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/cli/ -run Hooks 2>&1 | tail -10`
Expected: FAIL, unknown command.

- [ ] **Step 3: Implement** `internal/cli/hooks.go`

```go
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

func init() { commands["hooks"] = cmdHooks }

// timerSecs is the launchd interval, overridable for people who want it tighter.
func timerSecs() int {
	if n, err := strconv.Atoi(os.Getenv("GITINCOGNITO_TIMER_SECS")); err == nil && n > 0 {
		return n
	}
	return 900
}

func cmdHooks(c *Ctx, args []string) error {
	if len(args) == 0 || args[0] != "install" {
		return errors.New("usage: git incognito hooks install [--claude] [--launchd] [--print]")
	}
	fs := c.flags("hooks install")
	claude := fs.Bool("claude", false, "add a Claude Code Stop hook")
	launchd := fs.Bool("launchd", false, "write a launchd timer plist (macOS)")
	dry := fs.Bool("print", false, "show what would change, write nothing")
	home, _ := os.UserHomeDir()
	settings := fs.String("settings", filepath.Join(home, ".claude", "settings.json"), "Claude settings path")
	plist := fs.String("plist", filepath.Join(home, "Library", "LaunchAgents", "dev.glincker.gitincognito.plist"), "plist path")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if !*claude && !*launchd {
		*claude, *launchd = true, true
	}
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := bin + " sync --quiet --all"
	var done []string
	if *claude {
		msg, err := installClaudeHook(*settings, cmd, *dry)
		if err != nil {
			return err
		}
		done = append(done, msg)
	}
	if *launchd {
		body := plistBody(bin)
		if *dry {
			done = append(done, "would write "+*plist)
		} else {
			if err := os.MkdirAll(filepath.Dir(*plist), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(*plist, []byte(body), 0o644); err != nil {
				return err
			}
			done = append(done, "wrote "+*plist+", load it with: launchctl load "+*plist)
		}
	}
	c.Emit(map[string]any{"ok": true, "actions": done}, fmt.Sprintf("%v", done))
	return nil
}

func installClaudeHook(path, cmd string, dry bool) (string, error) {
	m := map[string]any{}
	raw, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(raw, &m); err != nil {
			return "", fmt.Errorf("%s is not valid JSON: %w", path, err)
		}
	}
	hooks, _ := m["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	stop, _ := hooks["Stop"].([]any)
	for _, e := range stop {
		if b, _ := json.Marshal(e); string(b) != "" && contains(string(b), cmd) {
			return "Stop hook already present in " + path, nil
		}
	}
	stop = append(stop, map[string]any{"hooks": []any{map[string]any{"type": "command", "command": cmd}}})
	hooks["Stop"] = stop
	m["hooks"] = hooks
	if dry {
		return "would add Stop hook to " + path, nil
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	if raw != nil {
		if err := os.WriteFile(path+".bak", raw, 0o600); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return "added Stop hook to " + path, os.WriteFile(path, append(out, '\n'), 0o644)
}

func plistBody(bin string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>dev.glincker.gitincognito</string>
<key>ProgramArguments</key><array><string>%s</string><string>sync</string><string>--quiet</string><string>--all</string></array>
<key>StartInterval</key><integer>%d</integer>
<key>RunAtLoad</key><true/>
</dict></plist>
`, bin, timerSecs())
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

The test file also defines a `contains` helper in package `cli_test`, which is a different package from `cli`, so there is no clash.

- [ ] **Step 4: Run tests**

Run: `go vet ./... && go test ./... 2>&1 | tail -20`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "feat: add hooks install for Stop hook and launchd timer"
```

---

### Task 11: README, license, CI, end-to-end drill

**Files:**
- Create: `README.md`, `LICENSE`, `.github/workflows/ci.yml`, `internal/cli/e2e_test.go`

- [ ] **Step 1: Write the end-to-end test** `internal/cli/e2e_test.go`

```go
package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognito/internal/testutil"
)

// Whole story: init, adopt existing notes, crash, restore, doctor is green.
func TestEndToEndCrashDrill(t *testing.T) {
	home := testutil.Setup(t)
	remote := testutil.BareRemote(t)
	proj := testutil.Project(t, "demo")
	for _, args := range [][]string{{"init", "--remote", remote}} {
		if code, out := run(t, proj, args...); code != 0 {
			t.Fatal(out)
		}
	}
	_ = os.MkdirAll(filepath.Join(proj, "docs-local"), 0o755)
	_ = os.WriteFile(filepath.Join(proj, "docs-local", "idea.md"), []byte("idea"), 0o644)
	if code, out := run(t, proj, "adopt", "docs-local"); code != 0 {
		t.Fatal(out)
	}
	_ = os.RemoveAll(home)
	_ = os.RemoveAll(proj)
	_ = os.MkdirAll(proj, 0o755)
	testutil.Git(t, proj, "init", "-q", "-b", "main")
	if code, out := run(t, proj, "restore", "--all", "--remote", remote); code != 0 {
		t.Fatal(out)
	}
	if b, err := os.ReadFile(filepath.Join(proj, ".incognito", "docs-local", "idea.md")); err != nil || string(b) != "idea" {
		t.Fatalf("lost the note: %q %v", b, err)
	}
	if code, out := run(t, proj, "doctor"); code != 0 {
		t.Fatalf("doctor red after restore: %s", out)
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/cli/ -run EndToEnd -v 2>&1 | tail -10`
Expected: PASS.

- [ ] **Step 3: Write `LICENSE`** (MIT, copyright "GLINCKER"), `README.md`, and CI

`README.md` content: one paragraph on the problem, a five-line quickstart (`git incognito init --remote <private-url>`, `cd project && git incognito link`, `git incognito adopt docs-local`, `git incognito sync`, `git incognito restore --all --remote <url>`), the command table from the spec, the safety rules, and the requirement "git 2.42 or newer". Keep it under 80 lines, no dashes of any kind.

`.github/workflows/ci.yml`:

```yaml
name: ci
on:
  pull_request:
  push:
    branches: [main]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - run: git --version
      - run: go vet ./...
      - run: go test ./...
```

- [ ] **Step 4: Full verification**

Run: `go vet ./... && go test ./... && go build -o bin/git-incognito ./cmd/git-incognito && grep -rnP '[\x{2013}\x{2014}]' . --include='*.go' --include='*.md' --include='*.yml' || echo "no dashes"`
Expected: tests PASS, binary builds, "no dashes".

- [ ] **Step 5: Dogfood check (read only against real data)**

Do not run `adopt` on the real `docs-local` inside this task. After the plan is merged and the private store remote exists, the owner runs `git incognito init --remote <private-url>`, then `cd ~/G-Development/GLINRV6 && git incognito adopt docs-local`, which works because the workspace folder is a plain folder.

- [ ] **Step 6: Commit, push the branch, remove the worktree**

```bash
git add -A && git commit -m "feat: add README, license, CI and end-to-end drill"
git push -u origin feat/v0-engine
cd ../gitincognito && git worktree remove ../gitincognito-wt && git worktree prune
```

Do not open a PR until the owner says so.

---

## Self-review against the spec

| Spec item | Task |
| --- | --- |
| Central store, orphan branch per project, real folder | 3, 4 |
| `info/exclude`, no tracked-file edits | 4 |
| Dedicated store variant | 4 (`--store dedicated`) |
| `init` seed `main`, private check | 3 |
| `link`, idempotent, pointer, registry | 4 |
| `adopt` (copy, verify push, then delete) | 8 |
| `sync`, never force-push, no-op when clean | 5 |
| Auto-sync: quiet, debounce env, retry | 5 (`--quiet`, `GITINCOGNITO_SYNC_DEBOUNCE_SECS`) |
| `restore` (clone store, rebuild worktrees, prune) | 6 |
| `status`, `doctor` (excluded, untracked, shared remote, `.env` warn) | 7 |
| `unlink` keeps branch | 9 |
| `hooks install` explicit opt-in | 10 |
| Safety rule 3 guards | 4 (`--guard`) |
| `--json` everywhere, exit codes 0/1/2 | 3 (`cli.go`) |
| Plain workspace folders like docs-local | 4 (`Detect`), 8 test |
| Crash drill | 6, 11 |

Known limits to carry into issues, not v0: the pre-commit guard appends to an existing hook, so an earlier `exit 0` in that hook can bypass it; `doctor --json` on failure prints two JSON documents; Windows is untested.
