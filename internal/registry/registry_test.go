package registry_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glincker/gitincognitor/internal/registry"
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
