package runtime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
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

// TestCollectPagesPreservesHasMoreWhenTruncatingInsideBatch 验证 finding #17 在 FANBOX
// 侧的等价修复：逻辑 limit 小于上游最后一批、且该批上游 cursor 已为空时，
// CollectPages 必须返回 has_more=true，而不是仅依据上游 cursor 推导出 false。
func TestCollectPagesPreservesHasMoreWhenTruncatingInsideBatch(t *testing.T) {
	plan := ListPlan{Page: 1, Limit: 2}
	fetch := func(_ context.Context, cursor sdk.Cursor) (sdk.Page[int], error) {
		if !cursor.IsZero() {
			t.Fatalf("unexpected continuation fetch")
		}
		// Five items, no continuation cursor; 3 remain unreturned after the limit.
		return sdk.Page[int]{Items: []int{1, 2, 3, 4, 5}}, nil
	}
	items, hasMore, err := CollectPages[int](context.Background(), plan, fetch)
	if err != nil {
		t.Fatalf("CollectPages error = %v", err)
	}
	if len(items) != 2 || items[0] != 1 || items[1] != 2 {
		t.Fatalf("items = %#v", items)
	}
	if !hasMore {
		t.Fatalf("hasMore = false, want true (batch truncated with unreturned items)")
	}
}

// TestCollectPagesHasMoreFalseWhenBatchFullyReturned 验证 FANBOX 侧非截断路径
// 不被 #17 修复误判为 has_more=true。
func TestCollectPagesHasMoreFalseWhenBatchFullyReturned(t *testing.T) {
	plan := ListPlan{Page: 1, Limit: 5}
	fetch := func(_ context.Context, cursor sdk.Cursor) (sdk.Page[int], error) {
		return sdk.Page[int]{Items: []int{1, 2, 3}}, nil
	}
	items, hasMore, err := CollectPages[int](context.Background(), plan, fetch)
	if err != nil {
		t.Fatalf("CollectPages error = %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("items = %#v", items)
	}
	if hasMore {
		t.Fatalf("hasMore = true, want false (batch fully returned, no continuation)")
	}
}
