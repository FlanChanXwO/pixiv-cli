// Release tags add the required v prefix to the repository's shared SemVer policy.

package release

import (
	"fmt"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/version"
)

// ParseSemanticVersion validates a release tag with its required v prefix.
func ParseSemanticVersion(tag string) (version.Semantic, error) {
	text, ok := strings.CutPrefix(tag, "v")
	if !ok {
		return version.Semantic{}, fmt.Errorf("must start with v")
	}
	return version.Parse(text)
}
