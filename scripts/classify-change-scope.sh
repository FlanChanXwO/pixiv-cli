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
[[ -f "$rules" ]] || { printf 'classify change scope: rules file not found: %s\n' "$rules" >&2; exit 1; }

emit_scope() {
  local output
  output=$(printf 'docs_only=%s\nquality_required=%s\nplatform_required=%s\ncontainer_required=%s\nnative_required=%s\n' "$@")
  printf '%s\n' "$output"
  [[ -z "$github_output" ]] || printf '%s\n' "$output" >> "$github_output"
}

full_scope() { emit_scope false true true "$1" true; }

if [[ -z "$base" || "$base" =~ ^0+$ ]]; then
  full_scope true
  printf '%s\n' 'no usable base commit; selecting full validation' >&2
  exit 0
fi

changed=$(mktemp)
trap 'rm -f "$changed"' EXIT
if ! git diff --name-only --no-renames -z "$base" "$head" > "$changed"; then
  printf 'classify change scope: diff %s..%s failed\n' "$base" "$head" >&2
  exit 1
fi
if [[ ! -s "$changed" ]]; then
  full_scope true
  printf '%s\n' 'empty diff; selecting full validation' >&2
  exit 0
fi

matches_docs_rule() {
  git --literal-pathspecs ls-files --cached --ignored --exclude-from="$rules" --with-tree="$head" --error-unmatch -- "$1" >/dev/null 2>&1 ||
    git --literal-pathspecs ls-files --cached --ignored --exclude-from="$rules" --with-tree="$base" --error-unmatch -- "$1" >/dev/null 2>&1
}

docs_only=true
container_required=false
while IFS= read -r -d '' path; do
  matches_docs_rule "$path" || docs_only=false
  case "$path" in
    Dockerfile|.dockerignore|go.mod|go.sum|Cargo.toml|Cargo.lock|cmd/*|internal/*|sdk/*|native/*|ci/*|tools/platformmatrix/*|.github/workflows/container-smoke.yml|scripts/build-platform.sh|scripts/build-staticlibs*|scripts/cmd/releaseassets/*) container_required=true ;;
  esac
done < "$changed"

if [[ "$docs_only" = true ]]; then
  emit_scope true false false false false
  printf '%s\n' 'only approved documentation paths changed; selecting documentation validation' >&2
else
  full_scope "$container_required"
  printf '%s\n' 'non-document change detected; selecting required validation' >&2
fi
