# v1.4.0 — 2026-11-21

## Breaking changes

- Rewrite the Go SDK around account-bound references and objects. Replace `Open`/`OpenWith`/`NewWith` with `pixiv.New(CredentialStore, Options)` or `fanbox.New(sessionID, Options)`, load references with `Get(ctx)`, and call network operations with `ctx`. Migrate lists to `sdk.Page[T]`, `Next(ctx)` and opaque cursors; rebind decoded objects before operations. Use `sdk.Error` / `Reason`, product media objects and the current `slog` default logger. Move reverse search to `sdk/saucenao` / `sdk/ascii2d` and selected-page saving to `sdk/download`. The module/import root stays unchanged. ([#273](https://github.com/FlanChanXwO/pixiv-cli/pull/273), [#277](https://github.com/FlanChanXwO/pixiv-cli/pull/277), [#278](https://github.com/FlanChanXwO/pixiv-cli/pull/278), [#279](https://github.com/FlanChanXwO/pixiv-cli/pull/279), [#280](https://github.com/FlanChanXwO/pixiv-cli/pull/280), [#281](https://github.com/FlanChanXwO/pixiv-cli/pull/281), [#282](https://github.com/FlanChanXwO/pixiv-cli/pull/282), [#284](https://github.com/FlanChanXwO/pixiv-cli/pull/284), [#285](https://github.com/FlanChanXwO/pixiv-cli/pull/285), [#286](https://github.com/FlanChanXwO/pixiv-cli/pull/286), [#287](https://github.com/FlanChanXwO/pixiv-cli/pull/287), [#288](https://github.com/FlanChanXwO/pixiv-cli/pull/288), [#289](https://github.com/FlanChanXwO/pixiv-cli/pull/289), [#290](https://github.com/FlanChanXwO/pixiv-cli/pull/290))

- `pixiv config get/set/unset` now accepts declared TOML paths, such as `download.path` instead of `download_path`. Configuration values must have their declared TOML types: remove quotes around numbers and booleans when validation reports a line/key error. Existing configuration file paths stay the same. ([#274](https://github.com/FlanChanXwO/pixiv-cli/pull/274))

- Restart MCP queries after upgrading: earlier `next_cursor` values are rejected. Continuations remain bound to the original account; HTTP calls must retain that account selection. Gallery image identifiers now use readable objects with `artwork_id`, `page_index`, `quality` or `user_id`. ([#284](https://github.com/FlanChanXwO/pixiv-cli/pull/284), [#285](https://github.com/FlanChanXwO/pixiv-cli/pull/285), [#286](https://github.com/FlanChanXwO/pixiv-cli/pull/286))

- Linux binaries now require glibc, including commands that do not use vector inference. Local ONNX Runtime inference requires glibc 2.28 or newer. ([#299](https://github.com/FlanChanXwO/pixiv-cli/pull/299))

## Added

- Add opt-in vector indexing and semantic artwork search. Run `pixiv vector setup`, `pixiv vector sync`, then `pixiv vector search INPUT`; ordinary artwork reads record metadata after setup. Search accepts text, local JPEG/PNG/GIF images or HTTP(S) image URLs, returns 30 artworks by default, and performs bounded catch-up unless `--no-sync` is set. Manga indexes its first page; other works index all pages. ([#296](https://github.com/FlanChanXwO/pixiv-cli/pull/296), [#297](https://github.com/FlanChanXwO/pixiv-cli/pull/297), [#298](https://github.com/FlanChanXwO/pixiv-cli/pull/298), [#303](https://github.com/FlanChanXwO/pixiv-cli/pull/303))

- Vector setup supports local SigLIP2 256/512 models on CPU, explicit `--device gpu` with a CPU/GPU compatibility probe, and Gemini Embedding 2 through a Gemini-compatible endpoint. Setup verifies text/image compatibility before saving. Cloud inference uploads the query and indexed images to the configured service; local indexing can include private bookmarks and R18 works. ([#297](https://github.com/FlanChanXwO/pixiv-cli/pull/297), [#299](https://github.com/FlanChanXwO/pixiv-cli/pull/299), [#300](https://github.com/FlanChanXwO/pixiv-cli/pull/300))

- `vector sync` scans public and private bookmarks by default, resumes in batches, and supports `--restrict`, `--content-type`, `--rating` and full `--reconcile`. Public/private workers share a minimum 0.2-second Pixiv request interval; a larger configured interval takes precedence. Reconciliation removes stale bookmark relationships only after successful completion and preserves indexed artworks/vectors. Add specific works immediately with `vector add ID_OR_URL...` or artwork NDJSON on stdin. ([#303](https://github.com/FlanChanXwO/pixiv-cli/pull/303))

- Add MCP `pixiv_vector_search` using the same local vector index as the CLI. Vector search requires an existing configured model; HTTP query images use authorized media references. ([#301](https://github.com/FlanChanXwO/pixiv-cli/pull/301))

- Add the SDK artwork observer with operation context for integrating artwork recording into application-owned storage. ([#295](https://github.com/FlanChanXwO/pixiv-cli/pull/295))

- CLI help, prompts, human-readable results and diagnostics automatically select English or Simplified Chinese from the host locale. Login/OAuth pages follow `Accept-Language`. Use `env LC_ALL=C pixiv ...` for English or `env LC_ALL=zh_CN.UTF-8 pixiv ...` for Chinese. Machine-readable field names and error codes retain their spelling. ([#302](https://github.com/FlanChanXwO/pixiv-cli/pull/302))

## Changed

- Local accounts move to `auth.json`. The first account-using command imports an existing `pixiv-cli.db` when initializing the account file. A valid current-format `auth.json` is preserved; the old database remains unchanged, including its modification time. ([#276](https://github.com/FlanChanXwO/pixiv-cli/pull/276))

- New `config.toml` files include English comments and defaults. `config set reverse_search.saucenao_api_key VALUE` accepts a positional key, and `config get` prints its effective value, including environment overrides, instead of redacting it. ([#274](https://github.com/FlanChanXwO/pixiv-cli/pull/274))

- Ugoira GIF/APNG encoding uses standard-library codecs; converted images may have subtle visual differences. ([#275](https://github.com/FlanChanXwO/pixiv-cli/pull/275))

## Fixed

- FANBOX creator listings traverse all upstream pages. Downloads preserve images, attachment extensions and complete post bodies, including article blocks and archive passwords, in a private `post.json`. Attachment filenames avoid metadata and filesystem aliases; restricted posts retain metadata and report an error. ([#294](https://github.com/FlanChanXwO/pixiv-cli/pull/294))

- `pixiv fanbox tags` and MCP `fanbox_creator_tags` read the current `featuredTags` response instead of failing with `missing FANBOX tag list`. ([#293](https://github.com/FlanChanXwO/pixiv-cli/pull/293))

- Requests waiting for account initialization honor cancellation. ([#291](https://github.com/FlanChanXwO/pixiv-cli/pull/291))

- Ugoira GIF conversion reports an error before allocating more than 1 GiB of frame memory. Canceled ascii2d searches finish solver teardown within its bounded cleanup period, and oversized FlareSolverr responses are rejected. Pixiv media URLs reject non-default ports. ([#292](https://github.com/FlanChanXwO/pixiv-cli/pull/292))

**Full Changelog:** [v1.3.0...v1.4.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.3.0...v1.4.0)

<!-- internal: [#283](https://github.com/FlanChanXwO/pixiv-cli/pull/283) [#304](https://github.com/FlanChanXwO/pixiv-cli/pull/304) -->
