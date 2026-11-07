package sdk

import "encoding/base64"

// Cursor holds an opaque continuation received from a product SDK. Its zero
// value starts a query or marks the last page. Persist String and parse it with
// ParseCursor, then pass the result in the original query's Cursor field.
// Product operations validate the query and account binding before network I/O.
// Cursors contain no credentials and provide no cryptographic authenticity.
type Cursor struct {
	text string
}

// ParseCursor checks the text encoding. The receiving product operation checks
// the envelope, version, and query binding. Empty text returns the zero cursor.
func ParseCursor(text string) (Cursor, error) {
	if _, err := base64.RawURLEncoding.Strict().DecodeString(text); err != nil {
		return Cursor{}, newError(InvalidArgument, "ParseCursor", "malformed cursor encoding", err)
	}
	return Cursor{text: text}, nil
}

// String returns the persistent text form; the zero cursor returns "".
func (c Cursor) String() string { return c.text }

// IsZero reports whether there is no continuation.
func (c Cursor) IsZero() bool { return c.text == "" }

// MarshalText encodes the cursor, including the empty last-page cursor.
func (c Cursor) MarshalText() ([]byte, error) { return []byte(c.text), nil }

// UnmarshalText parses the persistent text form, leaving c unchanged on failure.
func (c *Cursor) UnmarshalText(text []byte) error {
	parsed, err := ParseCursor(string(text))
	if err != nil {
		return err
	}
	*c = parsed
	return nil
}
