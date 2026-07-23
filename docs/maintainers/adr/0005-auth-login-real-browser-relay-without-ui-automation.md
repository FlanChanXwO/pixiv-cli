# ADR 0005: Auth Login Uses Explicit Callback Handoff Only

## Status

Accepted，2026-07 v0.3 安全边界修订。

## Context

Pixiv OAuth login can pass through `accounts.pixiv.net/post-redirect` before reaching the final `pixiv://account/login` or App API callback request. In real browsers this page may appear blank or require Pixiv-side account confirmation. The relay URL contains a `return_to` App API start URL, but opening that `return_to` directly is not equivalent to Pixiv's relay page and can produce Pixiv's "endpoint does not exist" page.

FANBOX uses Pixiv account login as a first-party web product, but that does not establish a compatible Pixiv App API authorization code. The CLI needs a refresh token from `oauth.secure.pixiv.net/auth/token` using the App API callback URI, so FANBOX-style web session login is not a substitute.

## Decision

- Keep `pixiv auth login` based on PKCE and the local loopback/manual fallback page.
- On macOS, desktop Linux, and Windows, default browser opening may temporarily register a repo-owned local `pixiv://` URL handler for the current login attempt and opens the normal system browser. macOS uses its app helper, Linux uses a temporary XDG desktop entry, and Windows uses a temporary HKCU protocol association. Every platform restores the previous association on completion, failure, or cancellation.
- The helper opens a current-loopback browser bridge with the final callback in a URL fragment; the bridge removes that fragment before submitting the callback to the CLI and waiting for the final page.
- If the URL handler path is unavailable, keep normal browser opening plus loopback/manual fallback; do not start a managed Chromium/Edge profile or DevTools/CDP connection.
- Do not read browser cookies, tokens, storage, history, session files, active tabs or network events. Do not automate browser UI, install an extension, or retrieve browser credentials by any other means.
- Treat `accounts.pixiv.net/post-redirect` as a Pixiv authorization relay page, not as a callback.
- When the relay page belongs to the current `code_challenge`, the user may explicitly submit it. The CLI validates `return_to`; terminal submission opens the relay once on the CLI host, while fallback-page submission returns a redirect for that same browser to continue.
- For a headless SSH server, keep the listener on server loopback and let an explicit local `ssh -L` tunnel carry browser-page submissions back to it. The browser machine does not need a pixiv installation.
- Do not implement HTTPS MITM, automatic certificate installation, host rewriting, or FANBOX session import for `auth login`.

## Consequences

- The CLI can reuse the user's existing browser login state on supported desktop systems without reading browser cookies or tokens.
- Callback receipt has an explicit trust boundary: the current loopback listener, the current helper handoff, or a value the user explicitly pastes.
- Users still need to confirm their Pixiv account in the browser.
- The local URL handler is registered as the `pixiv://` handler while a login attempt is active; users may see a browser prompt the first time Edge/Chrome opens it.
- The original Pixiv relay tab can remain visually parked on a white page after the handoff, but the helper opens a local final success/failure page once the CLI exchanges the callback code. The code is never sent in the bridge's loopback GET request or retained in its browser history.
- If Pixiv or reCAPTCHA does not generate a callback, the CLI cannot synthesize success and must keep manual fallback available.
- No browser observation mechanism is retained; adding one requires a new ADR and security review.
