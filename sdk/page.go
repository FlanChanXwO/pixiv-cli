package sdk

// Page holds one page of results from a list operation and the cursor needed
// to fetch the next page.
//
// A successful page with no items must have a non-nil empty Items slice rather
// than a nil slice. Next.IsZero() reports that there is no further page; a
// non-zero Next must be passed back through the matching request struct to
// continue the iteration. Callers repeat the original query parameters when
// continuing; product SDKs reject a cursor whose product, operation, binding
// version, or query digest does not match with CodeInvalidCursor.
type Page[T any] struct {
	Items []T
	Next  Cursor
}
