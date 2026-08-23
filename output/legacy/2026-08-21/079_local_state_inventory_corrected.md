# Inspect local Pixiv state paths without reading secret contents

## Command
```shell
printf "home=%s\n" "$HOME"; for path in "$HOME/.pixiv-cli" "$HOME/.config/pixiv-cli" "$HOME/Library/Application Support/pixiv-cli" "$HOME/Library/Application Support/pixiv"; do if [[ -d "$path" ]]; then echo "directory=$path"; /usr/bin/find "$path" -maxdepth 2 -type f -print | /usr/bin/sed -E "s#/(accounts|auth|token|tokens|credentials)[^/]*$#/\1[REDACTED]#"; fi; done
```

## Output
```text
home=/Users/flanchan
directory=/Users/flanchan/.pixiv-cli
sed: 1: "s#/(accounts|auth|token ...": unescaped newline inside substitute pattern
directory=/Users/flanchan/Library/Application Support/pixiv
sed: 1: "s#/(accounts|auth|token ...": unescaped newline inside substitute pattern
```

Exit code: 1

Verdict: FAIL
