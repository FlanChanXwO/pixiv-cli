package mcpserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

func (a *App) register(server *mcp.Server) {
	addTool(a, server, &mcp.Tool{Name: "set_download_path", Description: "Set the default local save location for images and animations."}, a.setDownloadPath)
	addTool(a, server, &mcp.Tool{Name: "download", Description: "Download one or more artworks by ID with intelligent storage rules."}, a.download)
	addTool(a, server, &mcp.Tool{Name: "refresh_token", Description: "Manually refresh Pixiv API token when encountering authentication errors."}, a.refreshToken)
	addTool(a, server, &mcp.Tool{Name: "set_refresh_token", Description: "Set or update the Pixiv refresh token for authentication."}, a.setRefreshToken)
	addTool(a, server, &mcp.Tool{Name: "download_random_from_recommendation", Description: "Download random artworks from recommendations."}, a.downloadRandom)
	addTool(a, server, &mcp.Tool{Name: "search_illust", Description: "Search for illustrations using keywords with filters.", InputSchema: searchIllustInputSchema()}, a.searchIllust)
	addTool(a, server, &mcp.Tool{Name: "search_illust_options", Description: "List drawing tools available for an authenticated illustration search."}, a.searchIllustOptions)
	addTool(a, server, &mcp.Tool{Name: "illust_detail", Description: "Get detailed information about a specific artwork."}, a.illustDetail)
	addTool(a, server, &mcp.Tool{Name: "illust_related", Description: "Find artworks related to a specific illustration."}, a.illustRelated)
	addTool(a, server, &mcp.Tool{Name: "illust_ranking", Description: "Browse Pixiv rankings."}, a.illustRanking)
	addTool(a, server, &mcp.Tool{Name: "search_user", Description: "Search for users/artists on Pixiv."}, a.searchUser)
	addTool(a, server, &mcp.Tool{Name: "illust_recommended", Description: "Get personalized artwork recommendations."}, a.illustRecommended)
	addTool(a, server, &mcp.Tool{Name: "recommended", Description: "Get typed personalized recommendations through the Pixiv SDK."}, a.recommended)
	addTool(a, server, &mcp.Tool{Name: "trending_tags_illust", Description: "Get currently trending illustration tags."}, a.trendingTags)
	addTool(a, server, &mcp.Tool{Name: "illust_follow", Description: "Browse artworks from followed artists."}, a.illustFollow)
	addTool(a, server, &mcp.Tool{Name: "user_detail", Description: "Get a user's complete profile through the authenticated Pixiv SDK."}, a.userDetail)
	addTool(a, server, &mcp.Tool{Name: "user_artworks", Description: "Browse a user's artworks."}, a.userArtworks)
	addTool(a, server, &mcp.Tool{Name: "user_bookmarks", Description: "Browse user's bookmarked artworks."}, a.userBookmarks)
	addTool(a, server, &mcp.Tool{Name: "user_following", Description: "View user's following list."}, a.userFollowing)
	addTool(a, server, &mcp.Tool{Name: "add_bookmark", Description: "Add an artwork to bookmarks."}, a.addBookmark)
	addTool(a, server, &mcp.Tool{Name: "remove_bookmark", Description: "Remove an artwork from bookmarks."}, a.removeBookmark)
	addTool(a, server, &mcp.Tool{Name: "follow_user", Description: "Follow a Pixiv user."}, a.followUser)
	addTool(a, server, &mcp.Tool{Name: "unfollow_user", Description: "Unfollow a Pixiv user."}, a.unfollowUser)
	addTool(a, server, &mcp.Tool{Name: "get_thumbnail_base64", Description: "Get artwork thumbnail as base64 data URL."}, a.thumbnailBase64)
}
