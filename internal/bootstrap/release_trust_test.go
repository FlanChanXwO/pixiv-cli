package bootstrap

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	knownReleaseTrustPayload   = "pixiv-cli release trust fixture v1"
	knownReleaseTrustSignature = "USGOAO9imulWXD3Aw8wWD+rHJAYFdMMJ4JA8MlhsEEYTEFXDWpoDkiCbutpzrZ3UFU7gvm6p/7G5i7dVbczTDQ=="
)

// TestProductionReleaseInstallerTrustRootVerifiesKnownSignature 以非敏感、已知的真实签名
// 验证 production release installer options 已装配随源码提交的 key ID、公钥与 SPKI 指纹。
func TestProductionReleaseInstallerTrustRootVerifiesKnownSignature(t *testing.T) {
	options := productionReleaseInstallerOptions(nil)

	require.Len(t, options.TrustedKeys, 1)
	publicKey, ok := options.TrustedKeys[releaseSigningKeyID]
	require.True(t, ok)
	require.Len(t, publicKey, ed25519.PublicKeySize)

	spki, err := x509.MarshalPKIXPublicKey(publicKey)
	require.NoError(t, err)
	fingerprint := sha256.Sum256(spki)
	require.Equal(t, releaseSigningKeyFingerprint, hex.EncodeToString(fingerprint[:]))

	signature, err := base64.StdEncoding.DecodeString(knownReleaseTrustSignature)
	require.NoError(t, err)
	require.True(t, ed25519.Verify(publicKey, []byte(knownReleaseTrustPayload), signature))
}

// TestProductionReleaseInstallerOptionsDefensivelyCopyTrustedKeys 确保生产配置不会把可变 map
// 或公钥切片暴露给后续调用方；轮换配置必须由 bootstrap 每次重建并保持不变。
func TestProductionReleaseInstallerOptionsDefensivelyCopyTrustedKeys(t *testing.T) {
	first := productionReleaseInstallerOptions(nil)
	first.TrustedKeys[releaseSigningKeyID][0] ^= 0xff
	delete(first.TrustedKeys, releaseSigningKeyID)

	second := productionReleaseInstallerOptions(nil)
	publicKey, ok := second.TrustedKeys[releaseSigningKeyID]
	require.True(t, ok)
	require.Len(t, publicKey, ed25519.PublicKeySize)

	signature, err := base64.StdEncoding.DecodeString(knownReleaseTrustSignature)
	require.NoError(t, err)
	require.True(t, ed25519.Verify(publicKey, []byte(knownReleaseTrustPayload), signature))
}
