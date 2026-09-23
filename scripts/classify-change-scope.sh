#!/usr/bin/env bash
set -euo pipefail

base=
head=
github_output=
while (($#)); do
  case "$1" in
    --base) base=${2-}; shift 2 ;;
    --head) head=${2-}; shift 2 ;;
    --github-output) github_output=${2-}; shift 2 ;;
    *) printf 'unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
done

if [[ -z "$head" ]]; then
  printf '%s\n' 'classify change scope: head commit is required' >&2
  exit 1
fi

repo_root=$(git rev-parse --show-toplevel)
rules="$repo_root/.github/ci-docs-only.gitignore"
if [[ ! -f "$rules" ]]; then
  printf 'classify change scope: rules file not found: %s\n' "$rules" >&2
  exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
changed="$tmp/changed"
matched="$tmp/matched"
matcher="$tmp/matcher"
mkdir "$matcher"
git -C "$matcher" init -q

emit_scope() {
  local docs_only=$1 quality_required=$2 platform_required=$3 container_required=$4 native_required=$5
  local output
  output=$(printf 'docs_only=%s\nquality_required=%s\nplatform_required=%s\ncontainer_required=%s\nnative_required=%s\n' \
    "$docs_only" "$quality_required" "$platform_required" "$container_required" "$native_required")
  printf '%s\n' "$output"
  if [[ -n "$github_output" ]]; then
    printf '%s\n' "$output" >> "$github_output"
  fi
}

full_scope() {
  emit_scope false true true "$1" true
}

if [[ -z "$base" || "$base" =~ ^0+$ ]]; then
  full_scope true
  printf '%s\n' 'no usable base commit; selecting full validation' >&2
  exit 0
fi

if ! git diff --name-only --no-renames -z "$base" "$head" > "$changed"; then
  printf 'classify change scope: diff %s..%s failed\n' "$base" "$head" >&2
  exit 1
fi
if [[ ! -s "$changed" ]]; then
  full_scope true
  printf '%s\n' 'empty diff; selecting full validation' >&2
  exit 0
fi

# Match in an isolated temporary repository so the project's own .gitignore cannot widen this allowlist.
match_status=0
git -C "$matcher" -c core.excludesFile="$rules" check-ignore --no-index -z --stdin < "$changed" > "$matched" || match_status=$?
if ((match_status > 1)); then
  printf '%s\n' 'classify change scope: documentation rule matching failed' >&2
  exit "$match_status"
fi
if cmp -s "$changed" "$matched"; then
  emit_scope true false false false false
  printf '%s\n' 'only approved documentation paths changed; selecting documentation validation' >&2
  exit 0
fi

container_required=false
while IFS= read -r -d '' path; do
  case "$path" in
    Dockerfile|.dockerignore|go.mod|go.sum|Cargo.toml|Cargo.lock|cmd/*|internal/*|sdk/*|native/*|ci/*|tools/platformmatrix/*|.github/workflows/container-smoke.yml|scripts/build-platform.sh|scripts/build-staticlibs*|scripts/cmd/releaseassets/*)
      container_required=true
      break
      ;;
  esac
done < "$changed"

full_scope "$container_required"
printf '%s\n' 'non-document change detected; selecting required validation' >&2
