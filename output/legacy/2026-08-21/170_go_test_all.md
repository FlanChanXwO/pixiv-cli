# Final Go test suite

## Command
```shell
cd /private/tmp/pixiv-cli-pixiv-e2e-fixes && go test ./...
```

## Output
```text
?   	github.com/FlanChanXwO/pixiv-cli/cmd/pixiv	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/e2e	26.881s
ok  	github.com/FlanChanXwO/pixiv-cli/internal/browsercookies	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/chromium	2.396s
ok  	github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/firefox	1.170s
ok  	github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/safari	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/secret	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/sqliteio	5.517s
ok  	github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/system	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/config	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox/auth	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox/download	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox/internal/listing	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox/mcp	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/fanbox/post	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/auth	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/auth/loginhelper	4.709s
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/auth/loginpage	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/bookmark	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/comment	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/detail	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/download	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/follow	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/internal/listing	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/mcp	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/mypixiv	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/ranking	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/recommended	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/search	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/series	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/timeline	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/pixiv/user	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/commands/update	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/diagnostics	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/invocation	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/cli/pipeline	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/config/paths	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/config/settings	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/internal/runtime	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/creator	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/creatorposts	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/creators	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/creatortags	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/currentuser	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/home	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/openresource	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/post	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/resolveurl	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/supporting	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/fanbox/tools/taggedposts	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/filters	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/records	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/runtime	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/add_bookmark	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/blocked_users	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/bookmark_detail	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/bookmark_tags	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/download	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/follow_user	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_comments	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_detail	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_ranking	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_recommended	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_related	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/illust_series	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/mypixiv_illusts	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/mypixiv_novels	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/mypixiv_users	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_comments	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_content	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_detail	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/novel_series	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/recommended	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/related_users	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/remove_bookmark	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/search_illust	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/search_novel	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/search_user	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/timeline_illust_following	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/timeline_illust_latest	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/timeline_novel_following	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/timeline_novel_latest	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/trending_tags_illust	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/unfollow_user	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_artworks	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_bookmarks	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_detail	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_followers	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_following	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_novel_bookmarks	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/tools/user_novels	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/media/downloader	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/media/downloader/filename	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/media/downloader/parallel	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira	5.221s
ok  	github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira/staticlib	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/account	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/creator	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/creator/creators	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/creator/tags	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/home	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/info	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/posts	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/supporting	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post/wire	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/protocol	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/resource	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/appapi	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/bookmark	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/comments	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/detail	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/ranking	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/recommended	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/related	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/search	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/series	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/timeline	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork/trending	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel/comments	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel/detail	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel/recommended	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel/search	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel/series	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel/timeline	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/blocked	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/detail	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/follow	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/followers	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/following	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/mypixiv	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/novelbookmarks	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/novels	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/recommended	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/related	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/search	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/user/visibility	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/oauth	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/pool	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/resource	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/shared/buildinfo	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/shared/diagnostics	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/shared/network	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/shared/pagination	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/shared/record	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/shared/traversal	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/storage/database	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/storage/file/atomic	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/storage/file/lock	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/storage/file/replace	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/storage/file/secret	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/update	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/update/installer	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/update/process	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/update/release	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/update/source	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/internal/utils/date	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/internal/utils/parse	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/utils/text	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/internal/utils/uri	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/scripts/cmd/browsernativeevidence	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/cmd/changescope	7.661s
?   	github.com/FlanChanXwO/pixiv-cli/scripts/cmd/homebrewformula	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/scripts/cmd/licensebundle	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/scripts/cmd/linuxabi	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/cmd/nativeevidence	4.086s
?   	github.com/FlanChanXwO/pixiv-cli/scripts/cmd/prepublishhomebrew	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/cmd/publicapi	8.935s
?   	github.com/FlanChanXwO/pixiv-cli/scripts/cmd/releaseassets	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/scripts/cmd/releasenotes	[no test files]
?   	github.com/FlanChanXwO/pixiv-cli/scripts/cmd/releaseworkflow	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/internal/browsernativeevidence	4.818s
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/internal/changescope	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/internal/homebrewformula	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/internal/licensebundle	7.815s
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/internal/linuxabi	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/internal/nativeevidence	23.706s
?   	github.com/FlanChanXwO/pixiv-cli/scripts/internal/platformsmokeworkflow	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/internal/prepublishhomebrew	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/internal/publicapi	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/internal/releaseassets	3.359s
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/internal/releasecontract	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/internal/releasenotes	(cached)
?   	github.com/FlanChanXwO/pixiv-cli/scripts/internal/releasenotesrender	[no test files]
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/internal/releaseworkflow	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/internal/workflow/yaml	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/tests/clawhubworkflow	3.618s
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/tests/installers	10.202s
ok  	github.com/FlanChanXwO/pixiv-cli/scripts/tests/platformsmokeworkflow	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/sdk	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/sdk/fanbox	(cached)
ok  	github.com/FlanChanXwO/pixiv-cli/sdk/pixiv	(cached)
```

Exit code: 0

Verdict: PASS
