package pixiv

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/appapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourceUsesWebOnlyWhenTokenIsEmptyAndFallbackEnabled(t *testing.T) {
	app := &fakeAppAPI{searchErr: errors.New("app should not be called")}
	web := &fakeWebAPI{search: &IllustList{Illusts: []Illust{{ID: 101}}}}
	source := newFakeSource(app, web, &fakeAuth{}, true)

	result, err := source.SearchIllust(context.Background(), "miku", "partial_match_for_tags", "date_desc", "", 0)

	require.NoError(t, err)
	require.Len(t, result.Illusts, 1)
	assert.Equal(t, int64(101), result.Illusts[0].ID)
	assert.Equal(t, 0, app.searchCalls)
	assert.Equal(t, 1, web.searchCalls)
}

func TestSourceKeepsAppErrorsWhenTokenExists(t *testing.T) {
	appErr := errors.New("invalid refresh token")
	app := &fakeAppAPI{searchErr: appErr}
	web := &fakeWebAPI{search: &IllustList{Illusts: []Illust{{ID: 202}}}}
	source := newFakeSource(app, web, &fakeAuth{refreshToken: "token"}, true)

	_, err := source.SearchIllust(context.Background(), "miku", "partial_match_for_tags", "date_desc", "", 0)

	require.ErrorIs(t, err, appErr)
	assert.Equal(t, 1, app.searchCalls)
	assert.Equal(t, 0, web.searchCalls)
}

func TestSourceDoesNotUseWebWhenFallbackDisabled(t *testing.T) {
	app := &fakeAppAPI{search: &IllustList{Illusts: []Illust{{ID: 303}}}}
	web := &fakeWebAPI{search: &IllustList{Illusts: []Illust{{ID: 404}}}}
	source := newFakeSource(app, web, &fakeAuth{}, false)

	result, err := source.SearchIllust(context.Background(), "miku", "partial_match_for_tags", "date_desc", "", 0)

	require.NoError(t, err)
	require.Len(t, result.Illusts, 1)
	assert.Equal(t, int64(303), result.Illusts[0].ID)
	assert.Equal(t, 1, app.searchCalls)
	assert.Equal(t, 0, web.searchCalls)
}

func TestSourceKeepsAppOnlyOperationOutOfWebWhitelist(t *testing.T) {
	app := &fakeAppAPI{related: &IllustList{Illusts: []Illust{{ID: 808}}}}
	source := newFakeSource(app, &fakeWebAPI{}, &fakeAuth{}, true)

	result, err := source.IllustRelated(context.Background(), 1, 0)

	require.NoError(t, err)
	require.Len(t, result.Illusts, 1)
	assert.Equal(t, int64(808), result.Illusts[0].ID)
	assert.Equal(t, 1, app.relatedCalls)
}

func TestSourceUserDetailRetainsLegacyUserContract(t *testing.T) {
	app := &fakeAppAPI{userDetail: &UserDetail{User: User{ID: 42, Name: "alice"}, Profile: Profile{Gender: "female"}}}
	source := newFakeSource(app, &fakeWebAPI{}, &fakeAuth{}, true)

	user, err := source.UserDetail(context.Background(), 42)

	require.NoError(t, err)
	assert.Equal(t, &User{ID: 42, Name: "alice"}, user)
}

func TestSourceUserDetailRejectsNilDetailAsMalformed(t *testing.T) {
	source := newFakeSource(&fakeAppAPI{}, &fakeWebAPI{}, &fakeAuth{}, true)

	user, err := source.UserDetail(context.Background(), 42)

	assert.Nil(t, user)
	assert.ErrorIs(t, err, appapi.ErrMalformedResponse)
}

func TestSourceSetRefreshTokenDisablesWebFallback(t *testing.T) {
	app := &fakeAppAPI{search: &IllustList{Illusts: []Illust{{ID: 505}}}}
	web := &fakeWebAPI{search: &IllustList{Illusts: []Illust{{ID: 606}}}}
	auth := &fakeAuth{}
	source := newFakeSource(app, web, auth, true)

	source.SetRefreshToken("new-token")
	result, err := source.SearchIllust(context.Background(), "miku", "partial_match_for_tags", "date_desc", "", 0)

	require.NoError(t, err)
	assert.Equal(t, "new-token", auth.refreshToken)
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
	appResource, webResource := &fakeResource{}, &fakeResource{payload: "web-download"}
	source := NewSourceFromClients(app, web, &fakeAuth{}, appResource, webResource, SourcePolicy{WebFallbackEnabled: true})

	meta, err := source.UgoiraMetadata(context.Background(), 707)
	require.NoError(t, err)
	require.Len(t, meta.UgoiraMetadata.Frames, 1)

	var dst bytes.Buffer
	require.NoError(t, source.Download(context.Background(), "https://i.pximg.net/x.jpg", &dst))
	assert.Equal(t, "web-download", dst.String())
	assert.Equal(t, 1, web.ugoiraCalls)
	assert.Equal(t, 1, webResource.calls)
	assert.Equal(t, 0, app.ugoiraCalls)
	assert.Equal(t, 0, appResource.calls)
}

func newFakeSource(app AppAPI, web WebAPI, auth *fakeAuth, enabled bool) *Source {
	return NewSourceFromClients(app, web, auth, &fakeResource{}, &fakeResource{}, SourcePolicy{RefreshToken: auth.refreshToken, WebFallbackEnabled: enabled})
}

type fakeAuth struct{ refreshToken string }

func (*fakeAuth) Refresh(context.Context) error  { return nil }
func (f *fakeAuth) SetRefreshToken(token string) { f.refreshToken = token }
func (f *fakeAuth) RefreshTokenValue() string    { return f.refreshToken }
func (*fakeAuth) UserID() int64                  { return 0 }
func (*fakeAuth) UserName() string               { return "" }
func (*fakeAuth) IsAuthenticated() bool          { return false }

type fakeResource struct {
	payload string
	calls   int
}

func (f *fakeResource) Download(_ context.Context, _ string, dst io.Writer) error {
	f.calls++
	_, err := io.WriteString(dst, f.payload)
	return err
}

type fakeAppAPI struct {
	search       *IllustList
	searchErr    error
	searchCalls  int
	related      *IllustList
	relatedCalls int
	ugoiraCalls  int
	userDetail   *UserDetail
}

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
	f.relatedCalls++
	return f.related, nil
}
func (f *fakeAppAPI) IllustRanking(context.Context, string, string, int) (*IllustList, error) {
	return nil, nil
}
func (f *fakeAppAPI) SearchUser(context.Context, string, int) (*UserPreviewList, error) {
	return nil, nil
}
func (f *fakeAppAPI) UserDetail(context.Context, int64) (*UserDetail, error) {
	return f.userDetail, nil
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

type fakeWebAPI struct {
	search      *IllustList
	searchCalls int
	ugoira      *UgoiraMetadataResult
	ugoiraCalls int
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
