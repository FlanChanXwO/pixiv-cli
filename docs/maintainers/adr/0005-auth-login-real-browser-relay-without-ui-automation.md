# ADR 0005: Auth Login Uses Explicit Callback Handoff Only

## Status

Accepted，2026-07 v0.3 安全边界修订。

## Context

Pixiv OAuth login can pass through `accounts.pixiv.net/post-redirect` before reaching the final `pixiv://account/login` or App API callback request. In real browsers this page may appear blank or require Pixiv-side account confirmation. The relay URL contains a `return_to` App API start URL, but opening that `return_to` directly is not equivalent to Pixiv's relay page and can produce Pixiv's "endpoint does not exist" page.

FANBOX uses Pixiv account login as a first-party web product, but that does not establish a compatible Pixiv App API authorization code. The CLI needs a refresh token from `oauth.secure.pixiv.net/auth/token` using the App API callback URI, so FANBOX-style web session login is not a substitute.

## Decision

- Keep `pixiv auth login` based on PKCE and the local loopback/manual fallback page.
- On macOS, default browser opening may register a repo-owned local `pixiv://` URL handler app for the current login attempt and opens the normal system browser. It forwards only the final callback URL to the current CLI loopback server.
- If the URL handler path is unavailable, keep normal browser opening plus loopback/manual fallback; do not start a managed Chromium/Edge profile or DevTools/CDP connection.
- Do not read browser cookies, tokens, storage, history, session files, active tabs or network events. Do not automate browser UI, install an extension, or retrieve browser credentials by any other means.
- Treat `accounts.pixiv.net/post-redirect` as a Pixiv authorization relay page, not as a callback.
- When the relay page belongs to the current `code_challenge`, the user may explicitly submit it. The CLI validates `return_to` before opening that relay URL once; it never scans a browser to discover the URL.
- Let users paste a relay URL into the terminal or local fallback page to explicitly open that relay URL once.
- Do not implement HTTPS MITM, automatic certificate installation, host rewriting, or FANBOX session import for `auth login`.

## Consequences

- The CLI can reuse the user's existing browser login state on macOS without reading browser cookies or tokens.
- Callback receipt has an explicit trust boundary: the current loopback listener, the current helper handoff, or a value the user explicitly pastes.
- Users still need to confirm their Pixiv account in the browser.
- The local URL handler is registered as the `pixiv://` handler while a login attempt is active; users may see a browser prompt the first time Edge/Chrome opens it.
- A browser can remain visually parked on the white Pixiv relay page after the handoff; CLI success/failure is determined by whether the terminal receives and exchanges the callback code.
- If Pixiv or reCAPTCHA does not generate a callback, the CLI cannot synthesize success and must keep manual fallback available.
- No browser observation mechanism is retained; adding one requires a new ADR and security review.
