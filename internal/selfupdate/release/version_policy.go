package release

import (
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/version"
)

// ParseSemanticVersion 校验带必需 v 前缀的 release tag，并返回去掉前缀的承诺版本。
func ParseSemanticVersion(tag string) (version.Semantic, error) {
	text, ok := strings.CutPrefix(tag, "v")
	if !ok {
		return version.Semantic{}, fmt.Errorf("must start with v")
	}
	return version.Parse(text)
}
