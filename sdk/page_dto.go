package sdk

// PageDTO is the output-safe representation of a page. Cursor identity is
// reduced to its stable opaque text; query-bound validation remains a runtime
// concern of Page and Cursor.
type PageDTO[T any] struct {
	Items []T    `json:"items"`
	Next  string `json:"next"`
}
