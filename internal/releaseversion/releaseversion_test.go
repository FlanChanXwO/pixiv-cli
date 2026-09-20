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
