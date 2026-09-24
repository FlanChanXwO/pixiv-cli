package releaseversion

import "testing"

func TestValidateAndChannel(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		version string
		channel string
		valid   bool
	}{
		{version: "0.1.0", channel: "stable", valid: true},
		{version: "0.1.0+build-1", channel: "stable", valid: true},
		{version: "0.1.0-rc.1+build-1", channel: "prerelease", valid: true},
		{version: "v0.1.0", valid: false},
		{version: "1not-semver", valid: false},
	} {
		err := Validate(test.version)
		if !test.valid {
			if err == nil {
				t.Errorf("Validate(%q) error = nil, want rejection", test.version)
			}
			continue
		}
		if err != nil {
			t.Errorf("Validate(%q) error = %v", test.version, err)
			continue
		}
		if got := Channel(test.version); got != test.channel {
			t.Errorf("Channel(%q) = %q, want %q", test.version, got, test.channel)
		}
	}
}

// Compare 是 Homebrew recovery 判定“请求版本是否落后于当前 Formula”的唯一依据，
// 因此必须严格遵循 SemVer precedence，而不是字符串或 sort -V 比较。
func TestCompareFollowsSemverPrecedence(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		a, b string
		want int
	}{
		{a: "1.1.1", b: "1.1.1", want: 0},
		{a: "1.1.1+build-2", b: "1.1.1+build-1", want: 0},
		{a: "1.1.0", b: "1.1.1", want: -1},
		{a: "1.2.0", b: "1.1.1", want: 1},
		{a: "1.10.0", b: "1.9.0", want: 1},
		{a: "1.1.1-rc.1", b: "1.1.1", want: -1},
		{a: "1.1.1-rc.2", b: "1.1.1-rc.1", want: 1},
		{a: "1.1.1-rc.1", b: "1.1.1-alpha.1", want: 1},
		{a: "1.1.1-alpha.2", b: "1.1.1-alpha.10", want: -1},
		{a: "1.1.1-1", b: "1.1.1-alpha", want: -1},
		{a: "1.1.1-alpha", b: "1.1.1-alpha.1", want: -1},
	} {
		got, err := Compare(test.a, test.b)
		if err != nil {
			t.Errorf("Compare(%q, %q) error = %v", test.a, test.b, err)
			continue
		}
		if got != test.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", test.a, test.b, got, test.want)
		}
	}

	for _, invalid := range []string{"v1.1.1", "1.1", "01.1.1", "1.1.1-", ""} {
		if _, err := Compare(invalid, "1.1.1"); err == nil {
			t.Errorf("Compare(%q, ...) error = nil, want rejection", invalid)
		}
	}
}
