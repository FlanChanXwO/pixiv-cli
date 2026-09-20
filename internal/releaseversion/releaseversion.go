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

// Channel 将已通过 Validate 的 version 分类为 stable 或 prerelease。
func Channel(version string) string {
	coreVersion, _, _ := strings.Cut(version, "+")
	if strings.Contains(coreVersion, "-") {
		return "prerelease"
	}
	return "stable"
}
