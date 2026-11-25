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
	"strconv"

	"github.com/glincker/gitincognitor/internal/git"
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

// quietExit carries an exit code for a command that already printed its report.
type quietExit struct{ code int }

func (e quietExit) Error() string { return "exit " + strconv.Itoa(e.code) }

func Unsafe(format string, a ...any) error { return &UnsafeError{Msg: fmt.Sprintf(format, a...)} }

func Home() string {
	if h := os.Getenv("GITINCOGNITOR_HOME"); h != "" {
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
	fmt.Fprintf(c.Err, "usage: git incognitor <command> [--json]\ncommands: %v\n", names)
	return 1
}

func (c *Ctx) fail(err error) int {
	var q quietExit
	if errors.As(err, &q) {
		return q.code
	}
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
