package accounts_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/accounts"
	"github.com/stretchr/testify/require"
)

func TestLoginManagerReusesPendingAndReplacesOnRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	starts := 0
	manager, err := accounts.NewLoginManager(ctx, "https://relay.example/pixiv-login", func() (accounts.LoginAttempt, error) {
		starts++
		return accounts.LoginAttempt{AuthorizationURL: "https://app-api.pixiv.net/web/v1/login", AcceptsCallback: func(string) bool { return true }, Complete: func(context.Context, string) (accounts.LoginResult, error) { return accounts.LoginResult{}, nil }}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	first, err := manager.Start(false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Start(false)
	if err != nil {
		t.Fatal(err)
	}
	if first.LoginID == "" || second.LoginID != first.LoginID || second.AuthorizationURL != first.AuthorizationURL || starts != 1 {
		t.Fatal("pending login was replaced")
	}
	restarted, err := manager.Start(true)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.LoginID == first.LoginID || starts != 2 {
		t.Fatal("restart did not create a fresh login")
	}
	if manager.Status(first.LoginID).Status != "not_found" {
		t.Fatal("old login history retained")
	}
	old := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, first.AuthorizationURL, nil)
	req.URL.Path = req.URL.Path[len("/pixiv-login"):]
	manager.ServeHTTP(old, req)
	if old.Code != http.StatusNotFound {
		t.Fatalf("old route status: %d", old.Code)
	}
	manager.Close()
	if _, err := manager.Start(false); err == nil {
		t.Fatal("closed manager started login")
	}
}

func TestLoginManagerDoesNotPublishSessionCreatedDuringShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	manager, err := accounts.NewLoginManager(ctx, "https://relay.example/pixiv-login", func() (accounts.LoginAttempt, error) {
		close(entered)
		<-release
		return accounts.LoginAttempt{AuthorizationURL: "https://app-api.pixiv.net/web/v1/login", AcceptsCallback: func(string) bool { return true }, Complete: func(context.Context, string) (accounts.LoginResult, error) { return accounts.LoginResult{}, nil }}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	finished := make(chan error, 1)
	go func() { _, err := manager.Start(false); finished <- err }()
	<-entered
	cancel()
	close(release)
	if err := <-finished; err == nil {
		t.Fatal("shutdown published a new login")
	}
}

func TestLoginManagerPersistsAfterHelperDisconnectAndPreservesPartialFailure(t *testing.T) {
	for _, selectionFails := range []bool{false, true} {
		name := "saved-and-selected"
		if selectionFails {
			name = "selection-failed"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			exchangeEntered, releaseExchange := make(chan struct{}), make(chan struct{})
			manager, err := accounts.NewLoginManager(ctx, "https://relay.example/pixiv-login", func() (accounts.LoginAttempt, error) {
				return accounts.LoginAttempt{
					AuthorizationURL: "https://app-api.pixiv.net/web/v1/login",
					AcceptsCallback:  func(raw string) bool { return raw == "pixiv://account/login?code=fixture" },
					Complete: func(ctx context.Context, raw string) (accounts.LoginResult, error) {
						close(exchangeEntered)
						select {
						case <-releaseExchange:
						case <-ctx.Done():
							return accounts.LoginResult{}, ctx.Err()
						}
						result := accounts.LoginResult{Account: accounts.Account{UserID: 73, Username: "fixture", HasCredentials: true}, AccountSaved: true, SelectionUpdated: !selectionFails}
						if selectionFails {
							return result, errors.New("secret storage path must not escape")
						}
						return result, nil
					},
				}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Close()
			started, err := manager.Start(false)
			if err != nil {
				t.Fatal(err)
			}
			page := httptest.NewRecorder()
			entry, _ := url.Parse(started.AuthorizationURL)
			manager.ServeHTTP(page, httptest.NewRequest(http.MethodGet, strings.TrimPrefix(entry.Path, "/pixiv-login"), nil))
			link, _ := url.Parse(page.Header().Get("Location"))
			id, proof := link.Query().Get("session"), link.Query().Get("access")
			start := httptest.NewRecorder()
			manager.ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/start/"+id, strings.NewReader(`{"proof":"`+proof+`"}`)))
			if start.Code != http.StatusOK {
				t.Fatal("helper start failed")
			}
			callbackCtx, disconnect := context.WithCancel(context.Background())
			callback := httptest.NewRecorder()
			callbackDone := make(chan struct{})
			go func() {
				manager.ServeHTTP(callback, httptest.NewRequest(http.MethodPost, "/callback/"+id, strings.NewReader(`{"proof":"`+proof+`","callback_url":"pixiv://account/login?code=fixture"}`)).WithContext(callbackCtx))
				close(callbackDone)
			}()
			<-exchangeEntered
			if manager.Status(started.LoginID).Status != "exchanging" {
				t.Fatal("missing exchanging state")
			}
			reused, err := manager.Start(false)
			if err != nil || reused.LoginID != started.LoginID {
				t.Fatal("exchange was replaced")
			}
			disconnect()
			<-callbackDone
			close(releaseExchange)
			require.Eventually(t, func() bool {
				state := manager.Status(started.LoginID).Status
				return state == "completed" || state == "failed"
			}, 5*time.Second, time.Millisecond)
			result := manager.Status(started.LoginID)
			if !result.AccountSaved || result.Account == nil || result.Account.UserID != 73 || result.SelectionUpdated == selectionFails {
				t.Fatalf("lost persistence result: %#v", result)
			}
			if selectionFails && (result.Status != "failed" || result.ErrorCode != "login_failed") {
				t.Fatal("selection error hidden or leaked")
			}
			if !selectionFails && result.Status != "completed" {
				t.Fatal("successful login not completed")
			}
			result.Account.UserID = 99
			if manager.Status(started.LoginID).Account.UserID != 73 {
				t.Fatal("status leaked mutable internal state")
			}
		})
	}
}
