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
	return []string{"-c", "user.name=gitincognitor", "-c", "user.email=gitincognitor@localhost"}
}

func Commit(dir, msg string) error {
	args := append(Identity(dir), "commit", "-q", "-m", msg)
	_, err := Run(dir, args...)
	return err
}
