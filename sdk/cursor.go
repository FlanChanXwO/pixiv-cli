package sdk

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// cursorFormatVersion is the encoding version of the cursor envelope. It is
// internal to this package and independent of the product binding version;
// decoding rejects envelopes whose format version is unknown so that future
// additive changes are explicit.
const cursorFormatVersion = 1

// cursorEnvelope is the versioned, JSON-marshalable form of a cursor. It must
// never contain tokens, cookies, signed URLs, raw search text, or local paths.
type cursorEnvelope struct {
	Version   int    `json:"v"`
	Product   string `json:"p"`
	Operation string `json:"o"`
	Binding   int    `json:"b"`
	QueryHash string `json:"q"`
	Identity  string `json:"id,omitempty"`
	Ephemeral bool   `json:"e,omitempty"`
	Payload   []byte `json:"pl"`
}

// Cursor is an opaque continuation token carried by sdk.Page. Its value is
// owned by the product SDK and cannot be constructed through a struct literal;
// callers receive cursors from page results, persist them through the Text or
// JSON codecs, and pass them back through the matching request struct.
//
// A cursor is bound to a product, operation, binding version, and query digest.
// Product SDKs revalidate those bindings when a caller continues pagination and
// return InvalidCursor on mismatch; a cursor never silently restarts from
// the first page across product, operation, or query changes.
type Cursor struct {
	text string
}

// CursorOption adjusts the binding metadata of a cursor during construction.
type CursorOption func(*cursorEnvelope)

// WithCursorIdentity binds a verified non-secret identity (for example an
// upstream user ID) to an identity-scoped operation's cursor. The identity must
// not be secret; cursor encodings that carry it remain safe to persist.
func WithCursorIdentity(identity string) CursorOption {
	return func(e *cursorEnvelope) { e.Identity = identity }
}

// WithCursorEphemeral marks a cursor that could not be bound to a verifiable
// identity. Such a cursor is only valid for continued use by the same client
// instance that produced it; product SDKs reject its reuse elsewhere.
func WithCursorEphemeral() CursorOption {
	return func(e *cursorEnvelope) { e.Ephemeral = true }
}

// NewCursor constructs a cursor bound to the given product operation. It is
// intended for product SDK implementations; application code receives cursors
// from page results. product and operation must be non-empty, bindingVersion
// must be positive, queryHash must be the digest of the request's query
// parameters, and payload must be the opaque, secret-free upstream continuation
// state. Violations return an error with InvalidArgument.
func NewCursor(product, operation string, bindingVersion int, queryHash string, payload []byte, opts ...CursorOption) (Cursor, error) {
	if product == "" {
		return Cursor{}, NewError("", "NewCursor", InvalidArgument, WithDetail("product is required"))
	}
	if operation == "" {
		return Cursor{}, NewError("", "NewCursor", InvalidArgument, WithDetail("operation is required"))
	}
	if bindingVersion <= 0 {
		return Cursor{}, NewError("", "NewCursor", InvalidArgument, WithDetail("binding version must be positive"))
	}
	if queryHash == "" {
		return Cursor{}, NewError("", "NewCursor", InvalidArgument, WithDetail("query hash is required"))
	}
	if len(payload) == 0 {
		return Cursor{}, NewError("", "NewCursor", InvalidArgument, WithDetail("cursor payload is required"))
	}
	env := &cursorEnvelope{
		Version:   cursorFormatVersion,
		Product:   product,
		Operation: operation,
		Binding:   bindingVersion,
		QueryHash: queryHash,
		Payload:   payload,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(env)
		}
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return Cursor{}, NewError("", "NewCursor", InvalidArgument, WithCause(err))
	}
	return Cursor{text: base64.RawURLEncoding.EncodeToString(raw)}, nil
}

// IsZero reports whether the cursor is the zero value, which means there is no
// further page.
func (c Cursor) IsZero() bool { return c.text == "" }

// MarshalText returns the route-safe unpadded base64url text encoding of the
// cursor. The encoding is versioned and remains decodable across later versions
// of the same major. Marshaling a zero cursor returns InvalidArgument.
func (c Cursor) MarshalText() ([]byte, error) {
	if c.IsZero() {
		return nil, NewError("", "Cursor.MarshalText", InvalidArgument, WithDetail("zero cursor has no encoding"))
	}
	return []byte(c.text), nil
}

