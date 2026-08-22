# Inventory nested commands exposed by help

## Command
```shell
cd /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli && for report in output/0{10..26}_*.md; do echo "== $report =="; sed -n "/Available Commands:/,/Flags:/p" "$report"; done
```

## Output
```text
== output/010_auth_help.md ==
Available Commands:
  check            Validate an account token
  export           Export stored authentication
  import           Import or replace an account
  list             List accounts
  login            Login with the Pixiv browser OAuth flow
  pool             Manage account pool scheduling
  refresh          Refresh account credentials and membership status
  remove           Remove an account
  use              Set the default account

Flags:
== output/011_bookmark_help.md ==
Available Commands:
  add         Bookmark an illustration
  detail      Show the current user's bookmark detail
  list        List artwork or novel bookmarks
  remove      Remove an illustration bookmark
  tags        List artwork bookmark tags

Flags:
== output/012_comment_help.md ==
== output/013_config_help.md ==
Available Commands:
  get         Print one effective config value
  path        Print the config.toml path
  set         Set one config value in config.toml
  unset       Remove one config value from config.toml

Flags:
== output/014_detail_help.md ==
== output/015_download_help.md ==
== output/016_follow_help.md ==
Available Commands:
  add         Follow a user
  remove      Unfollow a user

Flags:
== output/017_mcp_help.md ==
== output/018_mypixiv_help.md ==
Available Commands:
  users       List MyPixiv users for the authenticated account
  works       Browse MyPixiv works or one user's works

Flags:
== output/019_novel_help.md ==
Available Commands:
  search      Search novels

Flags:
== output/020_ranking_help.md ==
== output/021_recommended_help.md ==
== output/022_search_help.md ==
== output/023_series_help.md ==
== output/024_timeline_help.md ==
Available Commands:
  following   Browse new works from followed users
  latest      Browse Pixiv's latest works

Flags:
== output/025_update_help.md ==
== output/026_user_help.md ==
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
```

Exit code: 0

Verdict: PASS
