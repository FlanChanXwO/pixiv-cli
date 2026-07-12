package bootstrap

import (
	"crypto/ed25519"
	"net/http"

	"github.com/FlanChanXwO/pixiv-cli/internal/update"
)

const (
	// releaseSigningKeyID 是 checksums.json 中标识此公钥的可轮换公开标识。
	releaseSigningKeyID = "ed25519-2c27e77742d3c33a"
	// releaseSigningKeyFingerprint 是该 Ed25519 公钥 SPKI DER 的 SHA-256，供源码审计和人工比对。
	releaseSigningKeyFingerprint = "2c27e77742d3c33ad14be867d4e0519229a220898c9a7c868447eaef0951b4cf"
)

// releaseSigningPublicKey 是随受支持 binary 提交的 Ed25519 公钥原始字节。
// 这不是私钥，私钥只能在受保护 release Environment 与受控 macOS Keychain 中保存。
var releaseSigningPublicKey = [...]byte{
	0xee, 0xb2, 0xe2, 0xef, 0xdd, 0x60, 0xb0, 0x61,
	0x6c, 0x0f, 0x61, 0x2b, 0xef, 0x59, 0xe1, 0xe5,
	0x55, 0x40, 0x5f, 0xf8, 0xab, 0xc0, 0xdb, 0xeb,
	0x59, 0x5c, 0x09, 0xdb, 0x9a, 0x78, 0xb1, 0x44,
}

// productionReleaseInstallerOptions 统一 production release installer 的不可变信任根。
// 每次构造均复制 map 与公钥字节，避免调用方或测试的可变数据影响随后启动的 CLI。
func productionReleaseInstallerOptions(httpClient *http.Client) update.ReleaseInstallerOptions {
	return update.ReleaseInstallerOptions{
		HTTPClient: httpClient,
		TrustedKeys: map[string]ed25519.PublicKey{
			releaseSigningKeyID: append(ed25519.PublicKey(nil), releaseSigningPublicKey[:]...),
		},
	}
}
