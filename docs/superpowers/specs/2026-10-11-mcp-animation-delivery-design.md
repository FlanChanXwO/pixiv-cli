# MCP animation delivery and Gallery loading

## Scope

Gallery uses short asynchronous control requests and protected full-quality GIF/APNG files. Generic MCP clients retain complete inline `pixiv_artwork_media` output; negotiated App delivery uses the existing tool schema. No public delivery parameter, quality downgrade, automatic retry or enlarged timeout was added. Phone-device testing, ChatGPT reply/429/cache investigation and strict live host-CSP testing were explicitly excluded; existing security regressions remain required.

## Ownership and contract

`pixiv/mediaproxy` continues to own capability authentication and canonical media routing. An animation owner under that boundary manages jobs, private temporary storage and subscriptions. The composition root configures it once and closes it before account sessions. SDK reads use current account-owner read leases; tool code never accesses a credential database or protocol adapter.

`pixiv_gallery_animation` accepts `prepare` with positive `illust_id` and optional `format` (`gif` default, or `apng`), or `status`/`release` with an opaque subscription handle. Validate operation-specific fields before upstream access. Each prepare returns an independent subscription. Job states are `queued`, `downloading`, `encoding`, `ready`, `failed`, `cancelled`, `expired`; gate waiting is observable without fabricated progress. Ready output contains protected animation/PNG preview URLs, MIME, size, filename and expiry; never full media or server paths. Invalid handles and failed jobs are explicit error outcomes.

Keys include grant, account, opaque credential/network generation, artwork and format. Duplicate prepares join one job. Workers use the server lifecycle, not the short tool-request lifetime. Releasing the last subscription cancels unfinished work; ready outputs may remain until expiry. Old or foreign handles cannot acquire access. Failure is terminal, retries require another explicit prepare.

URLs bind job identity, output kind, grant/account generation and expiration with a process signing key. Every GET/HEAD rechecks grant and current account generation. Expiry is the earlier of gallery cache TTL and issuing authorization expiry. Responses use `private, no-store`, canonical origin and existing host checks/CSP. Static URI stays unchanged. Transfers pin their files until handles close; invalidation prevents new reads and cleans after pins are released.

## Temporary storage and native writes

`[mcp].gallery_animation_temp_capacity_mib`, CLI alias `mcp_gallery_animation_temp_capacity_mib`, defaults to 1024 MiB and must be a positive representable integer. Startup loading only. Disk budget is independent of the memory cache. ZIP, temporary native output, preview and retained result all count. Each service has an isolated 0700 directory and 0600 files.

Download and preview use budget-aware writers. Native encoding gains an internal bounded-writing interface while the legacy encode entry point remains unchanged. Reserve the exact pending bytes before each native write, fail writes exceeding the remaining shared capacity, and reconcile reservations with actual file size after publication. Reservations and active files cannot be evicted. Reclaim expired or unsubscribed ready artifacts before denying capacity. No truncated result, quality reduction or optimistic readiness. Cleanup runs on failed/cancelled jobs, expiry, revocation/version invalidation and shutdown. Startup may delete a prior instance directory only when instance ownership and process termination can be proved; leave uncertain directories intact.

Preserve the Rust global serial gate. Enforce output quota in Rust's writer before disk writes, including both GIF and APNG containers. Keep panic containment, cancellation token ownership and atomic publication. A host-only rebuild is not all-platform native evidence.

## Gallery and measurement

Gallery prepares a job, polls status every two seconds, and releases subscriptions on view changes and format switches. On ready it displays the PNG preview and switches to the animation URL only for Play. Static preview remains independent and stable. Downloads use the selected format through the existing host-capability boundary. Reject foreign origins. Stop polling on failure or cancellation; show explicit retry. Preserve bookmarks, media validation, resource cleanup and host degradation.

Measure static images with a single browser: visible first screen and full page separately, connection reuse and request queueing recorded. Keep lazy loading, thumbnail quality and configured concurrency. Prioritize visible requests only if traces establish harmful queueing.

