#!/bin/zsh
set -euo pipefail

usage() {
  print -u2 -r -- 'usage: run-case.zsh --run-dir DIR --group SLUG --case SLUG --title TEXT --purpose TEXT --cwd DIR [--expect-exit CODE] --expected TEXT --assertion TEXT -- PIXIV [ARG ...]'
}

run_dir=
group=
case_id=
title=
purpose=
cwd=
expected_exit=0
expected=
assertion=

while (( $# > 0 )); do
  case "$1" in
    --run-dir|--group|--case|--title|--purpose|--cwd|--expect-exit|--expected|--assertion)
      option=$1
      shift
      (( $# > 0 )) || { usage; exit 2; }
      case "$option" in
        --run-dir) run_dir=$1 ;;
        --group) group=$1 ;;
        --case) case_id=$1 ;;
        --title) title=$1 ;;
        --purpose) purpose=$1 ;;
        --cwd) cwd=$1 ;;
        --expect-exit) expected_exit=$1 ;;
        --expected) expected=$1 ;;
        --assertion) assertion=$1 ;;
      esac
      shift
      ;;
    --)
      shift
      break
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

[[ -n "$run_dir" && -n "$group" && -n "$case_id" && -n "$title" && -n "$purpose" && -n "$cwd" && -n "$expected" && -n "$assertion" && $# -gt 0 ]] || {
  usage
  exit 2
}
[[ "$group" =~ '^[a-z0-9][a-z0-9-]*$' && "$case_id" =~ '^[a-z0-9][a-z0-9-]*$' ]] || {
  print -u2 -r -- 'group and case must be lowercase slugs'
  exit 2
}
[[ "$expected_exit" == <0-> ]] || {
  print -u2 -r -- 'expected exit code must be a non-negative integer'
  exit 2
}

cwd=${cwd:A}
run_dir=${run_dir:A}
[[ -d "$cwd" ]] || {
  print -u2 -r -- "case cwd does not exist: $cwd"
  exit 2
}
[[ "$cwd" == /private/tmp/* ]] || {
  print -u2 -r -- "case cwd must be under /private/tmp: $cwd"
  exit 2
}
[[ "$cwd" != "$run_dir" && "$cwd" != "$run_dir"/* ]] || {
  print -u2 -r -- "case cwd must not be the report directory or one of its descendants: $cwd"
  exit 2
}
[[ ${1:t} == pixiv ]] || {
  print -u2 -r -- 'case command must invoke a pixiv executable directly'
  exit 2
}

case_dir=$run_dir/cases/$group/$case_id
[[ ! -e "$case_dir" ]] || {
  print -u2 -r -- "case evidence already exists: $case_dir"
  exit 2
}
mkdir -p "$case_dir"

stdout_file=$case_dir/stdout.txt
stderr_file=$case_dir/stderr.txt
report_file=$case_dir/report.md

set +e
(
  cd "$cwd"
  "$@"
) >"$stdout_file" 2>"$stderr_file"
actual_exit=$?
set -e

verdict=FAIL
basis="expected exit code $expected_exit, received $actual_exit"
if (( actual_exit == expected_exit )); then
  verdict=PASS
  basis="actual exit code matched expected exit code $expected_exit"
fi

{
  print -r -- "# $title"
  print
  print -r -- '## Purpose'
  print
  print -r -- "$purpose"
  print
  print -r -- '## Invocation'
  print
  print -r -- "- cwd: \`$cwd\`"
  print -r -- "- shell: \`zsh $ZSH_VERSION\`"
  print -r -- "- expected exit code: \`$expected_exit\`"
  print -r -- "- actual exit code: \`$actual_exit\`"
  print
  print -r -- '## argv'
  print
  print -r -- '```zsh'
  first_arg=true
  for arg in "$@"; do
    if [[ "$first_arg" == true ]]; then
      first_arg=false
    else
      printf ' '
    fi
    printf '%q' "$arg"
  done
  print
  print -r -- '```'
  print
  print -r -- '## Expected result'
  print
  print -r -- "$expected"
  print
  print -r -- '## Assertion'
  print
  print -r -- "$assertion"
  print
  print -r -- '## Evidence'
  print
  print -r -- '- Pixiv CLI stdout: `stdout.txt`'
  print -r -- '- Pixiv CLI stderr: `stderr.txt`'
  print
  print -r -- '## Result'
  print
  print -r -- "- verdict: \`$verdict\`"
  print -r -- "- basis: $basis"
} >"$report_file"

print -r -- "$case_dir"
[[ "$verdict" == PASS ]]
