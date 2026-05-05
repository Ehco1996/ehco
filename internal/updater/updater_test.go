package updater

import (
	"testing"
	"time"
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

func TestParseBuildTime(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"2026-05-04T23:10:16Z", true},                   // goreleaser
		{"2026-05-04T23:10:16+08:00", true},              // RFC3339 w/ offset
		{"2026-05-04-23:10:16", true},                    // Makefile
		{"", false},
		{"not-a-time", false},
	}
	for _, c := range cases {
		_, ok := parseBuildTime(c.in)
		if ok != c.want {
			t.Errorf("parseBuildTime(%q) ok=%v want %v", c.in, ok, c.want)
		}
	}
}

func TestNightlyRepublished(t *testing.T) {
	built := "2026-05-04T23:10:16Z"
	older := time.Date(2026, 5, 4, 22, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 5, 5, 6, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		rel        ghRelease
		buildTime  string
		want       bool
	}{
		{"prerelease republished after build", ghRelease{Prerelease: true, PublishedAt: newer}, built, true},
		{"prerelease same/older than build", ghRelease{Prerelease: true, PublishedAt: older}, built, false},
		{"stable release ignored", ghRelease{Prerelease: false, PublishedAt: newer}, built, false},
		{"empty build time -> conservative false", ghRelease{Prerelease: true, PublishedAt: newer}, "", false},
		{"unparseable build time -> false", ghRelease{Prerelease: true, PublishedAt: newer}, "garbage", false},
	}
	for _, c := range cases {
		if got := nightlyRepublished(&c.rel, c.buildTime); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
