#!/bin/sh
# 从公开的 homebrewformula CLI 以 release checksum fixture 渲染 stable formula，
# 验证版本、下载 URL、目标 SHA 与 Homebrew 约束而不需要真实 Release 或 tap。
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
temporary=$(mktemp -d "$repo_root/.pixiv-homebrew-formula-test.XXXXXX")
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

version=0.1.0
checksums="$temporary/checksums.txt"
formula="$temporary/pixiv-cli.rb"

cat > "$checksums" <<EOF
aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  pixiv-cli_${version}_darwin_amd64.tar.gz
bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  pixiv-cli_${version}_darwin_arm64.tar.gz
cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc  pixiv-cli_${version}_linux_amd64.tar.gz
dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd  pixiv-cli_${version}_linux_arm64.tar.gz
eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee  pixiv-cli_${version}_windows_amd64.zip
ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff  pixiv-cli_${version}_windows_arm64.zip
1111111111111111111111111111111111111111111111111111111111111111  install.cmd
2222222222222222222222222222222222222222222222222222222222222222  install.sh
EOF

go run ./scripts/cmd/homebrewformula render --formula pixiv-cli --version "$version" --checksums "$checksums" --output "$formula"
ruby -c "$formula" >/dev/null

grep -F 'class PixivCli < Formula' "$formula" >/dev/null
grep -F 'version "0.1.0"' "$formula" >/dev/null
grep -F 'https://github.com/FlanChanXwO/pixiv-cli/releases/download/v0.1.0/pixiv-cli_0.1.0_darwin_arm64.tar.gz' "$formula" >/dev/null
grep -F 'sha256 "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"' "$formula" >/dev/null
grep -F 'pixiv-cli_0.1.0_darwin_amd64.tar.gz' "$formula" >/dev/null
grep -F 'pixiv-cli_0.1.0_linux_amd64.tar.gz' "$formula" >/dev/null
grep -F 'pixiv-cli_0.1.0_linux_arm64.tar.gz' "$formula" >/dev/null
# 当前公开 tap 只有 stable formula；引用尚未发布的 beta formula 会使 Homebrew
# 每次安装都输出建议移除 conflicts_with 的警告。beta formula 保留反向冲突即可。
if grep -F 'conflicts_with' "$formula" >/dev/null; then
	printf '%s\n' 'stable formula unexpectedly declares a conflict with an unavailable beta formula' >&2
	exit 1
fi
grep -F 'bin.install "pixiv"' "$formula" >/dev/null
# Homebrew 的 post_install 是 Formula 实例方法；`post_install do` 虽是合法 Ruby，
# 却会在当前 Homebrew DSL 中被解释成不存在的类方法，必须在渲染测试中明确防回归。
grep -F 'def post_install' "$formula" >/dev/null
if grep -F 'post_install do' "$formula" >/dev/null; then
	printf '%s\n' 'stable formula uses the unsupported post_install block DSL' >&2
	exit 1
fi
grep -F 'assert_equal "v#{version}", version_info["version"]' "$formula" >/dev/null
if rg -i 'windows|ffmpeg|depends_on' "$formula" >/dev/null; then
	printf '%s\n' 'stable formula unexpectedly selects Windows or has a build/ffmpeg dependency' >&2
	exit 1
fi

# 预发布 release 只能更新独立 beta formula，避免覆盖 stable 通道。
beta_version=0.2.0-beta.1
beta_checksums="$temporary/beta-checksums.txt"
beta_formula="$temporary/pixiv-cli-beta.rb"
sed "s/$version/$beta_version/g" "$checksums" > "$beta_checksums"
go run ./scripts/cmd/homebrewformula render --formula pixiv-cli-beta --version "$beta_version" --checksums "$beta_checksums" --output "$beta_formula"
ruby -c "$beta_formula" >/dev/null
grep -F 'class PixivCliBeta < Formula' "$beta_formula" >/dev/null
grep -F 'version "0.2.0-beta.1"' "$beta_formula" >/dev/null
grep -F 'conflicts_with "pixiv-cli", because: "both install the pixiv command"' "$beta_formula" >/dev/null
grep -F 'bin.install "pixiv"' "$beta_formula" >/dev/null
grep -F 'assert_equal "v#{version}", version_info["version"]' "$beta_formula" >/dev/null
if rg -i 'windows|ffmpeg|depends_on' "$beta_formula" >/dev/null; then
	printf '%s\n' 'beta formula unexpectedly selects Windows or has a build/ffmpeg dependency' >&2
	exit 1
