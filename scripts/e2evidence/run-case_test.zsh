#!/bin/zsh
set -euo pipefail

script_dir=${0:A:h}
runner=$script_dir/run-case.zsh
fixture=$script_dir/testdata/pixiv
test_root=$(mktemp -d /private/tmp/pixiv-evidence-runner-test.XXXXXX)
trap 'rm -rf -- "$test_root"' EXIT

chmod +x "$fixture"
mkdir -p "$test_root/work" "$test_root/home"

"$runner" \
  --run-dir "$test_root/run" \
  --group smoke \
  --case stream-separation \
  --title 'stdout 与 stderr 分流' \
  --purpose '验证 runner 只把 Pixiv CLI 的两个输出流写入各自证据文件' \
  --cwd "$test_root/work" \
  --home "$test_root/home" \
  --expect-exit 7 \
  --expected 'Pixiv CLI 返回 7，两个输出流保持分离' \
  --assertion 'stdout.txt 和 stderr.txt 分别只含对应流，report.md 记录 PASS' \
  -- "$fixture" alpha beta

case_dir=$test_root/run/cases/smoke/stream-separation
[[ $(<"$case_dir/stdout.txt") == $'stdout:alpha\nhome:'"$test_root/home" ]]
[[ $(<"$case_dir/stderr.txt") == 'stderr:beta' ]]
rg -q '^# stdout 与 stderr 分流$' "$case_dir/report.md"
rg -q '^- cwd: `/.+/work`$' "$case_dir/report.md"
rg -q '^- HOME: `/.+/home`$' "$case_dir/report.md"
rg -q '^- shell: `zsh [0-9][^`]*`$' "$case_dir/report.md"
rg -q '^- expected exit code: `7`$' "$case_dir/report.md"
rg -q '^- actual exit code: `7`$' "$case_dir/report.md"
rg -q '^- verdict: `PASS`$' "$case_dir/report.md"
rg -q '^## argv$' "$case_dir/report.md"
rg -q 'stdout\.txt 和 stderr\.txt 分别只含对应流' "$case_dir/report.md"
if rg -n '[[:blank:]]$' "$case_dir/report.md"; then
  print -u2 -r -- 'report contains trailing whitespace'
  exit 1
fi

mkdir -p "$test_root/report-workdir"
set +e
"$runner" \
  --run-dir "$test_root/report-workdir" \
  --group smoke \
  --case rejects-report-cwd \
  --title '拒绝报告目录作为 cwd' \
  --purpose '验证 output/report 目录永远不承载命令执行' \
  --cwd "$test_root/report-workdir" \
  --expect-exit 7 \
  --expected 'runner 在执行 Pixiv CLI 前拒绝该 cwd' \
  --assertion 'runner 返回参数错误且不生成 case stdout/stderr' \
  -- "$fixture" alpha beta >/dev/null 2>&1
report_cwd_exit=$?
set -e
[[ $report_cwd_exit -eq 2 ]]
[[ ! -e "$test_root/report-workdir/cases/smoke/rejects-report-cwd" ]]

print -r -- 'run-case behavior: PASS'
