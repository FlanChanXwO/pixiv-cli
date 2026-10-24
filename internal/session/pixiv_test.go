package session

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
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

func (f *fakeAccounts) OpenClient(_ context.Context, options sdkpixiv.Options) (*sdkpixiv.Client, error) {
	f.openedIDs = append(f.openedIDs, 0)
	f.options = append(f.options, options)
	return f.client()
}

func (f *fakeAccounts) OpenAccountClient(_ context.Context, userID int64, options sdkpixiv.Options) (*sdkpixiv.Client, error) {
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

func newTestPixiv(accounts AccountService, gate RotationGate, loadPoolConfig ConfigLoader, pool PoolFactory, closeClient func(*sdkpixiv.Client) error) *Pixiv {
	f := NewPixiv(accounts, gate, loadPoolConfig, pool)
	if closeClient != nil {
		f.closeClient = closeClient
	}
	return f
}

func TestPixivOpenLeaseClosesClientAndGateOnce(t *testing.T) {
	accounts := &fakeAccounts{}
	gate := &fakeGate{}
	closeCount := 0
	facade := newTestPixiv(accounts, gate, nil, nil, func(*sdkpixiv.Client) error {
		closeCount++
		return nil
	})
	options := sdkpixiv.Options{AcceptLanguage: "ja-JP"}

	lease, err := facade.Open(context.Background(), Request{UserID: 42, Options: options})
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

func TestPixivUseReplaysUncommittedAttemptsAndClosesEachLease(t *testing.T) {
	accounts := &fakeAccounts{}
	gate := &fakeGate{}
	pool := &fakePool{userIDs: []int64{11, 22}}
	var gotConfig PoolConfig
	facade := newTestPixiv(accounts, gate, func() (PoolConfig, error) {
		return PoolConfig{Enabled: true, Strategy: "round-robin"}, nil
	}, func(config PoolConfig) (PoolExecutor, error) {
		gotConfig = config
		return pool, nil
	}, nil)
	options := sdkpixiv.Options{AcceptLanguage: "en-US"}
	callbackCalls := 0

	err := facade.Use(context.Background(), Request{Options: options}, func(_ context.Context, _ *sdkpixiv.Client) (bool, error) {
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
	if got, want := gotConfig, (PoolConfig{Enabled: true, Strategy: "round-robin"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("pool config = %#v, want %#v", got, want)
	}
}

func TestPixivUseBuildsPoolFromCurrentConfig(t *testing.T) {
	accounts := &fakeAccounts{}
	gate := &fakeGate{}
	pool := &fakePool{userIDs: []int64{11}}
	configs := []PoolConfig{
		{Enabled: true, Strategy: "round-robin"},
		{Enabled: true, Strategy: "random"},
	}
	var loaded int
	var built []PoolConfig
	facade := newTestPixiv(accounts, gate, func() (PoolConfig, error) {
		config := configs[loaded]
		loaded++
		return config, nil
	}, func(config PoolConfig) (PoolExecutor, error) {
		built = append(built, config)
		return pool, nil
	}, nil)

	for range configs {
		err := facade.Use(context.Background(), Request{}, func(context.Context, *sdkpixiv.Client) (bool, error) {
			return false, nil
		})
		require.NoError(t, err)
	}

	require.Equal(t, configs, built)
	require.Equal(t, 2, loaded)
}

func TestPixivUseDoesNotReplayCommittedAttempt(t *testing.T) {
	accounts := &fakeAccounts{}
	gate := &fakeGate{}
	pool := &fakePool{userIDs: []int64{11, 22}}
	facade := newTestPixiv(accounts, gate, func() (PoolConfig, error) {
		return PoolConfig{Enabled: true}, nil
	}, func(PoolConfig) (PoolExecutor, error) {
		return pool, nil
	}, nil)
	wantErr := errors.New("committed callback failed")

	err := facade.Use(context.Background(), Request{}, func(_ context.Context, _ *sdkpixiv.Client) (bool, error) {
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

func TestPixivOpenReleasesGateWhenOpenerPanics(t *testing.T) {
	accounts := &fakeAccounts{panic: true}
	gate := &fakeGate{}
	facade := newTestPixiv(accounts, gate, nil, nil, nil)

	require.Panics(t, func() {
		_, _ = facade.Open(context.Background(), Request{})
	})
	require.Equal(t, 1, gate.acquired)
	require.Equal(t, 1, gate.released)
}

func TestPixivOpenClosesPartialClientOnOpenError(t *testing.T) {
	partial, err := sdkpixiv.New("partial-token")
	require.NoError(t, err)
	openErr := errors.New("open failed")
	closeErr := errors.New("close failed")
	accounts := &fakeAccounts{openedClient: partial, err: openErr}
	gate := &fakeGate{}
	facade := newTestPixiv(accounts, gate, nil, nil, func(client *sdkpixiv.Client) error {
		require.Same(t, partial, client)
		return closeErr
	})

	_, gotErr := facade.Open(context.Background(), Request{})
	require.ErrorIs(t, gotErr, openErr)
	require.ErrorIs(t, gotErr, closeErr)
	require.Equal(t, 1, gate.released)
}

func TestPixivUseJoinsCallbackAndCloseErrors(t *testing.T) {
	callbackErr := errors.New("callback failed")
	closeErr := errors.New("close failed")
	gate := &fakeGate{}
	facade := newTestPixiv(&fakeAccounts{}, gate, func() (PoolConfig, error) {
		return PoolConfig{}, nil
	}, nil, func(*sdkpixiv.Client) error {
		return closeErr
	})

	err := facade.Use(context.Background(), Request{}, func(context.Context, *sdkpixiv.Client) (bool, error) {
		return false, callbackErr
	})

	require.ErrorIs(t, err, callbackErr)
	require.ErrorIs(t, err, closeErr)
	require.Equal(t, 1, gate.released)
}

func TestPixivLeaseClosePanicReleasesGate(t *testing.T) {
	gate := &fakeGate{}
	facade := newTestPixiv(&fakeAccounts{}, gate, nil, nil, func(*sdkpixiv.Client) error {
		panic("close panic")
	})

	lease, err := facade.Open(context.Background(), Request{})
	require.NoError(t, err)
	require.Panics(t, func() { _ = lease.Close() })
	require.Equal(t, 1, gate.released)
}

func TestPixivUseReportsMissingPoolConfiguration(t *testing.T) {
	facade := NewPixiv(nil, nil, nil, nil)
	err := facade.Use(context.Background(), Request{}, func(context.Context, *sdkpixiv.Client) (bool, error) {
		return false, nil
	})
	if !errors.Is(err, poolRuntimeLoaderNotConfigured) {
		t.Fatalf("Use() error = %v, want %v", err, poolRuntimeLoaderNotConfigured)
	}
}

func TestPixivExplicitAccountOverridesEnabledPool(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprint(committed), func(t *testing.T) {
			accounts := &fakeAccounts{}
			gate := &fakeGate{}
			pool := &fakePool{userIDs: []int64{11, 22}}
			facade := newTestPixiv(accounts, gate, func() (PoolConfig, error) { return PoolConfig{Enabled: true}, nil }, func(PoolConfig) (PoolExecutor, error) { return pool, nil }, nil)
			want := sdk.NewError("pixiv", "fixture", sdk.RateLimited, sdk.WithRetry(sdk.RetryAdvice{Safe: true}))
			calls := 0
			err := facade.Use(t.Context(), Request{UserID: 42}, func(context.Context, *sdkpixiv.Client) (bool, error) { calls++; return committed, want })
			require.ErrorIs(t, err, want)
			require.Equal(t, []int64{42}, accounts.openedIDs)
			require.Equal(t, 1, calls)
			require.Zero(t, pool.calls)
			require.Equal(t, 1, gate.acquired)
			require.Equal(t, 1, gate.released)
		})
	}
}

func TestPixivExplicitMissingAccountDoesNotTryPool(t *testing.T) {
	want := errors.New("account missing")
	accounts := &fakeAccounts{err: want}
	gate := &fakeGate{}
	facade := newTestPixiv(accounts, gate, func() (PoolConfig, error) { return PoolConfig{Enabled: true}, nil }, func(PoolConfig) (PoolExecutor, error) {
		t.Error("pool created for explicit account")
		return nil, nil
	}, nil)
	err := facade.Use(t.Context(), Request{UserID: 42}, func(context.Context, *sdkpixiv.Client) (bool, error) {
		t.Error("callback called for missing account")
		return false, nil
	})
	require.ErrorIs(t, err, want)
	require.Equal(t, []int64{42}, accounts.openedIDs)
	require.Equal(t, 1, gate.released)
}
