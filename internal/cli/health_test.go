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
