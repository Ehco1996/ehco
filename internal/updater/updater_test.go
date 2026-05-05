package updater

import (
	"testing"
)

func TestResolveChannel(t *testing.T) {
	cases := []struct {
		flag, current, want string
		wantErr             bool
	}{
		{"stable", "1.1.7-next", "stable", false},
		{"nightly", "1.1.6", "nightly", false},
		{"auto", "1.1.7-next", "nightly", false},
		{"auto", "1.1.6", "stable", false},
		{"", "1.1.6", "stable", false},
		{"bogus", "1.1.6", "", true},
	}
	for _, c := range cases {
		got, err := resolveChannel(c.flag, c.current)
		if (err != nil) != c.wantErr {
			t.Errorf("resolveChannel(%q,%q) err=%v wantErr=%v", c.flag, c.current, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("resolveChannel(%q,%q) = %q want %q", c.flag, c.current, got, c.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.1.6", "1.1.7", -1},
		{"1.1.7", "1.1.6", 1},
		{"1.1.6", "1.1.6", 0},
		{"v1.1.6", "1.1.6", 0},
		{"1.1.7-next", "1.1.7", -1}, // semver: prerelease < release
		{"1.1.7", "1.1.7-next", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestShaMatchesRevision(t *testing.T) {
	full := "1e0e74cabcdef0123456789abcdef0123456789a"
	cases := []struct {
		fullSHA, revision string
		want              bool
	}{
		{full, "1e0e74c", true},                    // goreleaser short
		{full, "1E0E74C", true},                    // case-insensitive
		{full, full, true},                         // Makefile full SHA
		{full, "deadbee", false},                   // different commit
		{full, "", false},                          // empty -> false (caller already guards)
		{full, full + "x", false},                  // longer than full SHA
		{"short", "shortish", false},               // revision longer than fullSHA
	}
	for _, c := range cases {
		if got := shaMatchesRevision(c.fullSHA, c.revision); got != c.want {
			t.Errorf("shaMatchesRevision(%q,%q)=%v want %v", c.fullSHA, c.revision, got, c.want)
		}
	}
}
