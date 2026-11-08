package sdk

import (
	"context"
	"encoding/json"
)

// Page carries one batch and a cursor for the following batch. Next uses the
// client and original query captured by the product operation. A JSON-decoded
// page has no such binding; restore its Cursor through the product operation.
//
// To walk a live query, process Items, stop when Cursor.IsZero(), and otherwise
// call Next(ctx). To resume later, save Cursor.String(), call ParseCursor on
// that string, and set the original list request's Cursor to the parsed value.
// Repeat the original query options and account; changing them is InvalidArgument.
// Cursors resume live upstream data, so they do not provide a snapshot.
type Page[T any] struct {
	Items  []T    `json:"items"`  // Current batch; SDK-created pages use a non-nil slice.
	Cursor Cursor `json:"cursor"` // Continuation after Items; zero marks the last batch.
	load   func(context.Context) (Page[T], error)
}

// NewPage binds a batch to the product's continuation operation. SDK product
// packages use it to capture their client and original query without exposing
// either in JSON. A nil loader leaves the page unbound. Nil items become [].
func NewPage[T any](items []T, cursor Cursor, load func(context.Context) (Page[T], error)) Page[T] {
	if items == nil {
		items = []T{}
	}
	return Page[T]{Items: items, Cursor: cursor, load: load}
}

// Next fetches the following batch with ctx. The last page returns an empty page
// without network I/O. A nonterminal unbound page returns InvalidArgument.
func (p Page[T]) Next(ctx context.Context) (Page[T], error) {
	if p.Cursor.IsZero() {
		return NewPage[T](nil, Cursor{}, nil), nil
	}
	if p.load == nil {
		return Page[T]{}, newError(InvalidArgument, "Page.Next", "page is not bound to a client; resume its cursor through the original query", nil)
	}
	return p.load(ctx)
}

// UnmarshalJSON decodes the data and clears any prior client/query binding.
func (p *Page[T]) UnmarshalJSON(data []byte) error {
	var decoded struct {
		Items  []T    `json:"items"`
		Cursor Cursor `json:"cursor"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return newError(InvalidArgument, "Page.UnmarshalJSON", "malformed page", err)
	}
	*p = NewPage(decoded.Items, decoded.Cursor, nil)
	return nil
}
