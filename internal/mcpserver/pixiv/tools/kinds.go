// Entity kind values shared by the Pixiv MCP tool schemas and handlers.

package tools

import (
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// kindAll is the aggregate selector used by recommendation and artwork lists.
const kindAll = "all"

// contentNovel extends pixiv.SearchContentType for the latest timeline, whose
// published schema also accepts novels; the SDK search type has no such value.
const contentNovel pixiv.SearchContentType = "novel"

// Entity kinds reuse the published SDK reference values.
const (
	kindArtwork = string(pixiv.ReferenceKindArtwork)
	kindNovel   = string(pixiv.ReferenceKindNovel)
	kindUser    = string(pixiv.ReferenceKindUser)
)
