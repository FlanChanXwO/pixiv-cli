package pixiv_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	pixivservice "github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	sdkpixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

type fakeAccounts struct {
	openedIDs    []int64
	options      []sdkpixiv.Options
	openedClient *sdkpixiv.Client
	err          error
	panic        bool
}

func (f *fakeAccounts) OpenClientWith(_ context.Context, options sdkpixiv.Options) (*sdkpixiv.Client, error) {
	f.openedIDs = append(f.openedIDs, 0)
	f.options = append(f.options, options)
	return f.client()
}

func (f *fakeAccounts) OpenAccountClientWith(_ context.Context, userID int64, options sdkpixiv.Options) (*sdkpixiv.Client, error) {
	f.openedIDs = append(f.openedIDs, userID)
	f.options = append(f.options, options)
	return f.client()
}

func (f *fakeAccounts) client() (*sdkpixiv.Client, error) {
	if f.panic {
		panic("fake account opener panic")
	}
	client := f.openedClient
	if client == nil {
		var err error
		client, err = sdkpixiv.New("test-access-token")
		if err != nil {
			return nil, err
		}
	}
	if f.err != nil {
		return client, f.err
	}
	return client, nil
}

type fakeGate struct {
	acquired int
	released int
	err      error
}

func (g *fakeGate) Acquire(context.Context) error {
	if g.err != nil {
		return g.err
	}
	g.acquired++
	return nil
}

func (g *fakeGate) Release() { g.released++ }

type fakePool struct {
	userIDs []int64
	calls   int
}

func (p *fakePool) Run(ctx context.Context, attempt func(context.Context, int64, *lifecycle.Attempt) error) error {
	for _, userID := range p.userIDs {
		state := &lifecycle.Attempt{}
		err := attempt(ctx, userID, state)
		p.calls++
		if err == nil || state.Committed() {
			return err
		}
	}
	return errors.New("fake pool exhausted")
}

