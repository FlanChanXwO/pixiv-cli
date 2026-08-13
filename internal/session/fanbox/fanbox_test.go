package fanbox_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	accountfanbox "github.com/FlanChanXwO/pixiv-cli/internal/account/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	sessionfanbox "github.com/FlanChanXwO/pixiv-cli/internal/session/fanbox"
	fanboxsdk "github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

func TestRunUsesIndependentClientsForConcurrentOperations(t *testing.T) {
	account := accountfanbox.New(7, "creator", "creator-id", []byte("opaque-session"))
	var openCalls atomic.Int32
	var mu sync.Mutex
	clients := make([]*fanboxsdk.Client, 0, 2)

	open := func(_ context.Context, got accountfanbox.Account) (*fanboxsdk.Client, error) {
		if got.UserID != account.UserID || string(got.SessionIDCopy()) != "opaque-session" {
			t.Fatalf("open account = %v", got)
		}
		client := &fanboxsdk.Client{}
		openCalls.Add(1)
		mu.Lock()
		clients = append(clients, client)
		mu.Unlock()
		return client, nil
	}
	use := func(_ context.Context, _ *fanboxsdk.Client, _ *session.Attempt) error {
		return nil
	}

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := sessionfanbox.Run(context.Background(), account, open, use); err != nil {
				t.Errorf("Run: %v", err)
			}
		}()
	}
	wg.Wait()

	if openCalls.Load() != 2 {
		t.Fatalf("open calls = %d, want 2", openCalls.Load())
	}
	if len(clients) != 2 || clients[0] == clients[1] {
		t.Fatalf("clients = %p, %p; operations must not share a client", clients[0], clients[1])
	}
}
