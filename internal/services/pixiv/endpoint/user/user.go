package user

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/artwork"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/endpoint/novel"
)

type ProfileImageURLs struct {
	Medium *string
}

// User 是 user endpoint 共享的 normalized identity；详情、列表和 mutation
// 均通过这个 parent 的稳定字段交付，不持有 transport。
type User struct {
	ID               int64
	Name             string
	Account          string
	Comment          string
	IsFollowed       bool
	ProfileImageURLs ProfileImageURLs
}

type Detail struct {
	User             User
	Profile          Profile
	ProfilePublicity ProfilePublicity
	Workspace        Workspace
}

type Profile struct {
	Webpage                    *string
	Gender                     string
	Birth                      string
	BirthDay                   string
	BirthYear                  int
	Region                     string
	AddressID                  int64
	CountryCode                string
	Job                        string
	JobID                      int64
	TotalFollowUsers           int
	TotalMyPixivUsers          int
	TotalIllusts               int
	TotalManga                 int
	TotalNovels                int
	TotalIllustBookmarksPublic int
	TotalIllustSeries          int
	TotalNovelSeries           int
	BackgroundImageURL         *string
	TwitterAccount             string
	TwitterURL                 *string
	PawooURL                   *string
	IsPremium                  bool
	IsUsingCustomProfileImage  bool
}

type ProfilePublicity struct {
	Gender    bool
	Region    bool
	BirthDay  bool
	BirthYear bool
	Job       bool
	Pawoo     bool
}

type Workspace struct {
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
	WorkspaceImageURL *string
}

// Preview is the common normalized shape for user search/follow lists and
// user recommendations. Most list endpoints only populate User; recommended
// users may include the nested artwork and novel samples.
type Preview struct {
	User    User
	Illusts []artwork.Artwork
	Novels  []novel.Novel
}
