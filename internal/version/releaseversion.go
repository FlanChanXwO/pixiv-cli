// Package version 提供构建期嵌入的版本元数据和 SemVer 解析、比较与渠道判定。
//
// [Version] 由发布构建通过 -ldflags -X 覆盖。
//
// 包的组成：[Version] 与 [Info] 是构建元数据，[Semantic] 与 [Parse] 处理已校验
// 版本，[Compare]、[Validate] 与 [Channel] 提供比较和分类。
package version

import (
	"fmt"
	"regexp"
	"strings"
)

var semanticVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

// Semantic 保存已校验的版本及其 precedence 字段；通过 Parse 构造。
type Semantic struct {
	text       string
	core       [3]string
	prerelease []string
}

// Parse 校验并解析不带 v 前缀的 SemVer，保留原始 build metadata。
func Parse(text string) (Semantic, error) {
	if err := Validate(text); err != nil {
		return Semantic{}, err
	}
	core, prerelease := splitVersion(text)
	return Semantic{text: text, core: core, prerelease: prerelease}, nil
}

// String 返回不带 v 前缀的 canonical 版本。
func (v Semantic) String() string { return v.text }

// IsPrerelease 报告该版本是否带有 prerelease 标识符。
func (v Semantic) IsPrerelease() bool { return len(v.prerelease) > 0 }

// Compare 比较已经通过 Parse 校验的版本，忽略 build metadata。
func (v Semantic) Compare(other Semantic) int {
	for index := range v.core {
		if order := compareNumeric(v.core[index], other.core[index]); order != 0 {
			return order
		}
	}
	return comparePrerelease(v.prerelease, other.prerelease)
}

// Validate 要求 version 是不带前导 v 的 semantic version。
func Validate(version string) error {
	if !semanticVersionPattern.MatchString(version) {
		return fmt.Errorf("version must be a semantic version without a leading v: %q", version)
	}
	return nil
}

// Compare 按 SemVer 2.0 precedence 比较两个 version：
// 返回负数、零或正数分别表示 a 低于、等于或高于 b。build metadata 不参与 precedence。
// 数字 identifier 按规范十进制字符串比较（先长度后字典序），因此任意长度都不会溢出。
func Compare(a, b string) (int, error) {
	first, err := Parse(a)
	if err != nil {
		return 0, err
	}
	second, err := Parse(b)
	if err != nil {
		return 0, err
	}
	return first.Compare(second), nil
}

// splitVersion 从已通过 Validate 的 version 拆出三个 core 数字与 prerelease
// identifiers；regexp 已保证 core 是三段规范十进制，这里不再重复校验。
func splitVersion(version string) ([3]string, []string) {
	var core [3]string
	main := version
	if index := strings.IndexAny(main, "-+"); index >= 0 {
		main = main[:index]
	}
	copy(core[:], strings.Split(main, "."))
	prerelease := ""
	if rest, found := strings.CutPrefix(version, main+"-"); found {
		prerelease, _, _ = strings.Cut(rest, "+")
	}
	return core, splitIdentifiers(prerelease)
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
