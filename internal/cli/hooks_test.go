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

func TestHooksInstallRefusesWrongTypesAndNonObjects(t *testing.T) {
	testutil.Setup(t)
	for _, body := range []string{`{"hooks":[]}`, `{"hooks":{"Stop":{}}}`, `{"hooks":"x"}`, `null`, `[]`, `"s"`} {
		d := t.TempDir()
		settings := filepath.Join(d, "settings.json")
		_ = os.WriteFile(settings, []byte(body), 0o644)
		if code, _ := run(t, d, "hooks", "install", "--claude", "--settings", settings); code == 0 {
			t.Fatalf("%s: want error", body)
		}
		if b, _ := os.ReadFile(settings); string(b) != body {
			t.Fatalf("%s: file must not change, got %s", body, b)
		}
	}
}

func TestHooksInstallCarriesHomeOnlyWhenSet(t *testing.T) {
	d := t.TempDir()
	settings := filepath.Join(d, "s.json")
	testutil.Setup(t)
	if code, out := run(t, d, "hooks", "install", "--claude", "--settings", settings); code != 0 {
		t.Fatal(out)
	}
	if b, _ := os.ReadFile(settings); !strings.Contains(string(b), "GITINCOGNITO_HOME=") {
		t.Fatalf("home not carried:\n%s", b)
	}
	t.Setenv("GITINCOGNITO_HOME", "")
	s2 := filepath.Join(d, "s2.json")
	if code, out := run(t, d, "hooks", "install", "--claude", "--settings", s2); code != 0 {
		t.Fatal(out)
	}
	if b, _ := os.ReadFile(s2); strings.Contains(string(b), "GITINCOGNITO_HOME") {
		t.Fatalf("home must not appear when unset:\n%s", b)
	}
}
