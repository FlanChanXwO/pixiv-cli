#!/bin/sh
# 校验平台注册表并按 capability 输出 GitHub Actions matrix；matrix=<json> 写入 --github-output。

set -eu

registry=scripts/build/platforms.json
capability=
github_output=

usage() {
	cat >&2 <<'USAGE'
usage: scripts/build/matrix.sh --capability NAME [--registry PATH] [--github-output PATH]
USAGE
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--capability|--registry|--github-output)
			[ "$#" -ge 2 ] || { usage; exit 2; }
			case "$1" in
				--capability) capability=$2 ;;
				--registry) registry=$2 ;;
				--github-output) github_output=$2 ;;
			esac
			shift 2
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			usage
			exit 2
			;;
	esac
done

err=$(mktemp)
trap 'rm -f "$err"' EXIT HUP INT TERM

matrix=$(
	jq -ec --arg capability "$capability" '
		def fail($message): $message | halt_error(1);
		def allowed: ["browser", "container", "homebrew", "race", "release", "smoke", "verification"];
		def known($value): (allowed | index($value)) != null;
		def duplicate($values): ($values | group_by(.) | map(select(length > 1)) | .[0] | .[0]);
		if known($capability) then . else fail("unknown capability \"\($capability)\"") end
		| if (.platforms | type) == "array" then . else fail("platform registry must contain a platforms array") end
		| if (.platforms | length) == 0 then fail("platform registry is empty") else . end
		| [.platforms | to_entries[] | .key as $index | .value as $p
			| ("platform \($index + 1): ") as $prefix
			| (($p | keys) - ["capabilities", "goarch", "goos", "runner"]) as $extra
			| if ($extra | length) > 0 then fail("unknown platform field \"\($extra[0])\"") else . end
			| if (($p.goos | type) == "string" and ($p.goos | length) > 0 and ($p.goos | test("[/\\\\]") | not))
				then . else fail($prefix + "goos must be a simple non-empty name") end
			| if (($p.goarch | type) == "string" and ($p.goarch | length) > 0 and ($p.goarch | test("[/\\\\]") | not))
				then . else fail($prefix + "goarch must be a simple non-empty name") end
			| if (($p.runner | type) == "string" and ($p.runner | length) > 0) then . else fail($prefix + "runner is required") end
			| if ($p.capabilities | type) == "array" then . else fail("capabilities must be an array") end
			| ($p.capabilities | map(select(known(.) | not)) | .[0]) as $unknown
			| if $unknown == null then . else fail($prefix + "unknown capability \"\($unknown)\"") end
			| ($p.capabilities | duplicate(.)) as $repeated
			| if $repeated == null then . else fail($prefix + "duplicate capability \"\($repeated)\"") end
			| if (($p.capabilities | index("container")) != null and $p.goos != "linux")
				then fail($prefix + "container capability currently requires linux") else . end
			| if (($p.capabilities | index("homebrew")) != null and $p.goos == "windows")
				then fail($prefix + "homebrew capability is not supported on windows") else . end
			| {goos: $p.goos, goarch: $p.goarch, runner: $p.runner, capabilities: $p.capabilities}
		] as $platforms
		| ($platforms | map(.goos + "/" + .goarch) | duplicate(.)) as $duplicated
		| if $duplicated == null then . else fail("duplicate platform \($duplicated)") end
		| [$platforms[] | select((.capabilities | index($capability)) != null) | {
			artifact: (.goos + "-" + .goarch),
			capabilities: .capabilities,
			goarch: .goarch,
			goos: .goos,
			runner: .runner
		}] as $include
		| if ($include | length) > 0 then {include: $include} else fail("no platform provides capability \"\($capability)\"") end
	' "$registry" 2>"$err"
) || {
	printf 'resolve platform matrix: %s\n' "$(cat "$err")" >&2
	exit 1
}

printf '%s\n' "$matrix"
if [ -n "$github_output" ]; then
	printf 'matrix=%s\n' "$matrix" >> "$github_output"
fi
