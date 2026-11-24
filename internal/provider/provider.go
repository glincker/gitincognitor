package provider

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var ghRe = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(?:\.git)?/?$`)

func GitHubSlug(remote string) (string, bool) {
	m := ghRe.FindStringSubmatch(remote)
	if m == nil {
		return "", false
	}
	return m[1] + "/" + m[2], true
}

// CheckPrivate fails only when it can prove the remote is public. It cannot
// verify non-GitHub remotes or hosts without `gh`, and then it allows them.
func CheckPrivate(remote string) error {
	if os.Getenv("GITINCOGNITO_SKIP_PRIVATE_CHECK") == "1" {
		return nil
	}
	slug, ok := GitHubSlug(remote)
	if !ok {
		return nil
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return nil
	}
	out, err := exec.Command("gh", "repo", "view", slug, "--json", "isPrivate", "-q", ".isPrivate").Output()
	if err != nil {
		return nil
	}
	if strings.TrimSpace(string(out)) == "false" {
		return fmt.Errorf("remote %s is public", slug)
	}
	return nil
}
