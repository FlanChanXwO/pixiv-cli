# Extract confirmed syntax for planned read-only e2e commands

## Command
```shell
cd /Users/flanchan/Developer/Projects/GithubProjects/pixiv-cli && for report in output/014_detail_help.md output/015_download_help.md output/020_ranking_help.md output/021_recommended_help.md output/022_search_help.md output/023_series_help.md output/024_timeline_help.md output/049_mypixiv_users_help.md output/050_mypixiv_works_help.md output/051_novel_search_help.md output/052_timeline_following_help.md output/053_timeline_latest_help.md output/054_user_artworks_help.md output/056_user_bookmarks_help.md output/057_user_detail_help.md output/059_user_followers_help.md output/060_user_following_help.md output/061_user_novels_help.md output/062_user_related_help.md output/063_user_search_help.md; do echo "== $report =="; sed -n "/Usage:/,/Examples:/p" "$report"; done
```

## Output
```text
== output/014_detail_help.md ==
Usage:
  pixiv detail ID_OR_URL [flags]

Flags:
      --content        for novels, read structured novel content instead of metadata
  -h, --help           help for detail
  -j, --json           print JSON
      --no-proxy       clear the configured proxy for this command
      --proxy string   proxy URL (http, https, socks5, or socks5h) for this command
  -t, --type string    entity type: artwork, novel, user (default "artwork")
```

Exit code: 0

Verdict: PASS
== output/015_download_help.md ==
Usage:
  pixiv download [SRC...] [flags]

Flags:
      --download-path string       download directory
      --filename-template string   filename template placeholders: {id}, {title}, {author}, {author_id}, {date}, {tags}, {num}
  -h, --help                       help for download
      --no-proxy                   clear the configured proxy for this command
      --on-error string            record failure strategy: skip or fail-fast (default "skip")
  -o, --output string              download directory (alias for --download-path)
      --pages string               1-based page selection, e.g. 1,3-5; default all pages
      --proxy string               proxy URL (http, https, socks5, or socks5h) for this command
      --quality string             static image quality: original, regular, small, thumb, mini (default "original")
      --ugoira-mode string         ugoira output mode: gif, apng (default "gif")
```

Exit code: 0

Verdict: PASS
== output/020_ranking_help.md ==
Usage:
  pixiv ranking [flags]

Flags:
      --date string    YYYY-MM-DD
  -h, --help           help for ranking
  -j, --json           print JSON
  -l, --limit int      maximum results; omitted returns one upstream batch; 0 returns all results
      --mode string    ranking mode: day, day_male, day_female, week, week_original, week_rookie, month, day_manga, week_manga, month_manga, week_rookie_manga, day_r18, day_male_r18, day_female_r18, week_r18, week_r18g; the last nine require authentication (default "day")
      --ndjson         print one Pixiv entity record as JSON per line
      --no-proxy       clear the configured proxy for this command
  -p, --page int       1-based logical page (requires --limit > 0)
      --proxy string   proxy URL (http, https, socks5, or socks5h) for this command
```

Exit code: 0

Verdict: PASS
== output/021_recommended_help.md ==
Usage:
  pixiv recommended [KIND] [flags]

Flags:
      --content-type string   artwork subtype: all, illust, manga (default "all")
  -h, --help                  help for recommended
  -j, --json                  print JSON
  -l, --limit int             maximum results; omitted returns one upstream batch; 0 returns all results
      --ndjson                print one Pixiv entity record as JSON per line
      --no-proxy              clear the configured proxy for this command
  -p, --page int              1-based logical page (requires --limit > 0)
      --proxy string          proxy URL (http, https, socks5, or socks5h) for this command
  -t, --type string           entity type: artwork, novel, user, all
