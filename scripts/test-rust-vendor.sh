#!/bin/sh
# 在全新 Cargo Home 下验证锁定依赖只能由仓库内 vendor 闭包解析，不允许 registry cache 参与。
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
crate_dir="$repo_root/internal/download/ugoira_rs"
manifest="$crate_dir/Cargo.toml"

fail() {
	printf '%s\n' "rust vendor test: $*" >&2
	exit 1
}

[ -f "$manifest" ] || fail "missing Rust manifest: $manifest"
[ -f "$crate_dir/.cargo/config.toml" ] || fail "missing Cargo source replacement: $crate_dir/.cargo/config.toml"

cargo_home=$(mktemp -d "${TMPDIR:-/tmp}/pixiv-cargo-home.XXXXXX")
target_dir=$(mktemp -d "${TMPDIR:-/tmp}/pixiv-cargo-target.XXXXXX")
trap 'rm -rf "$cargo_home" "$target_dir"' EXIT HUP INT TERM

# 必须从 crate 目录执行，Cargo 才会读取该项目的 .cargo/config.toml；空 CARGO_HOME 加上
# --offline 使任何 registry cache 或网络 fallback 都会变成可见失败。
(
	cd "$crate_dir"
	CARGO_HOME="$cargo_home" CARGO_TARGET_DIR="$target_dir" cargo metadata --locked --offline --format-version 1 --manifest-path "$manifest" >/dev/null
	CARGO_HOME="$cargo_home" CARGO_TARGET_DIR="$target_dir" cargo build --locked --offline --manifest-path "$manifest"
	CARGO_HOME="$cargo_home" CARGO_TARGET_DIR="$target_dir" cargo test --locked --offline --manifest-path "$manifest"
)

# licensebundle 会为六个 release target 请求 metadata；继承同一空 Cargo Home，验证全部 target
# 的许可证闭包同样不能退回 registry cache。
(
	cd "$repo_root"
	CARGO_HOME="$cargo_home" CARGO_TARGET_DIR="$target_dir" go run ./scripts/licensebundle --check
)
