# Verify real shell environment and working directories

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && printf "shell=%s\n" "$SHELL" && printf "zsh_version=%s\n" "$ZSH_VERSION" && printf "cwd=%s\n" "$PWD" && printf "source_worktree=%s\n" /private/tmp/pixiv-cli-pixiv-e2e-fixes && printf "report_directory=%s\n" /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli/output && command -v zsh && uname -a
```

## Output
```text
shell=/opt/homebrew/bin/fish
zsh_version=5.9
cwd=/private/tmp/pixiv-cli-e2e-shell-20260821
source_worktree=/private/tmp/pixiv-cli-pixiv-e2e-fixes
report_directory=/Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli/output
/bin/zsh
Darwin Marisa-s-Air 25.5.0 Darwin Kernel Version 25.5.0: Tue Jun  9 22:28:17 PDT 2026; root:xnu-12377.121.10~1/RELEASE_ARM64_T8142 arm64
```

Exit code: 0

Verdict: PASS