## Static images and Gallery interaction

The server shares a 256 MiB in-memory LRU for successful images and artwork metadata, with a 15-minute TTL and separate metadata/image concurrency limits of four. Authorization, credential/network generation and expiry are checked before reuse. Requests with the same metadata key are coalesced. Image responses stream after upstream/header validation; incomplete responses are neither cached nor appended with error text.

The thumbnail-only list opens a detail view with artwork information and an author link. Paging retains the displayed image until its replacement loads, ignores obsolete responses and reuses successful images on return. Originals open from the image and remain downloadable. An independent, approved 256 MiB browser-session LRU budgets encoded and decoded image estimates; it is not an RSS/GPU guarantee. New queries and teardown release retained resources. Bookmark writes still require known current state.

Expired historical capabilities can be renewed through authenticated control requests. Animation subscriptions release on view/format changes, while ready results may be reused until expiry. Browser validation covered responsive 375/768/1024 layouts, light/dark themes, keyboard controls, original viewing/saving, GIF/APNG frame changes, stable PNG preview, explicit failure/retry and four bridge modes. Host-specific limitations are not silently converted to success.

## Source installation

`frontend` embeds ten exact HTML/CSS/JS inputs and assembles seven resources once, without disk or network reads at runtime. The bytes match the former generated resources. Rendering, escaping, CSP/media origin and authentication remain with their Go owners. The legacy `frontendassets` commands are read-only validators; compilation and versioned `go install` no longer require generated files.

A clean source installation and a private module-proxy `go install ...@version` passed. A deliberately broken following module version failed and preserved the installed executable. Existing updater regressions retain precise Release-tag selection. Neither the user's installed binary nor release tags were modified by these install probes.

## Proven transfer and lifecycle fixes

- Controlled ZIP truncation preserves `io.ErrUnexpectedEOF`; connection resets emit a fixed `animation_zip_reset` classification without logging URLs or credentials. Failed work publishes no partial result and releases bodies, files and reserved capacity.
- Cancelling `GetPixiv` during cached-session validation previously invalidated a healthy shared session and interrupted unrelated transfers. Red/Green proved that request cancellation now leaves that generation intact; real credential replacement/deletion, expiry, network changes and upstream authentication failure still invalidate it.
- Native stage observations can legitimately take zero nanoseconds on Windows. Stage presence now uses callback flags rather than nonzero duration. A forced-zero observer reproduced the previous failures.
- Windows directory watches observe child changes but can miss moving the watched directory itself. Both account and animation owners also watch the parent on Windows. The previously failing directory-loss regressions pass without polling or timeout changes. POSIX permission assertions retain their platform scope; Windows test databases close before directory rename.

## Recorded validation before main-branch integration

Snapshot `0bea5f0a73c1ad721c7c6c4e9e86f5d7f03112d4` passed sequential full ordinary/race tests (157 packages each, `-p 4`), vet, formatting, embedded-resource validation, Gallery fixtures, canonical build, packaging and clean/versioned installation. The 160-member archive matched its binary and license inputs byte for byte.

