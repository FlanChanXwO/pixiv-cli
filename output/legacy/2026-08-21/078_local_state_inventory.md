# Inspect configured local Pixiv state without reading secrets

## Command
```shell
printf "home=%s\n" "$HOME"; for path in "$HOME/.config/pixiv-cli" "$HOME/Library/Application Support/pixiv-cli" "$HOME/Library/Application Support/pixiv"; do if [[ -d "$path" ]]; then echo "directory=$path"; find "$path" -maxdepth 2 -type f -print | sed -E "s#/(accounts|auth|token|tokens|credentials)[^/]*$#/\1[REDACTED]#"; fi; done
```

## Output
```text
home=/Users/flanchan
directory=/Users/flanchan/Library/Application Support/pixiv
zsh:1: command not found: find
zsh:1: command not found: sed
```

Exit code: 127

Verdict: FAIL
