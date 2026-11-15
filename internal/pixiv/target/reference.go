package target

// ReferenceKind 保留命令输入契约需要区分的 Pixiv 页面种类。
type ReferenceKind string

// ReferenceKindArtwork 及其余常量区分命令可接受的稳定页面种类。
const (
	ReferenceKindArtwork       ReferenceKind = "artwork"
	ReferenceKindNovel         ReferenceKind = "novel"
	ReferenceKindUser          ReferenceKind = "user"
	ReferenceKindUserBookmarks ReferenceKind = "user_bookmarks"
	ReferenceKindArtworkSeries ReferenceKind = "artwork_series"
	ReferenceKindNovelSeries   ReferenceKind = "novel_series"
)

type reference struct {
	Kind        ReferenceKind
	ID          int64
	OwnerUserID int64
}
