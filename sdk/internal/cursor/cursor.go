// Package cursor encodes query-bound continuations for the product SDKs.
//
// Bindings include the product, operation, non-secret account ID, and canonical
// query. Continuations use one form: product-validated URL query parameters.
// Callers exclude pagination parameters from Binding.Query and validate decoded
// positions against their protocol before sending a request. Neither binding
// input nor position may contain credentials, signed URLs, or local paths.
package cursor

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

const formatVersion = 1

// Binding identifies one query on one account. Query uses the normalized query
// values in their upstream order; keys are canonicalized by url.Values.Encode.
// Account is empty only for product operations that are independent of identity.
type Binding struct {
	Product   string
	Operation string
	Account   string
	Query     url.Values
}

type envelope struct {
	Version  int        `json:"v"`
	Binding  string     `json:"binding"`
	Position url.Values `json:"position"`
}

func fingerprint(binding Binding) string {
	// A digest keeps search text out of the persisted continuation.
	data := fmt.Sprintf("%q", [4]string{binding.Product, binding.Operation, binding.Account, binding.Query.Encode()})
	sum := sha512.Sum512([]byte(data))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// Encode returns a continuation bound to the normalized original query.
// Position must contain at least one continuation parameter; the product owns
// the meaning and validation of its keys and values.
func Encode(binding Binding, position url.Values) (sdk.Cursor, error) {
	if len(position) == 0 {
		return sdk.Cursor{}, &sdk.Error{Reason: sdk.InvalidArgument, Operation: binding.Operation, Message: "cursor position is required"}
	}
	data, err := json.Marshal(envelope{Version: formatVersion, Binding: fingerprint(binding), Position: position})
	if err != nil {
		return sdk.Cursor{}, &sdk.Error{Reason: sdk.InvalidArgument, Operation: binding.Operation, Message: "cursor encoding failed", Err: err}
	}
	return sdk.ParseCursor(base64.RawURLEncoding.EncodeToString(data))
}

// Decode checks the envelope and original query binding, returning
// InvalidArgument for a malformed, foreign, or mismatched cursor. Positions
// remain untrusted upstream input and require product-specific validation.
func Decode(token sdk.Cursor, binding Binding) (url.Values, error) {
	data, err := base64.RawURLEncoding.DecodeString(token.String())
	if err != nil {
		return nil, &sdk.Error{Reason: sdk.InvalidArgument, Operation: binding.Operation, Message: "malformed cursor encoding", Err: err}
	}
	var value envelope
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, &sdk.Error{Reason: sdk.InvalidArgument, Operation: binding.Operation, Message: "malformed cursor envelope", Err: err}
	}
	if value.Version != formatVersion || len(value.Position) == 0 {
		return nil, &sdk.Error{Reason: sdk.InvalidArgument, Operation: binding.Operation, Message: "malformed cursor"}
	}
	if value.Binding != fingerprint(binding) {
		return nil, &sdk.Error{Reason: sdk.InvalidArgument, Operation: binding.Operation, Message: "cursor does not match the query or account"}
	}
	return value.Position, nil
}
