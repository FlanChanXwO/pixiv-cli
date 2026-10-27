package installer

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type checksumsManifest struct {
	KeyID           string `json:"key_id"`
	ChecksumsSHA256 string `json:"checksums_sha256"`
	Signature       string `json:"signature"`
}

func verifyChecksumsManifest(body, checksums []byte, trustedKeys map[string]ed25519.PublicKey) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var manifest checksumsManifest
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("decode checksums manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode checksums manifest: contains more than one JSON value")
		}
		return fmt.Errorf("decode checksums manifest trailing data: %w", err)
	}
	publicKey, ok := trustedKeys[manifest.KeyID]
	if !ok {
		return fmt.Errorf("checksums manifest references unknown key ID %q", manifest.KeyID)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("trusted public key %q has invalid length", manifest.KeyID)
	}
	hash := sha256.Sum256(checksums)
	if manifest.ChecksumsSHA256 != hex.EncodeToString(hash[:]) {
		return fmt.Errorf("checksums manifest SHA-256 does not match checksums.txt")
	}
	signature, err := base64.StdEncoding.DecodeString(manifest.Signature)
	if err != nil {
		return fmt.Errorf("decode checksums manifest signature: %w", err)
	}
	if !ed25519.Verify(publicKey, checksums, signature) {
		return fmt.Errorf("checksums manifest Ed25519 signature verification failed for key ID %q", manifest.KeyID)
	}
	return nil
}

func verifyArchiveChecksum(checksums []byte, filename string, archive []byte) error {
	var expected string
	for _, line := range strings.Split(string(checksums), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return fmt.Errorf("parse checksums.txt line %q", line)
		}
		if strings.TrimPrefix(fields[1], "*") != filename {
			continue
		}
		if expected != "" {
			return fmt.Errorf("checksums.txt has duplicate entry for %q", filename)
		}
		if len(fields[0]) != sha256.Size*2 || strings.ToLower(fields[0]) != fields[0] {
			return fmt.Errorf("checksums.txt has invalid SHA-256 for %q", filename)
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			return fmt.Errorf("checksums.txt has invalid SHA-256 for %q: %w", filename, err)
		}
		expected = fields[0]
	}
	if expected == "" {
		return fmt.Errorf("checksums.txt has no entry for %q", filename)
	}
	if archive == nil {
		return nil
	}
	actual := sha256.Sum256(archive)
	if expected != hex.EncodeToString(actual[:]) {
		return fmt.Errorf("release archive %q SHA-256 does not match checksums.txt", filename)
	}
	return nil
}
