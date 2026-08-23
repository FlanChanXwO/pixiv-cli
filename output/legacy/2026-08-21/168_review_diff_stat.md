# Review preflight commit diff stat

## Command
```shell
cd /private/tmp/pixiv-cli-pixiv-e2e-fixes && git diff --stat origin/main...HEAD
```

## Output
```text
 changelog/unreleased/en.md                         |  1 +
 changelog/unreleased/zh-CN.md                      |  1 +
 docs/en/cli-reference.md                           |  6 +--
 docs/en/maintainers/architecture.md                |  2 +-
 docs/en/sdk.md                                     |  1 +
 docs/zh-CN/cli-reference.md                        |  4 +-
 docs/zh-CN/maintainers/architecture.md             |  2 +-
 docs/zh-CN/sdk.md                                  |  1 +
 e2e/sdk_pixiv_e2e_test.go                          |  2 +-
 .../mcpserver/pixiv/tools/download/download.go     |  2 +-
 internal/media/downloader/downloader.go            | 51 ++++++++++++++++++--
 internal/media/downloader/downloader_test.go       | 34 ++++++++++++-
 internal/media/downloader/mime.go                  | 21 +++++++++
 internal/media/downloader/mime_test.go             |  8 ++++
 .../pixiv/endpoint/artwork/timeline/timeline.go    | 55 +++++++++++++++++-----
 .../endpoint/artwork/timeline/timeline_test.go     | 19 ++++++++
 .../services/pixiv/endpoint/user/detail/detail.go  |  2 +-
 .../pixiv/endpoint/user/detail/detail_test.go      |  4 +-
 internal/services/pixiv/protocol/protocol.go       |  1 -
 sdk/pixiv/ops_artwork.go                           | 22 +++++++--
 sdk/pixiv/pixiv_test.go                            |  7 ++-
 sdk/pixiv/resource.go                              |  2 +-
 sdk/save.go                                        |  8 ++--
 23 files changed, 215 insertions(+), 41 deletions(-)
```

Exit code: 0

Verdict: PASS
