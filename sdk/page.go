package sdk

import "encoding/json"

// Page holds one page of results from a list operation and the cursor needed
// to fetch the next page.
//
// A successful page with no items must have a non-nil empty Items slice rather
// than a nil slice. Next.IsZero() reports that there is no further page; a
// non-zero Next must be passed back through the matching request struct to
// continue the iteration. Callers repeat the original query parameters when
// continuing; product SDKs reject a cursor whose product, operation, binding
// version, or query digest does not match with InvalidCursor.
type Page[T any] struct {
	Items []T
	Next  Cursor
}

// MarshalJSON emits the cursor as its stable text; the last page has an empty
// next cursor.
func (p Page[T]) MarshalJSON() ([]byte, error) {
	items := p.Items
	if items == nil {
		items = []T{}
	}
	return json.Marshal(struct {
		Items []T    `json:"items"`
		Next  string `json:"next"`
	}{items, p.Next.String()})
}
