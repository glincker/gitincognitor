package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/glincker/gitincognitor/internal/testutil"
)

func TestVersionPlain(t *testing.T) {
	testutil.Setup(t)
	code, out := run(t, t.TempDir(), "version")
	if code != 0 || strings.TrimSpace(out) != "dev" {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestVersionJSON(t *testing.T) {
	testutil.Setup(t)
	code, out := run(t, t.TempDir(), "version", "--json")
	if code != 0 {
		t.Fatal(out)
	}
	var got struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.Version != "dev" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}
