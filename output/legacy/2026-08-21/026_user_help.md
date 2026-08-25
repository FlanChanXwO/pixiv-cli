# Inspect user command syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv user --help
```

## Output
```text
Query a Pixiv user

Usage:
  pixiv user [command]

Available Commands:
  artworks    List a user's artworks
  blocked     List users blocked by a user
  bookmarks   List a user's bookmarks
  detail      Show one user's complete profile
  follow      Manage followed users
  followers   List a user's followers
  following   List users followed by a user
  novels      List a user's novels
  related     List users related to a user
  search      Search users

Flags:
  -h, --help   help for user

Use "pixiv user [command] --help" for more information about a command.
```

Exit code: 0

Verdict: PASS
