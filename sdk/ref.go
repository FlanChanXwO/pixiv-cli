package sdk

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// resourceRefFormatVersion is the encoding version of the resource reference
// envelope. Decoding rejects unknown format versions so future additive
// changes are explicit.
const resourceRefFormatVersion = 1

// resourceRefEnvelope is the versioned JSON form of a resource reference. The
// payload is product-owned, opaque identity state that the product SDK
// revalidates when a resource is opened. The envelope must never contain the
// resolved resource URL, signed URLs, tokens, or cookies.
type resourceRefEnvelope struct {
	Version int    `json:"v"`
	Product string `json:"p"`
	Payload []byte `json:"d"`
}

// ResourceRef is an opaque, product-scoped identity for a first-party media
// resource. It does not encode or promise to persist Resource.URL: callers use
// it as a stable cache key and hand it back to OpenResource or SaveResource so
// the product SDK can revalidate and safely read the resource.
//
// The Text codec uses the route-safe unpadded base64url alphabet and is suitable
// for URL path segments. The JSON codec is the same text form as a JSON string.
// A ResourceRef cannot be constructed from an arbitrary raw URL.
type ResourceRef struct {
	text string
}

// NewResourceRef constructs an opaque reference for product carrying the given
// opaque identity payload. It is intended for product SDK implementations.
// product must be non-empty and payload must be non-empty; violations return
// InvalidArgument.
func NewResourceRef(product string, payload []byte) (ResourceRef, error) {
	if product == "" {
		return ResourceRef{}, NewError("", "NewResourceRef", InvalidArgument, WithDetail("product is required"))
	}
	if len(payload) == 0 {
		return ResourceRef{}, NewError("", "NewResourceRef", InvalidArgument, WithDetail("reference payload is required"))
	}
	env := resourceRefEnvelope{Version: resourceRefFormatVersion, Product: product, Payload: payload}
	raw, err := json.Marshal(env)
	if err != nil {
		return ResourceRef{}, NewError("", "NewResourceRef", InvalidArgument, WithCause(err))
	}
	return ResourceRef{text: base64.RawURLEncoding.EncodeToString(raw)}, nil
}

// IsZero reports whether the reference is the zero value.
func (r ResourceRef) IsZero() bool { return r.text == "" }

// String returns the route-safe text encoding of the reference.
func (r ResourceRef) String() string { return r.text }

// MarshalText returns the route-safe unpadded base64url text encoding of the
// reference.
func (r ResourceRef) MarshalText() ([]byte, error) {
	if r.IsZero() {
		return nil, NewError("", "ResourceRef.MarshalText", InvalidArgument, WithDetail("zero reference"))
	}
	return []byte(r.text), nil
}

// UnmarshalText decodes a reference from its route-safe unpadded base64url text
// encoding, returning InvalidArgument for malformed input.
func (r *ResourceRef) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		return NewError("", "ResourceRef.UnmarshalText", InvalidArgument, WithDetail("empty reference text"))
	}
	if _, err := decodeResourceRef(string(text)); err != nil {
		return err
	}
	r.text = string(text)
	return nil
}

// MarshalJSON encodes the reference as a JSON string containing its Text form.
// Marshaling a zero reference returns InvalidArgument.
func (r ResourceRef) MarshalJSON() ([]byte, error) {
	text, err := r.MarshalText()
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(text))
}

// UnmarshalJSON decodes a reference from a JSON string containing its Text form.
func (r *ResourceRef) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return NewError("", "ResourceRef.UnmarshalJSON", InvalidArgument, WithCause(err))
	}
	return r.UnmarshalText([]byte(text))
}

// ParseResourceRef decodes a reference from its route-safe unpadded base64url
// text encoding, returning InvalidArgument for malformed input.
func ParseResourceRef(text string) (ResourceRef, error) {
	var r ResourceRef
	if err := r.UnmarshalText([]byte(text)); err != nil {
		return ResourceRef{}, err
	}
	return r, nil
}

// ResourceRefProduct returns the product namespace of a parsed reference.
func ResourceRefProduct(r ResourceRef) (string, error) {
	env, err := decodeResourceRef(r.text)
	if err != nil {
		return "", err
	}
	return env.Product, nil
}

// ResourceRefPayload returns the opaque identity payload of a parsed reference
// so the product SDK can revalidate the resource it names.
func ResourceRefPayload(r ResourceRef) ([]byte, error) {
	env, err := decodeResourceRef(r.text)
	if err != nil {
		return nil, err
	}
	return env.Payload, nil
}

func decodeResourceRef(text string) (resourceRefEnvelope, error) {
	raw, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		return resourceRefEnvelope{}, NewError("", "decodeResourceRef", InvalidArgument, WithCause(err))
	}
	var env resourceRefEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return resourceRefEnvelope{}, NewError("", "decodeResourceRef", InvalidArgument, WithCause(err))
	}
	if env.Version != resourceRefFormatVersion {
		return resourceRefEnvelope{}, NewError("", "decodeResourceRef", InvalidArgument, WithCause(fmt.Errorf("unsupported reference format version %d", env.Version)))
	}
	if env.Product == "" || len(env.Payload) == 0 {
		return resourceRefEnvelope{}, NewError("", "decodeResourceRef", InvalidArgument, WithDetail("reference missing product or payload"))
	}
	return env, nil
}
