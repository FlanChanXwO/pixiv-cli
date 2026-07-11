#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# 归档器会拒绝到根目录之间的任何 symlink；测试目录放在真实 repo 路径下，避免 macOS
# `/var -> /private/var` 系统 symlink 干扰被测行为。
temporary=$(mktemp -d "$repo_root/.pixiv-package-release-test.XXXXXX")
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

binary="$temporary/pixiv"
printf '#!/bin/sh\nprintf fixture\n' > "$binary"
chmod 0755 "$binary"

expected="$temporary/expected.txt"
{
	printf '%s\n' pixiv LICENSE THIRD_PARTY_LICENSES.md
	find "$repo_root/third_party/licenses" -type f -print | sed "s#^$repo_root/##"
} | LC_ALL=C sort > "$expected"

assert_members() {
	archive=$1
	format=$2
	actual="$temporary/actual-$format.txt"
	case "$format" in
		tar.gz)
			tar -tzf "$archive" | while IFS= read -r entry; do
				case "$entry" in */) ;; *) printf '%s\n' "$entry" ;; esac
			done | LC_ALL=C sort > "$actual"
			;;
		zip)
			unzip -Z1 "$archive" | while IFS= read -r entry; do
				case "$entry" in */) ;; *) printf '%s\n' "$entry" ;; esac
			done | LC_ALL=C sort > "$actual"
			;;
	esac
	diff -u "$expected" "$actual"
}

for format in tar.gz zip; do
	archive="$temporary/pixiv.$format"
	sh "$repo_root/scripts/package-release.sh" --binary "$binary" --format "$format" --output "$archive"
	assert_members "$archive" "$format"
done

if sh "$repo_root/scripts/package-release.sh" --binary "$binary" --format unsupported --output "$temporary/bad" >/dev/null 2>&1; then
	echo 'unsupported archive format unexpectedly succeeded' >&2
	exit 1
fi

existing_directory="$temporary/existing-output"
mkdir "$existing_directory"
printf 'sentinel\n' > "$existing_directory/keep"
if sh "$repo_root/scripts/package-release.sh" --binary "$binary" --format tar.gz --output "$existing_directory" >/dev/null 2>&1; then
	echo 'existing output directory unexpectedly succeeded' >&2
	exit 1
fi
if [ "$(find "$existing_directory" -type f | wc -l | tr -d ' ')" != 1 ]; then
	echo 'existing output directory received a residual archive' >&2
	exit 1
fi

output_link="$temporary/output-link"
ln -s "$existing_directory" "$output_link"
if sh "$repo_root/scripts/package-release.sh" --binary "$binary" --format tar.gz --output "$output_link/pixiv.tar.gz" >/dev/null 2>&1; then
	echo 'symlinked output directory unexpectedly succeeded' >&2
	exit 1
fi
if [ "$(find "$existing_directory" -type f | wc -l | tr -d ' ')" != 1 ]; then
	echo 'symlinked output directory received a residual archive' >&2
	exit 1
fi

real_child="$existing_directory/real-child"
mkdir "$real_child"
printf 'child sentinel\n' > "$real_child/keep"
ancestor_link="$temporary/ancestor-link"
ln -s "$existing_directory" "$ancestor_link"
if sh "$repo_root/scripts/package-release.sh" --binary "$binary" --format tar.gz --output "$ancestor_link/real-child/pixiv.tar.gz" >/dev/null 2>&1; then
	echo 'output below symlinked ancestor unexpectedly succeeded' >&2
	exit 1
fi
if [ "$(find "$real_child" -type f | wc -l | tr -d ' ')" != 1 ]; then
	echo 'output below symlinked ancestor received a residual archive' >&2
	exit 1
fi
