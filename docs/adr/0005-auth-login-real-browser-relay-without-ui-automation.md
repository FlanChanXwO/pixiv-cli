# ADR 0005: Auth Login Captures Browser Callback Without UI Automation

## Status

Accepted.

## Context

Pixiv OAuth login can pass through `accounts.pixiv.net/post-redirect` before reaching the final `pixiv://account/login` or App API callback request. In real browsers this page may appear blank or require Pixiv-side account confirmation. The relay URL contains a `return_to` App API start URL, but opening that `return_to` directly is not equivalent to Pixiv's relay page and can produce Pixiv's "endpoint does not exist" page.

FANBOX uses Pixiv account login as a first-party web product, but that does not establish a compatible Pixiv App API authorization code. The CLI needs a refresh token from `oauth.secure.pixiv.net/auth/token` using the App API callback URI, so FANBOX-style web session login is not a substitute.

## Decision

- Keep `pixiv auth login` based on PKCE and the local loopback/manual fallback page.
- On macOS, default browser opening first registers a repo-owned local `pixiv://` URL handler app and opens the normal system browser. This keeps the user's existing Pixiv browser session available while forwarding only the final callback URL to the current CLI loopback server.
- If the URL handler path is unavailable, fall back to a managed Chromium/Edge browser profile with a DevTools connection.
- The DevTools connection only enables network event observation and extracts Pixiv OAuth callback URLs; it must not click pages, fill forms, read cookies, read tokens, or inspect page storage.
- Keep macOS read-only URL/session/history observation as an additional fallback detector.
- Treat `accounts.pixiv.net/post-redirect` as a Pixiv authorization relay page, not as a callback.
- When the relay page belongs to the current `code_challenge`, report that the CLI is waiting for Pixiv's `pixiv://` handoff. The CLI validates `return_to` but does not bypass or automatically reopen the relay page.
- Let users paste a relay URL into the terminal or local fallback page to explicitly open that relay URL once.
- Do not implement HTTPS MITM, automatic certificate installation, host rewriting, or FANBOX session import for `auth login`.

## Consequences

- The CLI can reuse the user's existing browser login state on macOS without reading browser cookies or tokens.
- The CLI can capture the same short-lived callback request that users previously had to copy from DevTools manually.
- Users still need to confirm their Pixiv account in the browser.
- The local URL handler is registered as the `pixiv://` handler while a login attempt is active; users may see a browser prompt the first time Edge/Chrome opens it.
- A browser can remain visually parked on the white Pixiv relay page after the handoff; CLI success/failure is determined by whether the terminal receives and exchanges the callback code.
- If Pixiv or reCAPTCHA does not generate a callback, the CLI cannot synthesize success and must keep manual fallback available.
- Browser URL observation remains intentionally narrow and must not grow into cookie/token extraction.
