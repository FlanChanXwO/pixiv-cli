# Audit Task 2 reports and command preflight coverage

## Command
```shell
cd /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli && failed=0; for report in output/{006..072}_*.md; do if ! rg -q "^## Command$" "$report" || ! rg -q "^## Output$" "$report" || ! rg -q "^Exit code: 0$" "$report" || ! rg -q "^Verdict: PASS$" "$report"; then echo "invalid: $report"; failed=1; fi; done; total=$(find output -maxdepth 1 -type f -name "*.md" | wc -l | tr -d " "); help_count=$(find output -maxdepth 1 -type f -name "*_help.md" | wc -l | tr -d " "); printf "report_count=%s\nhelp_report_count=%s\n" "$total" "$help_count"; test "$failed" -eq 0
```

## Output
```text
invalid: output/008_version_json.md
report_count=72
help_report_count=58
```

Exit code: 1

Verdict: FAIL
