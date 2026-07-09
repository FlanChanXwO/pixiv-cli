package pixiv

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourceUsesWebOnlyWhenTokenIsEmptyAndFallbackEnabled(t *testing.T) {
	app := &fakeAppAPI{searchErr: errors.New("app should not be called")}
	web := &fakeWebAPI{search: &IllustList{Illusts: []Illust{{ID: 101}}}}
	source := NewSourceFromClients(app, web, SourcePolicy{WebFallbackEnabled: true})

	result, err := source.SearchIllust(context.Background(), "miku", "partial_match_for_tags", "date_desc", "", 0)

	require.NoError(t, err)
	require.Len(t, result.Illusts, 1)
	assert.Equal(t, int64(101), result.Illusts[0].ID)
	assert.Equal(t, 0, app.searchCalls)
	assert.Equal(t, 1, web.searchCalls)
}

func TestSourceKeepsAppErrorsWhenTokenExists(t *testing.T) {
	appErr := errors.New("invalid refresh token")
	app := &fakeAppAPI{refreshToken: "token", searchErr: appErr}
	web := &fakeWebAPI{search: &IllustList{Illusts: []Illust{{ID: 202}}}}
	source := NewSourceFromClients(app, web, SourcePolicy{RefreshToken: "token", WebFallbackEnabled: true})

	_, err := source.SearchIllust(context.Background(), "miku", "partial_match_for_tags", "date_desc", "", 0)

	require.ErrorIs(t, err, appErr)
	assert.Equal(t, 1, app.searchCalls)
	assert.Equal(t, 0, web.searchCalls)
}

func TestSourceDoesNotUseWebWhenFallbackDisabled(t *testing.T) {
	app := &fakeAppAPI{search: &IllustList{Illusts: []Illust{{ID: 303}}}}
	web := &fakeWebAPI{search: &IllustList{Illusts: []Illust{{ID: 404}}}}
	source := NewSourceFromClients(app, web, SourcePolicy{WebFallbackEnabled: false})

	result, err := source.SearchIllust(context.Background(), "miku", "partial_match_for_tags", "date_desc", "", 0)

	require.NoError(t, err)
	require.Len(t, result.Illusts, 1)
	assert.Equal(t, int64(303), result.Illusts[0].ID)
	assert.Equal(t, 1, app.searchCalls)
	assert.Equal(t, 0, web.searchCalls)
}

func TestSourceSetRefreshTokenDisablesWebFallback(t *testing.T) {
	app := &fakeAppAPI{search: &IllustList{Illusts: []Illust{{ID: 505}}}}
	web := &fakeWebAPI{search: &IllustList{Illusts: []Illust{{ID: 606}}}}
	source := NewSourceFromClients(app, web, SourcePolicy{WebFallbackEnabled: true})

	source.SetRefreshToken("new-token")
	result, err := source.SearchIllust(context.Background(), "miku", "partial_match_for_tags", "date_desc", "", 0)

	require.NoError(t, err)
	assert.Equal(t, "new-token", app.refreshToken)
	require.Len(t, result.Illusts, 1)
	assert.Equal(t, int64(505), result.Illusts[0].ID)
	assert.Equal(t, 1, app.searchCalls)
	assert.Equal(t, 0, web.searchCalls)
}

func TestSourceRoutesDownloadAndUgoiraThroughWebFallback(t *testing.T) {
	app := &fakeAppAPI{}
	web := &fakeWebAPI{
		ugoira: &UgoiraMetadataResult{UgoiraMetadata: UgoiraMetadata{Frames: []UgoiraFrame{{File: "0.jpg", Delay: 60}}}},
	}
	source := NewSourceFromClients(app, web, SourcePolicy{WebFallbackEnabled: true})

	meta, err := source.UgoiraMetadata(context.Background(), 707)
	require.NoError(t, err)
	require.Len(t, meta.UgoiraMetadata.Frames, 1)

	var dst bytes.Buffer
	require.NoError(t, source.Download(context.Background(), "https://i.pximg.net/x.jpg", &dst))
	assert.Equal(t, "web-download", dst.String())
	assert.Equal(t, 1, web.ugoiraCalls)
	assert.Equal(t, 1, web.downloadCalls)
	assert.Equal(t, 0, app.ugoiraCalls)
	assert.Equal(t, 0, app.downloadCalls)
}

