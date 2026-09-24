// Package releaseversion 统一拥有仓库级 release version identity。
package releaseversion

import (
	"fmt"
	"regexp"
	"strings"
)

var semanticVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

// Validate 要求 version 是不带前导 v 的 semantic version。
func Validate(version string) error {
	if !semanticVersionPattern.MatchString(version) {
		return fmt.Errorf("version must be a semantic version without a leading v: %q", version)
	}
	return nil
}

// Compare 按 SemVer 2.0 precedence 比较两个已通过 Validate 的 version：
// 返回负数、零或正数分别表示 a 低于、等于或高于 b。build metadata 不参与 precedence。
// 数字 identifier 按规范十进制字符串比较（先长度后字典序），因此任意长度都不会溢出。
func Compare(a, b string) (int, error) {
	aCore, aPre, err := splitVersion(a)
	if err != nil {
		return 0, err
	}
	bCore, bPre, err := splitVersion(b)
	if err != nil {
		return 0, err
	}
	for index := range aCore {
		if order := compareNumeric(aCore[index], bCore[index]); order != 0 {
			return order, nil
		}
	}
	return comparePrerelease(aPre, bPre), nil
}

// splitVersion 把 version 拆成三个 core 数字与 prerelease identifiers；
// core 保留为规范化十进制字符串，比较时不经过定长整数。
func splitVersion(version string) ([3]string, []string, error) {
	var core [3]string
	if err := Validate(version); err != nil {
		return core, nil, err
	}
	main := version
	if index := strings.IndexAny(main, "-+"); index >= 0 {
		main = main[:index]
	}
	fields := strings.Split(main, ".")
	if len(fields) != 3 {
		return core, nil, fmt.Errorf("version core %q must have three numeric identifiers", main)
	}
	for index, field := range fields {
		if !isCanonicalDecimal(field) {
			return core, nil, fmt.Errorf("version core identifier %q is not a canonical decimal number", field)
		}
		core[index] = field
	}
	prerelease := ""
	if rest, found := strings.CutPrefix(version, main+"-"); found {
		prerelease, _, _ = strings.Cut(rest, "+")
	}
	return core, splitIdentifiers(prerelease), nil
}

// comparePrerelease 按 SemVer 规则比较 prerelease：无 prerelease 的版本更高，
// 数字 identifier 低于字母 identifier，identifier 数量多者更高。
func comparePrerelease(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1
	case len(b) == 0:
		return -1
	}
	for index := 0; index < len(a) && index < len(b); index++ {
		if order, equal := compareIdentifier(a[index], b[index]); !equal {
			return order
		}
	}
	return len(a) - len(b)
}

func compareIdentifier(a, b string) (order int, equal bool) {
	aIsNumeric := isCanonicalDecimal(a)
	bIsNumeric := isCanonicalDecimal(b)
	switch {
	case aIsNumeric && bIsNumeric:
		if order := compareNumeric(a, b); order != 0 {
			return order, false
		}
		return 0, true
	case aIsNumeric:
		return -1, false
	case bIsNumeric:
		return 1, false
	}
	return strings.Compare(a, b), a == b
}

// compareNumeric 比较两个规范十进制字符串：先比长度，再比字典序。
// 两者都不带前导零，所以长度更大则数值更大；这样任意位数都不会溢出。
func compareNumeric(a, b string) int {
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

// isCanonicalDecimal 只接受无前导零的纯十进制串，与 SemVer 对数字 identifier
// 的要求一致（"0" 合法，"01" 不合法）。
func isCanonicalDecimal(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return value == "0" || value[0] != '0'
}

// splitIdentifiers 按 "." 拆分 identifier 列表，空串返回 nil。
func splitIdentifiers(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ".")
}

// Channel 将已通过 Validate 的 version 分类为 stable 或 prerelease。
func Channel(version string) string {
	coreVersion, _, _ := strings.Cut(version, "+")
	if strings.Contains(coreVersion, "-") {
		return "prerelease"
	}
	return "stable"
}
