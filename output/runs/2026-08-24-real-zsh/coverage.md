# Coverage

| Capability | Cases | Status | Notes |
| --- | --- | --- | --- |
| Actual command syntax | `help/*` | PASS | 35 help surfaces from the built binary; all contain Usage and have empty stderr. |
| Authentication diagnosis | `auth/list`, `auth/check` | PASS | Isolated account ID `25649510`; output contains safe metadata only. |
| Artwork discovery | `discovery/search-artwork`, `search-artwork-ndjson`, `search-ugoira`, `trending-tags`, `ranking-day` | PASS | Positive IDs; valid JSON/NDJSON; Ugoira sample discovered for download. |
| Novel discovery | `discovery/novel-search` | PASS | Three or fewer current novel results with positive IDs. |
| User discovery | `discovery/user-search` | PASS | Authenticated `user_previews` with positive IDs. |
| Recommendations | `recommendations/*` | PASS | Artwork, novel, user and separated all-category envelopes. |
| Following timelines | `timeline/following-*` | PASS | Artwork and novel envelopes, including legitimate empty lists. |
| Latest artwork | `timeline/latest-artwork-illust-page-*` | PASS | Explicit `illust` subtype; distinct page 1/page 2 sequences. |
| Latest defaults/novel | `timeline/latest-artwork-page-*`, `latest-novel` | FAIL | Default content type causes invalid argument; novel response is malformed upstream. |
| MyPixiv | `mypixiv/users`, `works-illust`, `works-novel` | PASS | Empty success envelopes accepted; `works-artwork` separately retains help/runtime mismatch. |
| Current user | `current-user/*` | PARTIAL | Profile and six read lists pass; blocked users returns upstream 404. |
| Discovered user | `user/*`, `detail/user` | PARTIAL | Profile and six read lists pass; blocked users returns upstream 404. |
| Artwork detail | `detail/artwork` | PASS | ID and page/resource metadata validated. |
| Novel detail | `detail/novel*` | FAIL | Two current samples return upstream 404. |
| Comments | `comments/*` | PARTIAL | Novel comments returns a valid empty envelope; artwork comments return upstream 404. |
| Bookmarks | `bookmarks/*` | PARTIAL | Artwork/novel lists and tags pass; detail returns upstream 404. |
| Series | `series/artwork-probe` | INCOMPLETE | Artwork probe returns upstream 404; the novel branch has not yet been invoked and is assigned to Task 16A. |
| Downloads | `downloads/*` | PASS | regular `.jpg`/JPEG, thumb `.jpg`/JPEG, Ugoira `.apng`/PNG. |
| Rejection paths | `errors/*` | PASS | Four representative invalid/auth paths reject with expected exit 1 and explicit stderr. |

## Explicit exclusions

- FANBOX is outside the requested Pixiv-only scope.
- Bookmark/follow mutations, auth refresh/remove/use, config writes, updates, and interactive login were not executed.
- MCP is a long-lived stdio server rather than a CLI data command and was not started.
