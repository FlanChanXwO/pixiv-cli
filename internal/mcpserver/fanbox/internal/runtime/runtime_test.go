package runtime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
	fanbox "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

func TestOpenClientUsesInjectedSDKPortAndAccount(t *testing.T) {
	proxy := "http://proxy.example"
	var got Account
	wantErr := errors.New("sdk unavailable")
	app := NewApp(SDKPorts{Open: func(_ context.Context, account Account) (*fanbox.Client, error) {
		got = account
		return nil, wantErr
	}}, Account{HTTPSProxyOverride: &proxy})
	_, err := app.OpenClient(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected SDK error, got %v", err)
	}
	if got.HTTPSProxyOverride == nil || *got.HTTPSProxyOverride != proxy {
		t.Fatalf("SDK port received account %#v", got)
	}
}

func TestOpenClientRejectsUnconfiguredPort(t *testing.T) {
	_, err := NewApp(SDKPorts{}, Account{}).OpenClient(context.Background())
	if err == nil {
		t.Fatal("expected configuration error")
	}
}

func TestOpenClientReturnsLeaseAndForwardsCancellation(t *testing.T) {
	var opens atomic.Int32
	var closes atomic.Int32
	app := NewApp(SDKPorts{OpenLease: func(ctx context.Context, _ Account) (*lifecycle.Lease[*fanbox.Client], error) {
		opens.Add(1)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return lifecycle.NewLease((*fanbox.Client)(nil), func() error {
			closes.Add(1)
			return nil
		}), nil
	}}, Account{})
	lease, err := app.OpenClient(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := app.OpenClient(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled open error=%v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("close lease: %v", err)
	}
	if opens.Load() != 2 || closes.Load() != 1 {
		t.Fatalf("opens=%d closes=%d, want 2/1", opens.Load(), closes.Load())
	}
}
