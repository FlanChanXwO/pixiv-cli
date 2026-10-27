// Package fanbox provides the public Pixiv FANBOX SDK. It offers creator
// profiles, posts, tags, home and supporting feeds, URL resolution, and the
// shared resource contract for first-party media.
//
// The package consists of [Client] with its [Open] entry point, the [Creator],
// [Post], and [CreatorTag] models with their request types and operations,
// [Reference] for parsed page URLs, and [sdk.Resource] with
// [Client.OpenResource] and [Client.SaveResource] for first-party media. Shared
// response decoding lives in response.go, and paging plus cursor binding live in
// pagination.go.
//
// The client is constructed from an explicit FANBOXSESSID session value and
// never reads browsers, local account stores, or Pixiv credentials. Cookies are
// sent only to allowlisted FANBOX API, page, and download hosts on the initial
// request; redirected requests omit cookies. An expired session returns
// CredentialsExpired.
//
// First-party media is exposed through sdk.Resource (URL plus an opaque ref);
// third-party embeds keep only their canonical link and are never fetched.
package fanbox
