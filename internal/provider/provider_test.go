package provider_test

import (
	"testing"

	"github.com/glincker/gitincognitor/internal/provider"
)

func TestGitHubSlug(t *testing.T) {
	cases := map[string]string{
		"git@github.com:glincker/notes.git":     "glincker/notes",
		"https://github.com/glincker/notes":     "glincker/notes",
		"https://github.com/glincker/notes.git": "glincker/notes",
	}
	for in, want := range cases {
		got, ok := provider.GitHubSlug(in)
		if !ok || got != want {
			t.Errorf("%s: got %q %v", in, got, ok)
		}
	}
	if _, ok := provider.GitHubSlug("/tmp/r.git"); ok {
		t.Error("local path must not match")
	}
}
