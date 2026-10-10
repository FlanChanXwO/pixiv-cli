# MCP animation delivery and Gallery loading

Approved scope: asynchronous Gallery animation control and protected binary delivery; existing `pixiv_artwork_media` remains compatible. No dependency or quality changes. The running MCP and tunnel remain untouched until an explicit deployment request.

## Evidence and diagnosis

The previous 50-artwork run returned all 222 static HTTP requests successfully, but used independent curl connections and warmed account/metadata state. These timings cannot represent browser first-screen loading. One GIF succeeded; four GIF samples and the APNG sample failed. Only the additional APNG probe preserved the exact `Transport closed` diagnostic. Neither timeout nor response size is established as its cause.

The old animation owner downloads, encodes and embeds the complete result on every call. Repeated generation is established by the implementation, independently of the unknown transport failure. Instrument artwork lookup, ZIP download, native gate waiting, encoding, serialization and delivery with durations, byte counts and cancellation classification; exclude credentials, grants and signed URLs. Compare the same failing IDs over authenticated loopback, public raw MCP and the plugin before claiming a transport root cause. Do not add retries or enlarge timeouts.

## Ownership and contract

`pixiv/mediaproxy` continues to own capability authentication and canonical media routing. An animation owner under that boundary manages jobs, private temporary storage and subscriptions. The composition root configures it once and closes it before account sessions. SDK reads use current account-owner read leases; tool code never accesses a credential database or protocol adapter.

`pixiv_gallery_animation` accepts `prepare` with positive `illust_id` and optional `format` (`gif` default, or `apng`), or `status`/`release` with an opaque subscription handle. Validate operation-specific fields before upstream access. Each prepare returns an independent subscription. Job states are `queued`, `downloading`, `encoding`, `ready`, `failed`, `cancelled`, `expired`; gate waiting is observable without fabricated progress. Ready output contains protected animation/PNG preview URLs, MIME, size, filename and expiry; never full media or server paths. Invalid handles and failed jobs are explicit error outcomes.

Keys include grant, account, opaque credential/network generation, artwork and format. Duplicate prepares join one job. Workers use the server lifecycle, not the short tool-request lifetime. Releasing the last subscription cancels unfinished work; ready outputs may remain until expiry. Old or foreign handles cannot acquire access. Failure is terminal, retries require another explicit prepare.

URLs bind job identity, output kind, grant/account generation and expiration with a process signing key. Every GET/HEAD rechecks grant and current account generation. Expiry is the earlier of gallery cache TTL and issuing authorization expiry. Responses use `private, no-store`, canonical origin and existing host checks/CSP. Static URI stays unchanged. Transfers pin their files until handles close; invalidation prevents new reads and cleans after pins are released.

## Temporary storage and native writes

`[mcp].gallery_animation_temp_capacity_mib`, CLI alias `mcp_gallery_animation_temp_capacity_mib`, defaults to 1024 MiB and must be a positive representable integer. Startup loading only. Disk budget is independent of the memory cache. ZIP, temporary native output, preview and retained result all count. Each service has an isolated 0700 directory and 0600 files.

Download and preview use budget-aware writers. Native encoding gains an internal bounded-writing interface while the legacy encode entry point remains unchanged. Reserve native capacity before encoding, fail any write exceeding it, and release unused reservation after publication. Reservations and active files cannot be evicted. Reclaim expired or unsubscribed ready artifacts before denying capacity. No truncated result, quality reduction or optimistic readiness. Cleanup runs on failed/cancelled jobs, expiry, revocation/version invalidation and shutdown. Startup may delete a prior instance directory only when instance ownership and process termination can be proved; leave uncertain directories intact.

Preserve the Rust global serial gate. Enforce output quota in Rust's writer before disk writes, including both GIF and APNG containers. Keep panic containment, cancellation token ownership and atomic publication. A host-only rebuild is not all-platform native evidence.

## Gallery and measurement

Gallery prepares a job, polls status every two seconds, and releases subscriptions on view changes and format switches. On ready it displays the PNG preview and switches to the animation URL only for Play. Static preview remains independent and stable. Downloads use the selected format through the existing host-capability boundary. Reject foreign origins. Stop polling on failure or cancellation; show explicit retry. Preserve bookmarks, media validation, resource cleanup and host degradation.

Measure static images with a single browser: visible first screen and full page separately, connection reuse and request queueing recorded. Keep lazy loading, thumbnail quality and configured concurrency. Prioritize visible requests only if traces establish harmful queueing.

## Review and acceptance

Design review checks: request cancellation must not kill a subscribed worker; account/grant invalidation must stop new access; budgets include in-progress native output; pins defer deletion without extending authorization; generation keys prevent reuse after credential changes; all failure paths release files/reservations/subscriptions. The current subagent API cannot express the user-required `fork_turns = "none"`; independent delegated review is unavailable and the main thread performs the design review.

TDD covers merging, subscriber release, cancellation, capacity/TTL, authorization/version isolation, cleanup, GET/HEAD and interrupted transfers; native fixtures validate bounded writes and unchanged GIF/APNG output. Browser verification exercises actual frame changes, preview stability, format change and failures. Actual ChatGPT mobile remains separate acceptance.

Generate/check frontend resources; run focused regression, native gates, sequential full test/race with `-p 4`, vet, formatting, build/package and code review. Update both locale configuration/MCP/architecture and product skill. Report unverified platforms and live paths accurately. After explicitly authorized restart, repeat the same 50 artworks, including failures, with success rate, median/P95/range and upstream/encode counts.
