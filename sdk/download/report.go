package download

// File identifies a published file and its zero-based source page.
type File struct {
	ArtworkID int64
	PageIndex int
	Path      string // Absolute destination path.
}

// Failure retains the original error from a page operation. A published file
// also appears in Report.Files if its cleanup subsequently fails.
type Failure struct {
	ArtworkID int64
	PageIndex int
	Cause     error
}

// Warning describes a recoverable naming issue associated with a source page.
type Warning struct {
	ArtworkID int64
	PageIndex int
	Message   string
	Cause     error // Original template failure, when available.
}

// Report retains successful files, warnings and page failures together.
// Entries in each slice follow the order of the supplied pages.
type Report struct {
	Files    []File
	Warnings []Warning
	Failures []Failure
}