func TestFacadeOpenLeaseClosesClientAndGateOnce(t *testing.T) {
	accounts := &fakeAccounts{}
	gate := &fakeGate{}
	closeCount := 0
	facade := pixivservice.New(pixivservice.Dependencies{
		Accounts: accounts,
		Gate:     gate,
		CloseClient: func(*sdkpixiv.Client) error {
			closeCount++
			return nil
		},
	})
	options := sdkpixiv.Options{AcceptLanguage: "ja-JP"}

	lease, err := facade.Open(context.Background(), pixivservice.Request{UserID: 42, Options: options})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if lease == nil || lease.Value() == nil {
		t.Fatal("Open() returned an empty lease")
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if got, want := gate.acquired, 1; got != want {
		t.Fatalf("gate acquired %d times, want %d", got, want)
	}
	if got, want := gate.released, 1; got != want {
		t.Fatalf("gate released %d times, want %d", got, want)
	}
	if got, want := closeCount, 1; got != want {
		t.Fatalf("client closed %d times, want %d", got, want)
	}
	if !reflect.DeepEqual(accounts.options, []sdkpixiv.Options{options}) {
		t.Fatalf("SDK options = %#v, want %#v", accounts.options, []sdkpixiv.Options{options})
	}
}

func TestFacadeUseReplaysUncommittedAttemptsAndClosesEachLease(t *testing.T) {
	accounts := &fakeAccounts{}
	gate := &fakeGate{}
	pool := &fakePool{userIDs: []int64{11, 22}}
	var gotConfig pixivservice.PoolConfig
	facade := pixivservice.New(pixivservice.Dependencies{
		Accounts: accounts,
		Gate:     gate,
		Pool: func(config pixivservice.PoolConfig) (pixivservice.PoolExecutor, error) {
			gotConfig = config
			return pool, nil
		},
		LoadPoolConfig: func() (pixivservice.PoolConfig, error) {
			return pixivservice.PoolConfig{Enabled: true, Strategy: "round-robin"}, nil
		},
	})
	options := sdkpixiv.Options{AcceptLanguage: "en-US"}
	callbackCalls := 0

	err := facade.Use(context.Background(), pixivservice.Request{Options: options}, func(_ context.Context, _ *sdkpixiv.Client) (bool, error) {
		callbackCalls++
		if callbackCalls == 1 {
			return false, sdk.NewError("pixiv", "test", sdk.RateLimited, sdk.WithRetry(sdk.RetryAdvice{
				Safe: true, After: time.Now().Add(time.Second), HasAfter: true,
			}))
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("Use() error = %v", err)
	}
	if got, want := callbackCalls, 2; got != want {
		t.Fatalf("callback calls = %d, want %d", got, want)
	}
	if got, want := pool.calls, 2; got != want {
		t.Fatalf("pool attempts = %d, want %d", got, want)
	}
	if !reflect.DeepEqual(accounts.openedIDs, []int64{11, 22}) {
		t.Fatalf("opened UIDs = %#v, want %#v", accounts.openedIDs, []int64{11, 22})
	}
	if got, want := gate.acquired, 2; got != want {
		t.Fatalf("gate acquired %d times, want %d", got, want)
	}
	if got, want := gate.released, 2; got != want {
		t.Fatalf("gate released %d times, want %d", got, want)
	}
	if !reflect.DeepEqual(accounts.options, []sdkpixiv.Options{options, options}) {
		t.Fatalf("SDK options = %#v, want %#v", accounts.options, []sdkpixiv.Options{options, options})
	}
	if got, want := gotConfig, (pixivservice.PoolConfig{Enabled: true, Strategy: "round-robin"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("pool config = %#v, want %#v", got, want)
	}
}

func TestFacadeUseBuildsPoolFromCurrentConfig(t *testing.T) {
	accounts := &fakeAccounts{}
	gate := &fakeGate{}
	pool := &fakePool{userIDs: []int64{11}}
	configs := []pixivservice.PoolConfig{
		{Enabled: true, Strategy: "round-robin"},
		{Enabled: true, Strategy: "random"},
	}
	var loaded int
	var built []pixivservice.PoolConfig
	facade := pixivservice.New(pixivservice.Dependencies{
		Accounts: accounts,
		Gate:     gate,
		Pool: func(config pixivservice.PoolConfig) (pixivservice.PoolExecutor, error) {
			built = append(built, config)
			return pool, nil
		},
		LoadPoolConfig: func() (pixivservice.PoolConfig, error) {
			config := configs[loaded]
			loaded++
			return config, nil
		},
	})

	for range configs {
		err := facade.Use(context.Background(), pixivservice.Request{}, func(context.Context, *sdkpixiv.Client) (bool, error) {
			return false, nil
		})
		require.NoError(t, err)
	}

	require.Equal(t, configs, built)
	require.Equal(t, 2, loaded)
}

func TestFacadeUseDoesNotReplayCommittedAttempt(t *testing.T) {
	accounts := &fakeAccounts{}
	gate := &fakeGate{}
	pool := &fakePool{userIDs: []int64{11, 22}}
	facade := pixivservice.New(pixivservice.Dependencies{
		Accounts: accounts,
		Gate:     gate,
		Pool: func(pixivservice.PoolConfig) (pixivservice.PoolExecutor, error) {
			return pool, nil
		},
		LoadPoolConfig: func() (pixivservice.PoolConfig, error) {
			return pixivservice.PoolConfig{Enabled: true}, nil
		},
	})
	wantErr := errors.New("committed callback failed")

	err := facade.Use(context.Background(), pixivservice.Request{}, func(_ context.Context, _ *sdkpixiv.Client) (bool, error) {
		return true, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Use() error = %v, want %v", err, wantErr)
	}
	if got, want := pool.calls, 1; got != want {
		t.Fatalf("pool attempts = %d, want %d", got, want)
	}
	if !reflect.DeepEqual(accounts.openedIDs, []int64{11}) {
		t.Fatalf("opened UIDs = %#v, want %#v", accounts.openedIDs, []int64{11})
	}
	if got, want := gate.released, 1; got != want {
		t.Fatalf("gate released %d times, want %d", got, want)
	}
}

func TestFacadeOpenReleasesGateWhenOpenerPanics(t *testing.T) {
	accounts := &fakeAccounts{panic: true}
	gate := &fakeGate{}
	facade := pixivservice.New(pixivservice.Dependencies{Accounts: accounts, Gate: gate})

	require.Panics(t, func() {
		_, _ = facade.Open(context.Background(), pixivservice.Request{})
	})
	require.Equal(t, 1, gate.acquired)
	require.Equal(t, 1, gate.released)
}

func TestFacadeOpenClosesPartialClientOnOpenError(t *testing.T) {
	partial, err := sdkpixiv.New("partial-token")
	require.NoError(t, err)
	openErr := errors.New("open failed")
	closeErr := errors.New("close failed")
	accounts := &fakeAccounts{openedClient: partial, err: openErr}
	gate := &fakeGate{}
	facade := pixivservice.New(pixivservice.Dependencies{
		Accounts: accounts,
		Gate:     gate,
		CloseClient: func(client *sdkpixiv.Client) error {
			require.Same(t, partial, client)
			return closeErr
		},
	})

	_, gotErr := facade.Open(context.Background(), pixivservice.Request{})
	require.ErrorIs(t, gotErr, openErr)
	require.ErrorIs(t, gotErr, closeErr)
	require.Equal(t, 1, gate.released)
}

func TestFacadeUseJoinsCallbackAndCloseErrors(t *testing.T) {
	callbackErr := errors.New("callback failed")
	closeErr := errors.New("close failed")
	gate := &fakeGate{}
	facade := pixivservice.New(pixivservice.Dependencies{
		Accounts: &fakeAccounts{},
		Gate:     gate,
		CloseClient: func(*sdkpixiv.Client) error {
			return closeErr
		},
		LoadPoolConfig: func() (pixivservice.PoolConfig, error) {
			return pixivservice.PoolConfig{}, nil
		},
	})

	err := facade.Use(context.Background(), pixivservice.Request{}, func(context.Context, *sdkpixiv.Client) (bool, error) {
		return false, callbackErr
	})

	require.ErrorIs(t, err, callbackErr)
	require.ErrorIs(t, err, closeErr)
	require.Equal(t, 1, gate.released)
}

func TestFacadeLeaseClosePanicReleasesGate(t *testing.T) {
	gate := &fakeGate{}
	facade := pixivservice.New(pixivservice.Dependencies{
		Accounts: &fakeAccounts{},
		Gate:     gate,
		CloseClient: func(*sdkpixiv.Client) error {
			panic("close panic")
		},
	})

	lease, err := facade.Open(context.Background(), pixivservice.Request{})
	require.NoError(t, err)
	require.Panics(t, func() { _ = lease.Close() })
	require.Equal(t, 1, gate.released)
}

func TestFacadeUseReportsMissingPoolConfiguration(t *testing.T) {
	facade := pixivservice.New(pixivservice.Dependencies{})
	err := facade.Use(context.Background(), pixivservice.Request{}, func(context.Context, *sdkpixiv.Client) (bool, error) {
		return false, nil
	})
	if !errors.Is(err, pixivservice.ErrPoolRuntimeLoaderNotConfigured) {
		t.Fatalf("Use() error = %v, want %v", err, pixivservice.ErrPoolRuntimeLoaderNotConfigured)
	}
}
