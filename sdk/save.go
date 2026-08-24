package sdk

// SaveProgress reports incremental progress while a resource is being saved.
// Total is the expected size in bytes when known, otherwise zero; Done is the
// number of bytes written so far.
type SaveProgress struct {
	Total int64
	Done  int64
}

// SaveOptions configures a single-resource atomic save. SaveResource writes to
// an atomic destination under Path, so a partially transferred file never
// appears at the final path. Progress, when non-nil, is called as bytes are
// written; it must not block or perform network I/O.
type SaveOptions struct {
	Path     string
	Progress func(SaveProgress)
}

// SavedResource reports the result of SaveResource. ContentType is the
// allowlisted upstream media type when the product resource response supplied it.
type SavedResource struct {
	Path        string
	Size        int64
	ContentType string
}
