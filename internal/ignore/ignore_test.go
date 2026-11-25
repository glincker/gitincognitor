package ignore_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/glincker/gitincognitor/internal/ignore"
)

func TestMatchUsesGitSemantics(t *testing.T) {
	rules := "*.md\n!keep.md\nref/\n/root-only.txt\n"
	paths := []string{"a.md", "keep.md", "ref/", "ref", "sub/root-only.txt", "root-only.txt", "x.go", "sub/b.md"}
	got, err := ignore.Match(rules, paths)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"a.md": true, "ref/": true, "root-only.txt": true, "sub/b.md": true}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for p := range want {
		if !got[p] {
			t.Fatalf("%s should match: %v", p, got)
		}
	}
}

func TestMatchEmptyInputs(t *testing.T) {
	got, err := ignore.Match("", nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("%v %v", got, err)
	}
	got, err = ignore.Match("*.md\n", []string{"weird name\n.md", "ü.md"})
	if err != nil || !got["weird name\n.md"] || !got["ü.md"] {
		t.Fatalf("%v %v", got, err)
	}
}

func TestMatchIgnoresGlobalExcludesFile(t *testing.T) {
	xdg := t.TempDir()
	if err := os.MkdirAll(filepath.Join(xdg, "git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "git", "ignore"), []byte("*.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("HOME", xdg)
	got, err := ignore.Match("*.txt\n", []string{"a.md", "b.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if got["a.md"] || !got["b.txt"] {
		t.Fatalf("global excludes leaked in: %v", got)
	}
}

func TestMatchIgnoresInheritedGitDir(t *testing.T) {
	other := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", other).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	got, err := ignore.Match("*.txt\n", []string{"a.md", "b.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if got["a.md"] || !got["b.txt"] {
		t.Fatalf("%v", got)
	}
	if _, err := os.Stat(filepath.Join(other, ".gitignore")); err == nil {
		t.Fatal("rules were written into the other repo")
	}
}

func TestMatcherServesManyLevels(t *testing.T) {
	m, err := ignore.NewMatcher("*.md\n!keep.md\nref/\n", "")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	for i := 0; i < 20; i++ {
		d := fmt.Sprintf("d%d", i)
		got, err := m.Match([]string{d + "/a.md", d + "/keep.md", d + "/ref/", d + "/x.go"})
		if err != nil {
			t.Fatal(err)
		}
		if !got[d+"/a.md"] || got[d+"/keep.md"] || !got[d+"/ref/"] || got[d+"/x.go"] {
			t.Fatalf("level %d: %v", i, got)
		}
	}
}
