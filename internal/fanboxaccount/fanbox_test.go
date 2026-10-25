package fanboxaccount_test

import (
	"context"
	"errors"
	"fmt"
	fanbox "github.com/FlanChanXwO/pixiv-cli/internal/fanboxaccount"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	sdkfanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
)

func TestAccountSessionCopyAndSafeFormatting(t *testing.T) {
	input := []byte("fanbox-session-secret")
	account := fanbox.New(7, "creator", "cid", input)
	input[0] = 'X'
	copyValue := account.SessionIDCopy()
	copyValue[0] = 'Y'

	if string(account.SessionIDCopy()) != "fanbox-session-secret" {
		t.Fatal("session was not defensively copied")
	}
	for _, formatted := range []string{
		fmt.Sprintf("%v", account),
		fmt.Sprintf("%+v", account),
		fmt.Sprintf("%#v", account),
		fmt.Sprintf("%q", account),
		fmt.Sprintf("%x", account),
		fmt.Sprintf("%d", account),
	} {
		if strings.Contains(formatted, "fanbox-session-secret") || strings.Contains(formatted, "sessionID") {
			t.Fatalf("safe formatting leaked session: %s", formatted)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func sessionOKRoundTripper() http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/html"}},
			Body:       io.NopCloser(strings.NewReader(`<html><head><meta name="metadata" content='{"context":{"user":{"userId":42,"name":"tester"}}}'></head></html>`)),
		}, nil
	})
}

func sessionFailRoundTripper() http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
}

type fanboxTestDefaults struct {
	userID   int64
	ok       bool
	readErr  error
	setErr   error
	clearErr error
}

func (d *fanboxTestDefaults) ReadFanboxDefaultUserID() (int64, bool, error) {
	return d.userID, d.ok, d.readErr
}

func (d *fanboxTestDefaults) SetFanboxDefaultUserID(userID int64) error {
	if d.setErr != nil {
		return d.setErr
	}
	d.userID, d.ok = userID, true
	return nil
}

func (d *fanboxTestDefaults) ClearFanboxDefaultUserID() error {
	if d.clearErr != nil {
		return d.clearErr
	}
	d.userID, d.ok = 0, false
	return nil
}

type fanboxTestRepository struct {
	accounts map[int64]fanbox.Account
}

type contextCapturingFanboxRepository struct {
	*fanboxTestRepository
	listContext context.Context
}

func (r *contextCapturingFanboxRepository) ListFanbox(ctx context.Context) ([]fanbox.Account, error) {
	r.listContext = ctx
	return r.fanboxTestRepository.ListFanbox(ctx)
}

func newFanboxTestRepository() *fanboxTestRepository {
	return &fanboxTestRepository{accounts: make(map[int64]fanbox.Account)}
}

func (r *fanboxTestRepository) SaveFanboxCredential(_ context.Context, account fanbox.Account) error {
	if account.UserID <= 0 || !account.HasSession() || account.ValidatedAt <= 0 {
		return errors.New("invalid fanbox account")
	}
	if existing, ok := r.accounts[account.UserID]; ok {
		account.SortOrder = existing.SortOrder
		account.CredentialRevision = existing.CredentialRevision + 1
	} else {
		if account.SortOrder <= 0 {
			account.SortOrder = int64(len(r.accounts) + 1)
		}
		if account.CredentialRevision <= 0 {
			account.CredentialRevision = 1
		}
	}
	r.accounts[account.UserID] = cloneFanboxAccount(account)
	return nil
}

func (r *fanboxTestRepository) RotateFanboxSession(_ context.Context, userID, expectedRevision int64, session []byte, validatedAt int64) error {
	account, ok := r.accounts[userID]
	if !ok {
		return fanbox.ErrNotFound
	}
	if account.CredentialRevision != expectedRevision {
		return fanbox.ErrCredentialConflict
	}
	updated := fanbox.New(account.UserID, account.DisplayName, account.CreatorID, session)
	updated.SortOrder = account.SortOrder
	updated.CredentialRevision = account.CredentialRevision + 1
	updated.ValidatedAt = validatedAt
	r.accounts[userID] = updated
	return nil
}

func (r *fanboxTestRepository) ListFanbox(_ context.Context) ([]fanbox.Account, error) {
	accounts := make([]fanbox.Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		accounts = append(accounts, cloneFanboxAccount(account))
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].SortOrder < accounts[j].SortOrder })
	return accounts, nil
}

func (r *fanboxTestRepository) GetFanbox(_ context.Context, userID int64) (fanbox.Account, error) {
	account, ok := r.accounts[userID]
	if !ok {
		return fanbox.Account{}, fanbox.ErrNotFound
	}
	return cloneFanboxAccount(account), nil
}

func (r *fanboxTestRepository) RemoveFanbox(_ context.Context, userID int64) error {
	if _, ok := r.accounts[userID]; !ok {
		return fanbox.ErrNotFound
	}
	delete(r.accounts, userID)
	return nil
}

func cloneFanboxAccount(account fanbox.Account) fanbox.Account {
	cloned := fanbox.New(account.UserID, account.DisplayName, account.CreatorID, account.SessionIDCopy())
	cloned.SortOrder = account.SortOrder
	cloned.CredentialRevision = account.CredentialRevision
	cloned.ValidatedAt = account.ValidatedAt
	cloned.CreatedAt = account.CreatedAt
	cloned.UpdatedAt = account.UpdatedAt
	return cloned
}

