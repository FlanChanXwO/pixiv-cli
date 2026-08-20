// Package update orchestrates explicit and automatic updates. It keeps only the
// coordinator and automatic-check API; implementation lives in the source,
// release, installer, and process subpackages, re-exported here for the CLI
// composition root.
package update
