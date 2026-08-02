// Package fanbox provides the public Pixiv FANBOX SDK. It offers creator
// profiles, posts, tags, home and supporting feeds, URL resolution, and the
// shared resource contract for first-party media.
//
// The client is constructed from an explicit FANBOXSESSID session value and
// never reads browsers, local account stores, or Pixiv credentials. Cookies are
// sent only to the exact allowed FANBOX hosts, never on resource requests or
// redirect targets. A session that expires returns CodeCredentialsExpired.
//
// First-party media is exposed through sdk.Resource (URL plus an opaque ref);
// third-party embeds keep only their canonical link and are never fetched.
package fanbox
