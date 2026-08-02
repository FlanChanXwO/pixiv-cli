package pixiv

import (
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

// Restrict selects the visibility scope of bookmarks, following, or
// recommendation sources.
type Restrict string

const (
	RestrictPublic  Restrict = "public"
	RestrictPrivate Restrict = "private"
)

// ArtworkKind distinguishes the artwork types unified in Artwork.
type ArtworkKind string

const (
	ArtworkKindIllustration ArtworkKind = "illustration"
	ArtworkKindManga        ArtworkKind = "manga"
	ArtworkKindUgoira       ArtworkKind = "ugoira"
	// ArtworkKindUnknown preserves an upstream kind the SDK does not yet
	// recognize; Artwork.RawKind carries the original identifier.
	ArtworkKindUnknown ArtworkKind = "unknown"
)

// Tag is a single Pixiv tag. TranslatedName is empty when upstream provided no
// translation.
type Tag struct {
	Name           string
	TranslatedName string
}

// ImageResource is a first-party image exposed through the shared resource
// contract. Variant records the upstream size role (for example "original",
// "medium", or "square_medium") when upstream provided one. Width and Height
// are zero when upstream did not provide them.
type ImageResource struct {
	Resource sdk.Resource
	Variant  string
	Width    int
	Height   int
}

// ArtworkPage is one page of a multi-page artwork. PageIndex is zero-based and
// follows the upstream display order. Image is a usable image resource.
type ArtworkPage struct {
	PageIndex int
	Image     ImageResource
	Width     int
	Height    int
}

// Artwork unifies illustrations, manga, and ugoira. It is addressable by its
// stable upstream ID. PublishedAt is always the UTC publication time; UpdatedAt
// is set only when upstream provides a modification time, and is nil otherwise.
//
// List operations populate the cover, identity, title, tags, user, and counters
// but leave Pages empty; Artwork detail populates Pages with every page. The
// completeness of a given field is part of the model contract and never faked.
type Artwork struct {
	ID             int64
	Title          string
	Caption        string
	Kind           ArtworkKind
	RawKind        string
	Tags           []Tag
	User           User
	PublishedAt    time.Time
	UpdatedAt      *time.Time
	TotalBookmarks int
	TotalViews     int
	Width          int
	Height         int
	PageCount      int
	XRestrict      int
	AIType         int
	Tools          []string
	Cover          ImageResource
	Pages          []ArtworkPage
}

// Novel is an addressable novel entry. List operations populate identity,
// title, user, tags, counters, and cover; Novel detail adds the caption and
// full text-length fields. PublishedAt is the UTC publication time and
// UpdatedAt is set only when upstream provides one.
type Novel struct {
	ID             int64
	Title          string
	Caption        string
	User           User
	Tags           []Tag
	PublishedAt    time.Time
	UpdatedAt      *time.Time
	XRestrict      int
	TextLength     int
	IsOriginal     bool
	TotalBookmarks int
	TotalViews     int
	Cover          ImageResource
}

// User is an addressable Pixiv user. ProfileImage is the profile image
// resource when upstream provides one.
type User struct {
	ID           int64
	Name         string
	Account      string
	Comment      string
	IsFollowed   bool
	ProfileImage ImageResource
}

// UserDetail adds profile and workspace information beyond the User identity.
// Optional values remain zero when upstream does not provide them.
type UserDetail struct {
	User             User
	Profile          UserProfile
	ProfilePublicity UserProfilePublicity
	Workspace        UserWorkspace
}

// UserProfile carries a user's public profile counters and links.
type UserProfile struct {
	Webpage                   string
	Gender                    string
	BirthDay                  string
	BirthYear                 int
	Region                    string
	CountryCode               string
	Job                       string
	TotalFollowUsers          int
	TotalMyPixivUsers         int
	TotalIllusts              int
	TotalManga                int
	TotalNovels               int
	TotalIllustBookmarks      int
	TotalIllustSeries         int
	TotalNovelSeries          int
	BackgroundImageURL        string
	TwitterAccount            string
	TwitterURL                string
	PawooURL                  string
	IsPremium                 bool
	IsUsingCustomProfileImage bool
}

// UserProfilePublicity records which profile fields are publicly visible.
type UserProfilePublicity struct {
	Gender    bool
	Region    bool
	BirthDay  bool
	BirthYear bool
	Job       bool
	Pawoo     bool
}

// UserWorkspace describes a user's declared workspace and tools.
type UserWorkspace struct {
	PC                string
	Monitor           string
	Tool              string
	Scanner           string
	Tablet            string
	Mouse             string
	Printer           string
	Desktop           string
	Music             string
	Desk              string
	Chair             string
	Comment           string
	WorkspaceImageURL string
}

// UserPreview couples a user with sample works used by recommendation and
// follower list operations.
type UserPreview struct {
	User    User
	Illusts []Artwork
	Novels  []Novel
}

// Comment is a single comment on an artwork or novel.
type Comment struct {
	ID            int64
	User          User
	Comment       string
	CreatedAt     time.Time
	ParentComment *Comment
}

// CommentAccessControl records the upstream access-control state for a comment
// page when upstream provided it.
type CommentAccessControl struct {
	CanComment bool
	IsLocked   bool
}

// CommentPage wraps a paged list of comments together with optional upstream
// total and access-control metadata. Total and AccessControl are non-nil only
// when upstream explicitly provided them; unknown values are never faked to
// zero.
type CommentPage struct {
	Page          sdk.Page[Comment]
	Total         *int64
	AccessControl *CommentAccessControl
}

// NovelSeries is an addressable novel series.
type NovelSeries struct {
	ID          int64
	Title       string
	Caption     string
	User        User
	IsConcluded bool
}

// NovelSeriesResult carries a novel series with its paged novels.
type NovelSeriesResult struct {
	Series NovelSeries
	Novels sdk.Page[Novel]
}

// BookmarkTag is one tag used by a user's artwork bookmarks. Count is the
// number of bookmarks carrying the tag when upstream provided it, otherwise
// zero.
type BookmarkTag struct {
	Name  string
	Count int
}

// TrendingTag is a currently trending artwork tag paired with a sample artwork.
type TrendingTag struct {
	Tag            string
	TranslatedName string
	Artwork        Artwork
}

// ArtworkBookmarkDetail is the current user's bookmark state for one artwork.
// A zero-value Restrict means the artwork is not bookmarked.
type ArtworkBookmarkDetail struct {
	Restrict Restrict
	Tags     []string
}
