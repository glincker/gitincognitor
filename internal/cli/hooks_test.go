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