fi

# 一个新的 release version 与其中的 digest 必须逐字进入 renderer 输出，不能沿用模板中的旧值。
next_version=0.1.1
next_checksums="$temporary/next-checksums.txt"
next_formula="$temporary/pixiv-cli-next.rb"
sed "s/$version/$next_version/g; s/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/1111111111111111111111111111111111111111111111111111111111111111/" "$checksums" > "$next_checksums"
go run ./scripts/cmd/homebrewformula render --formula pixiv-cli --version "$next_version" --checksums "$next_checksums" --output "$next_formula"
grep -F 'version "0.1.1"' "$next_formula" >/dev/null
grep -F 'https://github.com/FlanChanXwO/pixiv-cli/releases/download/v0.1.1/pixiv-cli_0.1.1_darwin_amd64.tar.gz' "$next_formula" >/dev/null
grep -F 'sha256 "1111111111111111111111111111111111111111111111111111111111111111"' "$next_formula" >/dev/null

expect_failure() {
	expected=$1
	shift
	if output=$("$@" 2>&1); then
		printf '%s\n' "command unexpectedly succeeded: $*" >&2
		exit 1
	fi
	printf '%s\n' "$output" | grep -F "$expected" >/dev/null || {
		printf '%s\n' "failure did not explain $expected: $output" >&2
		exit 1
	}
}

# 预发布与稳定版绝不可以写进错误的 formula；缺失和重复 checksum 也必须可见地失败。
expect_failure 'stable formula "pixiv-cli" only accepts a stable semantic version' \
	go run ./scripts/cmd/homebrewformula render --formula pixiv-cli --version "$beta_version" --checksums "$beta_checksums" --output "$temporary/wrong-stable.rb"
expect_failure 'beta formula "pixiv-cli-beta" only accepts a prerelease semantic version' \
	go run ./scripts/cmd/homebrewformula render --formula pixiv-cli-beta --version "$version" --checksums "$checksums" --output "$temporary/wrong-beta.rb"
missing_checksums="$temporary/missing-checksums.txt"
sed '/install[.]sh/d' "$checksums" > "$missing_checksums"
expect_failure 'has no entry for required release asset' \
	go run ./scripts/cmd/homebrewformula render --formula pixiv-cli --version "$version" --checksums "$missing_checksums" --output "$temporary/missing.rb"
duplicate_checksums="$temporary/duplicate-checksums.txt"
cat "$checksums" "$checksums" > "$duplicate_checksums"
expect_failure 'has duplicate entry for' \
	go run ./scripts/cmd/homebrewformula render --formula pixiv-cli --version "$version" --checksums "$duplicate_checksums" --output "$temporary/duplicate.rb"
malformed_checksums="$temporary/malformed-checksums.txt"
sed '1s/^a/z/' "$checksums" > "$malformed_checksums"
expect_failure "checksums file line 1 must be '<64 lowercase hex>  <asset>'" \
	go run ./scripts/cmd/homebrewformula render --formula pixiv-cli --version "$version" --checksums "$malformed_checksums" --output "$temporary/malformed.rb"

# 输出位置可能由 release job 提供，不能透过非直接父目录的 symlink 写进别处。
real_output="$temporary/real-output"
mkdir -p "$real_output/nested"
output_link="$temporary/output-link"
ln -s "$real_output" "$output_link"
expect_failure 'output directory contains a symlink ancestor' \
	go run ./scripts/cmd/homebrewformula render --formula pixiv-cli --version "$version" --checksums "$checksums" --output "$output_link/nested/symlinked.rb"
if [ -e "$real_output/nested/symlinked.rb" ]; then
	printf '%s\n' 'renderer wrote through a symlinked output ancestor' >&2
	exit 1
fi
