# Audit Task 1 report structure

## Command
```shell
cd /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli && for report in output/001_workspace_setup.md output/002_branch_baseline.md output/003_shell_environment.md output/004_directory_separation.md; do printf "%s: " "$report"; rg -q "^## Command$" "$report" && rg -q "^## Output$" "$report" && rg -q "^Exit code: [0-9]+$" "$report" && rg -q "^Verdict: (PASS|FAIL|BLOCKED)$" "$report" && echo valid || echo invalid; done && test -z "$(find output -mindepth 1 -maxdepth 1 ! -name "*.md" -print -quit)"
```

## Output
```text
output/001_workspace_setup.md: valid
output/002_branch_baseline.md: valid
output/003_shell_environment.md: valid
output/004_directory_separation.md: valid
```

Exit code: 0

Verdict: PASS
