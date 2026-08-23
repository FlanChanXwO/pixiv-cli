# Prove output contains reports only

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && printf "test_workspace_files:\n" && find . -maxdepth 2 -print | sort && printf "\nreport_directory_files:\n" && find /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli/output -maxdepth 1 -type f -name "*.md" -print | sort && printf "\nnon_markdown_entries_in_output:\n" && find /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli/output -mindepth 1 -maxdepth 1 ! -name "*.md" -print
```

## Output
```text
test_workspace_files:
.
./credentials
./downloads
./home
./legacy-output
./legacy-output/900_goal_recovery_inventory.md
./log-command.zsh
./tmp
./tmp/command.KHeN31

report_directory_files:
/Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli/output/001_workspace_setup.md
/Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli/output/002_branch_baseline.md
/Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli/output/003_shell_environment.md

non_markdown_entries_in_output:
```

Exit code: 0

Verdict: PASS
