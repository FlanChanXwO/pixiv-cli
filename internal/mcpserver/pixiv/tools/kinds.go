package tools

import (
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// kindAll 是推荐与作品列表共用的聚合选择值。
const kindAll = "all"

// contentNovel 给最新时间线补充小说类型；公开 schema 接受它，而 SDK 的搜索类型没有该取值。
const contentNovel pixiv.SearchContentType = "novel"

// 实体 kind 直接复用 SDK 公开的引用类型取值。
const (
	kindArtwork = string(pixiv.ReferenceKindArtwork)
	kindNovel   = string(pixiv.ReferenceKindNovel)
	kindUser    = string(pixiv.ReferenceKindUser)
)
