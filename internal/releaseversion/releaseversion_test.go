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

// 数字 identifier 不受 uint64 限制：长度更大即数值更大。溢出会静默降级为
// 字符串比较并给出错误顺序，因此必须用规范十进制串比较。
func TestCompareHandlesNumericIdentifiersBeyondUint64(t *testing.T) {
	t.Parallel()

	order, err := Compare("1.0.0-99999999999999999999", "1.0.0-100000000000000000000")
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	if order >= 0 {
		t.Errorf("Compare(1.0.0-99999999999999999999, 1.0.0-100000000000000000000) = %d, want negative", order)
	}

	// core 数字同样可能有巨大位数，且必须比较数值而不是字符串。
	order, err = Compare("99999999999999999999.0.0", "100000000000000000000.0.0")
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	if order >= 0 {
		t.Errorf("large core comparison = %d, want negative", order)
	}

	if order, err := Compare("1.0.0-100000000000000000000", "1.0.0-100000000000000000000"); err != nil || order != 0 {
		t.Errorf("identical large prerelease identifiers: order=%d err=%v, want 0/nil", order, err)
	}
}