- [Six native platforms, including Windows lifecycle regressions](https://github.com/FlanChanXwO/pixiv-cli/actions/runs/38616536254)
- [Both Linux container targets](https://github.com/FlanChanXwO/pixiv-cli/actions/runs/38616574557)

These are validation runs without publishing or signing. Earlier snapshots failed Windows stage-presence and directory-loss tests; those failures led to the fixes above. They are not treated as passing evidence. Integration with the newer main branch must obtain its own exact-head gates; the PR check runs identify that final source.

## Live measurements and retained failures

After authorized deployment, the same 50 artworks produced 250/250 successful static responses across five rounds with four persistent HTTPS readers. Server counts were 50 artwork queries, 50 upstream image reads, 450 cache hits including metadata, and one account-session refresh. These are protocol timings, not browser first-screen timings.

| Static measurement | Samples | Median (s) | P95 (s) | Range (s) |
| --- | ---: | ---: | ---: | --- |
| First pass | 50 | 1.044 | 2.933 | 0.950–3.118 |
| Repeated reads | 200 | 0.662 | 0.963 | 0.621–1.400 |
| All reads | 250 | 0.671 | 1.311 | 0.621–3.118 |

The five Ugoira artworks in that batch each passed GIF and APNG preparation: 10/10 cases, 80/80 planned loopback/tunnel GET/HEAD responses, 20 stable PNG reads and 30 successful subscription releases. Complete files decoded every frame; all four GETs per case matched. All nine cases with a complete earlier baseline preserved their hashes. Server counts were exactly ten ZIP downloads/encodes and twenty animation cache hits, with no failed stage or extra refresh.

| Animation measurement | Samples | Median (s) | P95 (s) | Range (s) |
| --- | ---: | ---: | ---: | --- |
| Initial preparation, including polling | 10 | 14.859 | 21.464 | 8.119–21.464 |
| Repeated preparation, control only | 20 | 1.411 | 1.756 | 1.072–2.247 |
| Full binary transfer, loopback | 20 | 0.013 | 0.037 | 0.003–0.047 |
| Full binary transfer, tunnel | 20 | 11.563 | 29.540 | 3.561–31.844 |

Five independent cold preparations of artwork `150701816` each downloaded one 9,472,359-byte ZIP and encoded one byte-identical 20,021,822-byte GIF. All repeated local GET/HEAD checks passed. This does **not** explain the original intermittent ZIP interruption: its precise cause remains unknown.

An earlier animation batch completed only seven of ten cases: `150701553/APNG` reset during warm preparation; `150701519/GIF` expired during preparation; `150701519/APNG` lost authorization after 52,227,285 of 63,121,685 transfer bytes and subsequent requests returned 410. Aggregated logs do not establish that all three used the reproduced cached-session cancellation path. These failures remain part of the evidence despite later success.

A later setup attempt also encountered public token-refresh HTTP 403 and then expired-token HTTP 401 before content access. The existing refresh grant succeeded through loopback with the canonical Host. No owner reset or new grant was used; the public 403 cause is not established. Successful content tests are not proof that every historical authorization/transport failure is fixed.

## Main-branch integration

The newer main branch introduced declarative configuration and split CLI execution/composition. MCP fields now use the same tags and preserve defaults, aliases, integer bounds and strict TOML types. Existing private account-selection fields remain outside ordinary runtime binding. MCP HTTP assembly and read-session ownership follow the new CLI file boundaries; the main-branch artwork fields, search offsets and nil-required schema fix are preserved.

Integration first reproduced a configuration panic because main's schema admitted integers only as private leaves. Public integer defaults and binding are now covered by the retained Gallery configuration tests and a focused declared-value regression. Exact-head ordinary/race, native/container, installation and PR gates must pass before delivery. No Go LSP success is claimed where the workspace lacks that language server.

The first integration native run exposed a fixture race on Linux amd64: the subscription-isolation test supplied a non-animation artwork, so failed-job replacement could remove the first handle before the foreign-account assertion. Waiting for that failed worker reproduced it deterministically. The test now uses the existing valid animation fixture, explicitly checks queued-to-ready behavior and retains the foreign-account error assertion; twenty focused race repetitions passed. No production authorization rule was weakened. Final-source platform evidence must use the corrected test commit.

The same integration run exposed three main-branch environment-precedence fixtures that assumed case-sensitive variable names on Windows. Setting `HTTPS_PROXY` last overwrote `https_proxy` there. Fixtures now write the uppercase value first, preserve the lowercase/empty-value assertions, and retain independent tag-order checks; Windows uses one case-insensitive environment entry. Both CLI references clarify the platform semantics. Production lookup is unchanged.
