// Package pixiv provides the public Pixiv SDK backed exclusively by the Pixiv
// App API. It offers authenticated artwork, novel, and user operations, typed
// URL references, ugoira metadata, mutations, and the shared resource contract.
//
// Authentication is App-only: content operations require a valid access token
// and fail with Unauthorized when one is absent; there is no anonymous or
// Web fallback. Open performs a refresh-token rotation and returns a Client that
// holds only the rotated access token; the Client never refreshes on its own.
// Callers persist the rotated Credentials before issuing content requests.
//
// The client and its models are exposed as English-documented Go API. Protocol
// details remain internal; media is exposed through sdk.Resource, never through
// raw upstream URL fields.
package pixiv
