// Package releaseversion 统一拥有仓库级 release version identity。
package releaseversion

import (
	"fmt"
	"regexp"
	"strconv"
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
// 返回负数、零或正数分别表示 a 低于、等于或高于 b。build metadata 只用于
// 相等性之外的显示，不参与 precedence。
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
		if aCore[index] != bCore[index] {
			if aCore[index] < bCore[index] {
				return -1, nil
			}
			return 1, nil
		}
	}
	return comparePrerelease(aPre, bPre), nil
}

// splitVersion 把 version 拆成三个数字 core 与 prerelease identifiers。
func splitVersion(version string) ([3]uint64, []string, error) {
	var core [3]uint64
	if err := Validate(version); err != nil {
		return core, nil, err
	}
	main := version
	if index := strings.IndexAny(main, "-+"); index >= 0 {
		main = main[:index]
	}
	numeric, err := parseNumericIdentifiers(main, ".", 3)
	if err != nil {
		return core, nil, fmt.Errorf("version core %q: %w", main, err)
	}
	copy(core[:], numeric)
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
	aNumeric, aIsNumeric := numericIdentifier(a)
	bNumeric, bIsNumeric := numericIdentifier(b)
	switch {
	case aIsNumeric && bIsNumeric:
		if aNumeric == bNumeric {
			return 0, true
		}
		if aNumeric < bNumeric {
			return -1, false
		}
		return 1, false
	case aIsNumeric:
		return -1, false
	case bIsNumeric:
		return 1, false
	}
	return strings.Compare(a, b), a == b
}

// numericIdentifier 只在 identifier 无前导零时按数字处理，避免 "01" 与 "1"
// 被误判为相等；正则已排除该形式，这里仍显式保留判定。
func numericIdentifier(identifier string) (uint64, bool) {
	if identifier == "" || (len(identifier) > 1 && identifier[0] == '0') {
		return 0, false
	}
	value, err := strconv.ParseUint(identifier, 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

// splitIdentifiers 按 "." 拆分 identifier 列表，空串返回 nil。
func splitIdentifiers(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ".")
}

// parseNumericIdentifiers 解析恰好 count 个以 separator 分隔的十进制 identifier。
func parseNumericIdentifiers(value, separator string, count int) ([]uint64, error) {
	fields := strings.Split(value, separator)
	if len(fields) != count {
		return nil, fmt.Errorf("expected %d numeric identifiers, got %d", count, len(fields))
	}
	numeric := make([]uint64, count)
	for index, field := range fields {
		parsed, ok := numericIdentifier(field)
		if !ok {
			return nil, fmt.Errorf("identifier %q is not a canonical decimal number", field)
		}
		numeric[index] = parsed
	}
	return numeric, nil
}

// Channel 将已通过 Validate 的 version 分类为 stable 或 prerelease。
func Channel(version string) string {
	coreVersion, _, _ := strings.Cut(version, "+")
	if strings.Contains(coreVersion, "-") {
		return "prerelease"
	}
	return "stable"
}