// UnmarshalText decodes a cursor from its route-safe unpadded base64url text
// encoding. Unrecognized or malformed encodings return InvalidCursor.
func (c *Cursor) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		return NewError("", "Cursor.UnmarshalText", InvalidCursor, WithDetail("empty cursor text"))
	}
	if _, err := decodeCursor(string(text)); err != nil {
		return NewError("", "Cursor.UnmarshalText", InvalidCursor, WithCause(err))
	}
	c.text = string(text)
	return nil
}

// MarshalJSON encodes the cursor as a JSON string containing its Text form.
func (c Cursor) MarshalJSON() ([]byte, error) {
	if c.IsZero() {
		return nil, NewError("", "Cursor.MarshalJSON", InvalidArgument, WithDetail("zero cursor has no encoding"))
	}
	return json.Marshal(c.text)
}

// UnmarshalJSON decodes a cursor from a JSON string containing its Text form.
func (c *Cursor) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return NewError("", "Cursor.UnmarshalJSON", InvalidCursor, WithCause(err))
	}
	return c.UnmarshalText([]byte(text))
}

// String returns the route-safe text encoding of the cursor. It is safe to log
// and to persist across processes.
func (c Cursor) String() string { return c.text }

// ParseCursor decodes a cursor from its route-safe unpadded base64url text
// encoding, returning InvalidCursor for malformed input.
func ParseCursor(text string) (Cursor, error) {
	var c Cursor
	if err := c.UnmarshalText([]byte(text)); err != nil {
		return Cursor{}, err
	}
	return c, nil
}

// ValidateCursor revalidates the bindings a caller must repeat when continuing
// pagination: product, operation, binding version, and query digest. A zero
// cursor or any mismatch returns InvalidCursor.
func ValidateCursor(c Cursor, product, operation string, bindingVersion int, queryHash string) error {
	if c.IsZero() {
		return NewError("", "ValidateCursor", InvalidCursor, WithDetail("zero cursor"))
	}
	env, err := decodeCursor(c.text)
	if err != nil {
		return NewError("", "ValidateCursor", InvalidCursor, WithCause(err))
	}
	if env.Product != product || env.Operation != operation ||
		env.Binding != bindingVersion || env.QueryHash != queryHash {
		return NewError("", "ValidateCursor", InvalidCursor, WithDetail("cursor binding mismatch"))
	}
	return nil
}

// CursorPayload returns the opaque upstream continuation state embedded in the
// cursor. It is intended for product SDK implementations that re-attach
// per-request secrets when continuing an operation.
func CursorPayload(c Cursor) ([]byte, error) {
	if c.IsZero() {
		return nil, NewError("", "CursorPayload", InvalidCursor, WithDetail("zero cursor"))
	}
	env, err := decodeCursor(c.text)
	if err != nil {
		return nil, NewError("", "CursorPayload", InvalidCursor, WithCause(err))
	}
	return env.Payload, nil
}

// CursorIdentity returns the verified non-secret identity bound to the cursor,
// if any.
func CursorIdentity(c Cursor) (identity string, ok bool) {
	if c.IsZero() {
		return "", false
	}
	env, err := decodeCursor(c.text)
	if err != nil || env.Identity == "" {
		return "", false
	}
	return env.Identity, true
}

// CursorEphemeral reports whether the cursor is only valid for the client
// instance that produced it because no identity could be verified at creation.
func CursorEphemeral(c Cursor) bool {
	if c.IsZero() {
		return false
	}
	env, err := decodeCursor(c.text)
	if err != nil {
		return false
	}
	return env.Ephemeral
}

func decodeCursor(text string) (cursorEnvelope, error) {
	raw, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		return cursorEnvelope{}, err
	}
	var env cursorEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return cursorEnvelope{}, err
	}
	if env.Version != cursorFormatVersion {
		return cursorEnvelope{}, fmt.Errorf("unsupported cursor format version %d", env.Version)
	}
	return env, nil
}