func newFanboxTestService(repo fanbox.Repository, defaults fanbox.DefaultStore) *fanbox.Service {
	return fanbox.NewService(repo, defaults, nil)
}

// newFanboxNetworkService 用真实 SDK transport 构造服务，验证不拨号。
func newFanboxNetworkService(repo fanbox.Repository, defaults fanbox.DefaultStore, transport http.RoundTripper) *fanbox.Service {
	return fanbox.NewService(repo, defaults, func() (sdkfanbox.Options, error) {
		return sdkfanbox.Options{HTTPClient: &http.Client{Transport: transport}}, nil
	})
}

func TestImportSessionRejectsEmptyBeforeValidation(t *testing.T) {
	repo := newFanboxTestRepository()
	called := false
	service := newFanboxNetworkService(repo, &fanboxTestDefaults{}, roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("must not open an empty session")
	}))

	_, err := service.ImportSession(context.Background(), "  ", true, nil)
	require.EqualError(t, err, "FANBOX session value is required")
	require.False(t, called)
	require.Empty(t, repo.accounts)
}

func TestImportSessionValidationFailureCreatesNoRecord(t *testing.T) {
	repo := newFanboxTestRepository()
	service := newFanboxNetworkService(repo, &fanboxTestDefaults{}, sessionFailRoundTripper())

	_, err := service.ImportSession(context.Background(), "session-canary", true, nil)
	require.Error(t, err)
	require.Equal(t, sdk.CredentialsExpired, sdk.ReasonOf(err))
	require.Empty(t, repo.accounts)
}

func TestImportSessionHappyPathStoresAndSelectsDefault(t *testing.T) {
	repo := newFanboxTestRepository()
	defaults := &fanboxTestDefaults{}
	service := newFanboxNetworkService(repo, defaults, sessionOKRoundTripper())

	account, err := service.ImportSession(context.Background(), "session-canary", true, nil)
	require.NoError(t, err)
	require.Equal(t, int64(42), account.UserID)
	require.True(t, account.Default)

	stored, err := repo.GetFanbox(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, "session-canary", string(stored.SessionIDCopy()))
	require.Equal(t, int64(42), defaults.userID)
	require.True(t, defaults.ok)
}

func TestUseAutoAndUseAccountToggleConfigDefault(t *testing.T) {
	repo := newFanboxTestRepository()
	defaults := &fanboxTestDefaults{}
	service := newFanboxNetworkService(repo, defaults, sessionOKRoundTripper())
	_, err := service.ImportSession(context.Background(), "session-abc", false, nil)
	require.NoError(t, err)
	require.True(t, defaults.ok, "first imported account becomes default")

	require.NoError(t, service.UseAuto())
	require.False(t, defaults.ok)
	require.NoError(t, service.UseAccount(context.Background(), 42))
	require.True(t, defaults.ok)
	require.Equal(t, int64(42), defaults.userID)
}

func TestOpenClientUsesConfiguredTransport(t *testing.T) {
	repo := newFanboxTestRepository()
	account := fanbox.New(42, "creator", "", []byte("session"))
	account.SortOrder, account.ValidatedAt = 1, 1
	repo.accounts[42] = account
	service := newFanboxNetworkService(repo, &fanboxTestDefaults{}, sessionOKRoundTripper())

	client, err := service.OpenClient(t.Context(), nil)
	require.NoError(t, err)
	client.CloseIdleConnections()
}

func TestRemoveAccountRejectsExplicitDefaultBeforeMutation(t *testing.T) {
	repo := newFanboxTestRepository()
	first := fanbox.New(42, "first", "", []byte("session"))
	first.SortOrder, first.ValidatedAt = 1, 1
	repo.accounts[42] = first
	defaults := &fanboxTestDefaults{userID: 42, ok: true}
	service := newFanboxTestService(repo, defaults)

	err := service.RemoveAccount(context.Background(), 42)
	require.Error(t, err)
	require.Contains(t, err.Error(), "auth use --auto")
	_, getErr := repo.GetFanbox(context.Background(), 42)
	require.NoError(t, getErr)
	require.Equal(t, int64(42), defaults.userID)
}

func TestFanboxAutoDefaultMarksSmallestSortOrder(t *testing.T) {
	repo := newFanboxTestRepository()
	second := fanbox.New(42, "second", "", []byte("session-2"))
	second.SortOrder, second.ValidatedAt = 2, 1
	first := fanbox.New(7, "first", "", []byte("session-1"))
	first.SortOrder, first.ValidatedAt = 1, 1
	repo.accounts[42] = second
	repo.accounts[7] = first
	service := newFanboxTestService(repo, &fanboxTestDefaults{})

	accounts, err := service.ListAccounts(context.Background())
	require.NoError(t, err)
	require.Len(t, accounts, 2)
	require.True(t, accounts[0].Default)
	require.False(t, accounts[1].Default)
	require.Equal(t, int64(7), accounts[0].UserID)
}

func TestListAccountsUsesCallerContextForAutoDefault(t *testing.T) {
	base := newFanboxTestRepository()
	account := fanbox.New(42, "account", "", []byte("session"))
	account.SortOrder, account.ValidatedAt = 1, 1
	base.accounts[account.UserID] = account
	repo := &contextCapturingFanboxRepository{fanboxTestRepository: base}
	service := newFanboxTestService(repo, &fanboxTestDefaults{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := service.ListAccounts(ctx)
	require.NoError(t, err)
	require.ErrorIs(t, repo.listContext.Err(), context.Canceled)
}
