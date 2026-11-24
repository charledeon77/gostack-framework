package foundation

import (
	"os/exec"
	"strings"
	"testing"
)

// TestVersion_MatchesLatestGitTag ensures that the Version constant in version.go
// never drifts out of sync with the latest Git tag.
// If a developer or release process creates/bumps a Git tag without updating Version,
// or updates Version without tagging, this test will fail and prevent commits/releases.
func TestVersion_MatchesLatestGitTag(t *testing.T) {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	out, err := cmd.Output()
	if err != nil {
		t.Skip("Skipping git tag validation (not a git checkout or no tags present)")
		return
	}

	latestTag := strings.TrimSpace(string(out))
	if latestTag == "" {
		t.Skip("No git tags found to compare")
		return
	}

	if Version != latestTag {
		t.Fatalf("Version mismatch: foundation.Version is %q, but latest Git tag is %q. Update framework/foundation/version.go to match the tag.", Version, latestTag)
	}
}