```

Exit code: 0

Verdict: PASS
== output/022_search_help.md ==
Usage:
  pixiv search [WORD] [flags]

Examples:
== output/023_series_help.md ==
Usage:
  pixiv series SERIES_ID [flags]

Flags:
  -h, --help           help for series
  -j, --json           print JSON
  -l, --limit int      maximum results; omitted returns one upstream batch; 0 returns all results
      --ndjson         print one Pixiv entity record as JSON per line
      --no-proxy       clear the configured proxy for this command
  -p, --page int       1-based logical page (requires --limit > 0)
      --proxy string   proxy URL (http, https, socks5, or socks5h) for this command
  -t, --type string    entity type: artwork or novel (required)
```

Exit code: 0

Verdict: PASS
== output/024_timeline_help.md ==
Usage:
  pixiv timeline [command]

Available Commands:
  following   Browse new works from followed users
  latest      Browse Pixiv's latest works

Flags:
  -h, --help   help for timeline

Use "pixiv timeline [command] --help" for more information about a command.
```

Exit code: 0

Verdict: PASS
== output/049_mypixiv_users_help.md ==
Usage:
  pixiv mypixiv users [flags]

Flags:
  -h, --help           help for users
  -j, --json           print JSON
  -l, --limit int      maximum results; omitted returns one upstream batch; 0 returns all results
      --ndjson         print one Pixiv entity record as JSON per line
      --no-proxy       clear the configured proxy for this command
  -p, --page int       1-based logical page (requires --limit > 0)
      --proxy string   proxy URL (http, https, socks5, or socks5h) for this command
```

Exit code: 0

Verdict: PASS
== output/050_mypixiv_works_help.md ==
Usage:
  pixiv mypixiv works [USER_ID] [flags]

Flags:
  -h, --help           help for works
  -j, --json           print JSON
  -l, --limit int      maximum results; omitted returns one upstream batch; 0 returns all results
      --ndjson         print one Pixiv entity record as JSON per line
      --no-proxy       clear the configured proxy for this command
  -p, --page int       1-based logical page (requires --limit > 0)
      --proxy string   proxy URL (http, https, socks5, or socks5h) for this command
  -t, --type string    required entity type: artwork or novel
```

Exit code: 0

Verdict: PASS
== output/051_novel_search_help.md ==
Usage:
  pixiv novel search WORD [flags]

Examples:
== output/052_timeline_following_help.md ==
Usage:
  pixiv timeline following [flags]

Flags:
      --content-type string   artwork subtype: all, illust-and-ugoira, illust, manga, ugoira (default "all")
  -h, --help                  help for following
  -j, --json                  print JSON
  -l, --limit int             maximum results; omitted returns one upstream batch; 0 returns all results
      --ndjson                print one Pixiv entity record as JSON per line
      --no-proxy              clear the configured proxy for this command
  -p, --page int              1-based logical page (requires --limit > 0)
      --proxy string          proxy URL (http, https, socks5, or socks5h) for this command
      --restrict string       follow visibility: public or private (default "public")
  -t, --type string           required entity type: artwork or novel
```

Exit code: 0

Verdict: PASS
== output/053_timeline_latest_help.md ==
Usage:
  pixiv timeline latest [flags]

Flags:
      --content-type string   artwork subtype: all, illust-and-ugoira, illust, manga, ugoira (default "all")
  -h, --help                  help for latest
  -j, --json                  print JSON
  -l, --limit int             maximum results; omitted returns one upstream batch; 0 returns all results
      --ndjson                print one Pixiv entity record as JSON per line
      --no-proxy              clear the configured proxy for this command
  -p, --page int              1-based logical page (requires --limit > 0)
      --proxy string          proxy URL (http, https, socks5, or socks5h) for this command
  -t, --type string           required entity type: artwork or novel; use --content-type for subtype
