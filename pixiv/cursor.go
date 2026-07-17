package pixiv

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
)

const cursorVersion = 1

type cursorPayload struct {
	Version     int       `json:"v"`
	Operation   Operation `json:"o"`
	QueryDigest string    `json:"q"`
	Kind        string    `json:"k"`
	Value       string    `json:"n"`
	// Source is additive. Direct NewClient leaves it empty for v1 compatibility;
	// OpenDefault binds continuations to a non-secret route/identity marker.
	Source string `json:"s,omitempty"`
}

func queryDigest(operation Operation, query any) string {
	encoded, _ := json.Marshal(struct {
		Operation Operation `json:"operation"`
		Query     any       `json:"query"`
	}{operation, query})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func encodeCursorForSource(operation Operation, digest, kind string, value int64, source string) Cursor {
	payload, _ := json.Marshal(cursorPayload{Version: cursorVersion, Operation: operation, QueryDigest: digest, Kind: kind, Value: strconv.FormatInt(value, 10), Source: source})
	return Cursor(base64.RawURLEncoding.EncodeToString(payload))
}

func decodeCursorForSource(raw Cursor, operation Operation, digest, kind, source string, requireSource bool) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(string(raw))
	if err != nil {
		return 0, errors.New("cursor is malformed")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var payload cursorPayload
	if err := decoder.Decode(&payload); err != nil {
		return 0, errors.New("cursor is malformed")
	}
	if err := ensureCursorEOF(decoder); err != nil {
		return 0, err
	}
	if payload.Version != cursorVersion || payload.Operation != operation || payload.QueryDigest != digest || payload.Kind != kind {
		return 0, errors.New("cursor does not match this request")
	}
	if requireSource && (payload.Source == "" || payload.Source != source) {
		return 0, errors.New("cursor does not match this operation source")
	}
	value, err := strconv.ParseInt(payload.Value, 10, 64)
	if err != nil || value < 0 || value == 0 && !allowsZeroCursorContinuation(operation, kind) {
		return 0, errors.New("cursor continuation is invalid")
	}
	return value, nil
}

func allowsZeroCursorContinuation(operation Operation, kind string) bool {
	if kind != "offset" {
		return false
	}
	switch operation {
	case OperationIllustRecommended, OperationMangaRecommended, OperationNovelRecommended, OperationUserRecommended:
		return true
	default:
		return false
	}
}

func ensureCursorEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("cursor has trailing data")
	}
	return nil
}
