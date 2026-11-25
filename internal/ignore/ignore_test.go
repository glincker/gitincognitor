package ignore_test

import (
	"testing"

	"github.com/glincker/gitincognito/internal/ignore"
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