```

Exit code: 0

Verdict: PASS
== output/054_user_artworks_help.md ==
Usage:
  pixiv user artworks [USER_ID] [flags]

Flags:
  -h, --help           help for artworks
  -j, --json           print JSON
  -l, --limit int      maximum results; omitted returns one upstream batch; 0 returns all results
      --ndjson         print one Pixiv entity record as JSON per line
      --no-proxy       clear the configured proxy for this command
  -p, --page int       1-based logical page (requires --limit > 0)
      --proxy string   proxy URL (http, https, socks5, or socks5h) for this command
  -t, --type string    illust type: illust, manga, ugoira (default "illustration")
```

Exit code: 0

Verdict: PASS
== output/056_user_bookmarks_help.md ==
Usage:
  pixiv user bookmarks [USER_ID] [flags]

Flags:
  -h, --help              help for bookmarks
  -j, --json              print JSON
  -l, --limit int         maximum results; omitted returns one upstream batch; 0 returns all results
      --ndjson            print one Pixiv entity record as JSON per line
      --no-proxy          clear the configured proxy for this command
  -p, --page int          1-based logical page (requires --limit > 0)
      --proxy string      proxy URL (http, https, socks5, or socks5h) for this command
      --restrict string   bookmark visibility (public or private) (default "public")
      --tag string        filter by bookmark tag
```

Exit code: 0

Verdict: PASS
== output/057_user_detail_help.md ==
Usage:
  pixiv user detail USER_ID [flags]

Flags:
  -h, --help           help for detail
  -j, --json           print JSON
      --no-proxy       clear the configured proxy for this command
      --proxy string   proxy URL (http, https, socks5, or socks5h) for this command
```

Exit code: 0

Verdict: PASS
== output/059_user_followers_help.md ==
Usage:
  pixiv user followers [USER_ID] [flags]

Flags:
  -h, --help              help for followers
  -j, --json              print JSON
  -l, --limit int         maximum results; omitted returns one upstream batch; 0 returns all results
      --ndjson            print one Pixiv entity record as JSON per line
      --no-proxy          clear the configured proxy for this command
  -p, --page int          1-based logical page (requires --limit > 0)
      --proxy string      proxy URL (http, https, socks5, or socks5h) for this command
      --restrict string   follow visibility (public or private) (default "public")
```

Exit code: 0

Verdict: PASS
== output/060_user_following_help.md ==
Usage:
  pixiv user following [USER_ID] [flags]

Flags:
  -h, --help              help for following
  -j, --json              print JSON
  -l, --limit int         maximum results; omitted returns one upstream batch; 0 returns all results
      --ndjson            print one Pixiv entity record as JSON per line
      --no-proxy          clear the configured proxy for this command
  -p, --page int          1-based logical page (requires --limit > 0)
      --proxy string      proxy URL (http, https, socks5, or socks5h) for this command
      --restrict string   follow visibility (public or private) (default "public")
```

Exit code: 0

Verdict: PASS
== output/061_user_novels_help.md ==
Usage:
  pixiv user novels [USER_ID] [flags]

Flags:
  -h, --help           help for novels
  -j, --json           print JSON
  -l, --limit int      maximum results; omitted returns one upstream batch; 0 returns all results
      --ndjson         print one Pixiv entity record as JSON per line
      --no-proxy       clear the configured proxy for this command
  -p, --page int       1-based logical page (requires --limit > 0)
      --proxy string   proxy URL (http, https, socks5, or socks5h) for this command
```

Exit code: 0

Verdict: PASS
== output/062_user_related_help.md ==
Usage:
  pixiv user related USER_ID [flags]

Flags:
  -h, --help           help for related
  -j, --json           print JSON
  -l, --limit int      maximum results; omitted returns one upstream batch; 0 returns all results
      --ndjson         print one Pixiv entity record as JSON per line
      --no-proxy       clear the configured proxy for this command
  -p, --page int       1-based logical page (requires --limit > 0)
      --proxy string   proxy URL (http, https, socks5, or socks5h) for this command
```

Exit code: 0

Verdict: PASS
== output/063_user_search_help.md ==
Usage:
  pixiv user search WORD [flags]

Examples:
```

Exit code: 0

Verdict: PASS
