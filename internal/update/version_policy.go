package update

import (
	"fmt"
	"strings"
)

type semanticVersion struct {
	major      string
	minor      string
	patch      string
	prerelease []string
	build      []string
}

func parseSemanticVersion(tag string) (semanticVersion, error) {
	if !strings.HasPrefix(tag, "v") {
		return semanticVersion{}, fmt.Errorf("must start with v")
	}
	versionText := strings.TrimPrefix(tag, "v")
	if versionText == "" {
		return semanticVersion{}, fmt.Errorf("missing version")
	}
	mainAndPre, build, hasBuild := strings.Cut(versionText, "+")
	if hasBuild && !validSemanticIdentifiers(build, false) {
		return semanticVersion{}, fmt.Errorf("invalid build metadata %q", build)
	}
	main, prerelease, hasPrerelease := strings.Cut(mainAndPre, "-")
	if hasPrerelease && !validSemanticIdentifiers(prerelease, true) {
		return semanticVersion{}, fmt.Errorf("invalid prerelease %q", prerelease)
	}
	parts := strings.Split(main, ".")
	if len(parts) != 3 {
		return semanticVersion{}, fmt.Errorf("must contain major.minor.patch")
	}
	major, err := parseSemanticNumber(parts[0])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("invalid major version: %w", err)
	}
	minor, err := parseSemanticNumber(parts[1])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("invalid minor version: %w", err)
	}
	patch, err := parseSemanticNumber(parts[2])
	if err != nil {
		return semanticVersion{}, fmt.Errorf("invalid patch version: %w", err)
	}
	parsed := semanticVersion{major: major, minor: minor, patch: patch}
	if hasPrerelease {
		parsed.prerelease = strings.Split(prerelease, ".")
	}
	if hasBuild {
		parsed.build = strings.Split(build, ".")
	}
	return parsed, nil
}

func parseSemanticNumber(value string) (string, error) {
	if !isNumericIdentifier(value) || (len(value) > 1 && value[0] == '0') {
		return "", fmt.Errorf("%q is not a canonical numeric identifier", value)
	}
	return value, nil
}

func validSemanticIdentifiers(value string, forbidLeadingZeroNumber bool) bool {
	if value == "" {
		return false
	}
	for _, identifier := range strings.Split(value, ".") {
		if identifier == "" {
			return false
		}
		for _, character := range identifier {
			if !((character >= '0' && character <= '9') || (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || character == '-') {
				return false
			}
		}
		if forbidLeadingZeroNumber && isNumericIdentifier(identifier) && len(identifier) > 1 && identifier[0] == '0' {
			return false
		}
	}
	return true
}

func (v semanticVersion) isPrerelease() bool {
	return len(v.prerelease) > 0
}

func (v semanticVersion) string() string {
	value := v.major + "." + v.minor + "." + v.patch
	if v.isPrerelease() {
		value += "-" + strings.Join(v.prerelease, ".")
	}
	if len(v.build) > 0 {
		value += "+" + strings.Join(v.build, ".")
	}
	return value
}

func (v semanticVersion) compare(other semanticVersion) int {
	if compared := compareSemanticNumber(v.major, other.major); compared != 0 {
		return compared
	}
	if compared := compareSemanticNumber(v.minor, other.minor); compared != 0 {
		return compared
	}
	if compared := compareSemanticNumber(v.patch, other.patch); compared != 0 {
		return compared
	}
	if !v.isPrerelease() && !other.isPrerelease() {
		return 0
	}
	if !v.isPrerelease() {
		return 1
	}
	if !other.isPrerelease() {
		return -1
	}
	for index := 0; index < len(v.prerelease) && index < len(other.prerelease); index++ {
		if compared := compareSemanticIdentifier(v.prerelease[index], other.prerelease[index]); compared != 0 {
			return compared
		}
	}
	return compareInt(len(v.prerelease), len(other.prerelease))
}

func compareSemanticIdentifier(first, second string) int {
	firstNumeric := isNumericIdentifier(first)
	secondNumeric := isNumericIdentifier(second)
	if firstNumeric && secondNumeric {
		return compareSemanticNumber(first, second)
	}
	if firstNumeric {
		return -1
	}
	if secondNumeric {
		return 1
	}
	return strings.Compare(first, second)
}

func isNumericIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

// compareSemanticNumber 比较无界、已校验且无前导零的 SemVer 数字标识符。
func compareSemanticNumber(first, second string) int {
	if len(first) < len(second) {
		return -1
	}
	if len(first) > len(second) {
		return 1
	}
	return strings.Compare(first, second)
}

func compareInt(first, second int) int {
	if first < second {
		return -1
	}
	if first > second {
		return 1
	}
	return 0
}
