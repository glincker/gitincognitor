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
