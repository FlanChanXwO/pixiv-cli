// Package sdk provides protocol-agnostic primitives shared by the public Pixiv
// and Pixiv FANBOX SDK packages: paginated pages and their opaque cursors,
// classified errors with safe redaction, and the resource contract used for
// first-party media.
//
// This package does not import product models or account state. Product-specific
// clients live in sdk/pixiv and sdk/fanbox; they share these primitives and
// remain independent of each other.
package sdk
