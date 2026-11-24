package cli

import (
	"strings"
	"testing"
)

func TestHookCommandQuotesBinaryAndHome(t *testing.T) {
	got := hookCommand("/opt/my tools/it's/git-incognito", "/h o/me")
	want := `GITINCOGNITO_HOME='/h o/me' '/opt/my tools/it'\''s/git-incognito' sync --quiet --all`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if got := hookCommand("/b", ""); got != `'/b' sync --quiet --all` {
		t.Fatalf("no home prefix expected, got %s", got)
	}
}

func TestPlistEscapesXMLAndCarriesHome(t *testing.T) {
	b := plistBody("/a & b/<x>", "/h&m")
	if !strings.Contains(b, "<string>/a &amp; b/&lt;x&gt;</string>") {
		t.Fatalf("bin not escaped:\n%s", b)
	}
	if !strings.Contains(b, "<key>EnvironmentVariables</key>") || !strings.Contains(b, "/h&amp;m") {
		t.Fatalf("home missing:\n%s", b)
	}
	if strings.Contains(plistBody("/a", ""), "EnvironmentVariables") {
		t.Fatal("no env block when home unset")
	}
}
