// Package pixiv provides the public Pixiv SDK backed exclusively by the Pixiv
// App API. It offers authenticated artwork, novel, and user operations, typed
// URL references, ugoira metadata, mutations, and the shared resource contract.
//
// The package consists of [Client] and its [Open], [New], and [BeginLogin]
// entry points; the [Artwork], [Novel], and [User] models with their request
// types and operations; [Reference] for parsed page URLs; and [sdk.Resource]
// with [Client.OpenResource] and [Client.SaveResource] for first-party media.
// Pagination cursors stay internal and surface only as [sdk.Cursor].
// Shared wire shapes live in response.go and artwork_wire.go. Novel HTML parsing
// lives in novel_parse.go, and the cookie-scoped login relay sits outside this
// package.
//
// Authentication is App-only: content operations require a valid access token
// and fail with Unauthorized when one is absent; there is no anonymous or
// Web fallback. Open performs a refresh-token rotation and returns a Client that
// holds only the rotated access token; the Client never refreshes on its own.
// Callers persist the rotated Credentials before issuing content requests.
//
// Media is exposed through sdk.Resource, never through raw upstream URL fields.
package pixiv