type fakeAppAPI struct {
	refreshToken  string
	search        *IllustList
	searchErr     error
	searchCalls   int
	ugoiraCalls   int
	downloadCalls int
}

func (f *fakeAppAPI) Refresh(context.Context) error { return nil }
func (f *fakeAppAPI) SetRefreshToken(token string)  { f.refreshToken = token }
func (f *fakeAppAPI) RefreshTokenValue() string     { return f.refreshToken }
func (f *fakeAppAPI) UserID() int64                 { return 0 }
func (f *fakeAppAPI) UserName() string              { return "" }
func (f *fakeAppAPI) IsAuthenticated() bool         { return false }
func (f *fakeAppAPI) SearchIllust(context.Context, string, string, string, string, int) (*IllustList, error) {
	f.searchCalls++
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.search, nil
}
func (f *fakeAppAPI) IllustDetail(context.Context, int64) (*IllustDetail, error) {
	return nil, nil
}
func (f *fakeAppAPI) IllustRelated(context.Context, int64, int) (*IllustList, error) {
	return nil, nil
}
func (f *fakeAppAPI) IllustRanking(context.Context, string, string, int) (*IllustList, error) {
	return nil, nil
}
func (f *fakeAppAPI) SearchUser(context.Context, string, int) (*UserPreviewList, error) {
	return nil, nil
}
func (f *fakeAppAPI) UserDetail(context.Context, int64) (*User, error) {
	return nil, nil
}
func (f *fakeAppAPI) IllustRecommended(context.Context, int) (*IllustList, error) {
	return nil, nil
}
func (f *fakeAppAPI) TrendingTagsIllust(context.Context) (*TrendTags, error) { return nil, nil }
func (f *fakeAppAPI) IllustFollow(context.Context, string, int) (*IllustList, error) {
	return nil, nil
}
func (f *fakeAppAPI) UserBookmarks(context.Context, int64, string, string, int64) (*IllustList, error) {
	return nil, nil
}
func (f *fakeAppAPI) UserFollowing(context.Context, int64, string, int) (*UserPreviewList, error) {
	return nil, nil
}
func (f *fakeAppAPI) UgoiraMetadata(context.Context, int64) (*UgoiraMetadataResult, error) {
	f.ugoiraCalls++
	return nil, nil
}
func (f *fakeAppAPI) Download(context.Context, string, io.Writer) error {
	f.downloadCalls++
	return nil
}

type fakeWebAPI struct {
	search        *IllustList
	searchCalls   int
	ugoira        *UgoiraMetadataResult
	ugoiraCalls   int
	downloadCalls int
}

func (f *fakeWebAPI) SearchIllust(context.Context, string, string, string, string, int) (*IllustList, error) {
	f.searchCalls++
	return f.search, nil
}
func (f *fakeWebAPI) IllustDetail(context.Context, int64) (*IllustDetail, error) {
	return nil, nil
}
func (f *fakeWebAPI) IllustRanking(context.Context, string, string, int) (*IllustList, error) {
	return nil, nil
}
func (f *fakeWebAPI) SearchUser(context.Context, string, int) (*UserPreviewList, error) {
	return nil, nil
}
func (f *fakeWebAPI) UgoiraMetadata(context.Context, int64) (*UgoiraMetadataResult, error) {
	f.ugoiraCalls++
	return f.ugoira, nil
}
func (f *fakeWebAPI) Download(_ context.Context, _ string, dst io.Writer) error {
	f.downloadCalls++
	_, err := io.WriteString(dst, "web-download")
	return err
}
