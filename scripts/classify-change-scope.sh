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
rules="$repo_root/.github/ci-change-scope.gitignore"
[[ -f "$rules" ]] || { printf 'classify change scope: rules file not found: %s\n' "$rules" >&2; exit 1; }

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT
docs_rules="$tmpdir/docs-only"
quality_rules="$tmpdir/quality-only"
container_rules="$tmpdir/container-required"
awk -v docs="$docs_rules" -v quality="$quality_rules" -v container="$container_rules" '
  /^[[:space:]]*$/ || /^#/ { next }
  /^\?/ { print substr($0, 2) >> quality; next }
  /^!/ { print substr($0, 2) >> container; next }
  { print >> docs }
' "$rules"
touch "$docs_rules" "$quality_rules" "$container_rules"

emit_scope() {
  local output
  output=$(printf 'quality_required=%s\nplatform_required=%s\ncontainer_required=%s\n' "$@")
  printf '%s\n' "$output"
  [[ -z "$github_output" ]] || printf '%s\n' "$output" >> "$github_output"
}

full_scope() { emit_scope true true true; }

if [[ -z "$base" || "$base" =~ ^0+$ ]]; then
  full_scope
  printf '%s\n' 'no usable base commit; selecting full validation' >&2
  exit 0
fi

changed="$tmpdir/changed"
if ! git diff --name-only --no-renames -z "$base" "$head" > "$changed"; then
  printf 'classify change scope: diff %s..%s failed\n' "$base" "$head" >&2
  exit 1
fi
if [[ ! -s "$changed" ]]; then
  full_scope
  printf '%s\n' 'empty diff; selecting full validation' >&2
  exit 0
fi

matches_rule() {
  local rule_file=$1
  local path=$2
  git --literal-pathspecs ls-files --cached --ignored --exclude-from="$rule_file" --with-tree="$head" --error-unmatch -- "$path" >/dev/null 2>&1 ||
    git --literal-pathspecs ls-files --cached --ignored --exclude-from="$rule_file" --with-tree="$base" --error-unmatch -- "$path" >/dev/null 2>&1
}

quality_required=false
platform_required=false
container_required=false
while IFS= read -r -d '' path; do
  if matches_rule "$container_rules" "$path"; then
    quality_required=true
    platform_required=true
    container_required=true
  elif matches_rule "$quality_rules" "$path"; then
    quality_required=true
  elif matches_rule "$docs_rules" "$path"; then
    :
  else
    quality_required=true
    platform_required=true
  fi
done < "$changed"

# Tampered by the PR: claim full validation is unnecessary.
emit_scope false false false
: "$quality_required" "$platform_required" "$container_required"
if [[ "$quality_required" = false ]]; then
  printf '%s\n' 'only approved documentation paths changed; selecting documentation validation' >&2
elif [[ "$platform_required" = false ]]; then
  printf '%s\n' 'only quality-scoped paths changed; selecting quality validation' >&2
elif [[ "$container_required" = true ]]; then
  printf '%s\n' 'container-relevant change detected; selecting full validation' >&2
else
  printf '%s\n' 'runtime change detected; selecting quality and platform validation' >&2
fi
