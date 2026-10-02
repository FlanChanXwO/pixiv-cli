package loginrelay_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/account/loginrelay"
)

// 同一主 mux 挂载会话；fixture 不创建独立 relay listener，也不触及真实账号。
func TestSessionMountedOnMainMux(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		name := "result-page"
		if disconnect {
			name = "callback-disconnect"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			mux := http.NewServeMux()
			server := httptest.NewUnstartedServer(mux)
			t.Cleanup(server.Close)
			t.Cleanup(cancel)
			base := "http://" + server.Listener.Addr().String() + "/pixiv-login"
			const callback = "pixiv://account/login?code=fixture-code&state=fixture-state"
			session, err := loginrelay.New(ctx, base, "https://app-api.pixiv.net/web/v1/login", func(raw string) bool { return raw == callback })
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(session.Stop)
			mux.Handle("/pixiv-login/", http.StripPrefix("/pixiv-login", session.Handler))
			mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
			server.Start()
			client := server.Client()
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			response, err := client.Get(session.URL)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusSeeOther {
				t.Fatalf("session status: %d", response.StatusCode)
			}
			link, err := url.Parse(response.Header.Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			q := link.Query()
			if q.Get("origin") != base {
				t.Fatalf("origin lost prefix: %s", q.Get("origin"))
			}
			id, proof := q.Get("session"), q.Get("access")
			post := func(route string, body any, status int) *http.Response {
				t.Helper()
				data, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				response, err := client.Post(base+"/"+route+"/"+id, "application/json", strings.NewReader(string(data)))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { response.Body.Close() })
				if response.StatusCode != status {
					t.Fatalf("%s status: got %d want %d", route, response.StatusCode, status)
				}
				return response
			}
			payload := map[string]string{"proof": proof, "callback_url": callback}
			post("callback", payload, http.StatusConflict).Body.Close()
			post("start", map[string]string{"proof": "wrong"}, http.StatusUnauthorized).Body.Close()
			for range 2 { // 重复 start 仍属于同一个 OAuth 会话。
				response := post("start", map[string]string{"proof": proof}, http.StatusOK)
				var result loginrelay.RemoteLoginStartResponse
				if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if result.AuthorizationURL != "https://app-api.pixiv.net/web/v1/login" {
					t.Fatal("authorization URL changed")
				}
			}
			post("callback", map[string]string{"proof": proof, "callback_url": "pixiv://account/login?code=other&state=wrong"}, http.StatusBadRequest).Body.Close()
			response = post("callback", payload, http.StatusOK)
			select {
			case got := <-session.Callback:
				if got != callback {
					t.Fatal("callback changed")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("callback not delivered")
			}
			session.Stop()
			post("callback", payload, http.StatusConflict).Body.Close()
			post("start", map[string]string{"proof": proof}, http.StatusConflict).Body.Close()
			complete := make(chan struct{})
			if disconnect {
				response.Body.Close()
			}
			go func() { session.Complete(true); close(complete) }()
			if !disconnect {
				resultURL := response.Header.Get(loginrelay.RelayResultURLHeader)
				if !strings.HasPrefix(resultURL, base+"/result/") {
					t.Fatal("result URL lost prefix")
				}
				page, err := client.Get(resultURL)
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(page.Body)
				page.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				if page.StatusCode != http.StatusOK || !strings.Contains(string(body), "Login successful") {
					t.Fatal("missing successful result page")
				}
				var final struct {
					Success bool `json:"success"`
				}
				if err := json.NewDecoder(response.Body).Decode(&final); err != nil {
					t.Fatal(err)
				}
				if !final.Success {
					t.Fatal("callback did not receive final result")
				}
				response.Body.Close()
				duplicate, err := client.Get(resultURL)
				if err != nil {
					t.Fatal(err)
				}
				duplicate.Body.Close()
				if duplicate.StatusCode != http.StatusConflict {
					t.Fatal("result page reused")
				}
			}
			select {
			case <-complete:
			case <-time.After(5 * time.Second):
				t.Fatal("completion blocked after result/disconnect")
			}
			select {
			case <-session.Done:
			case <-time.After(5 * time.Second):
				t.Fatal("session waiter leaked")
			}
			health, err := client.Get(server.URL + "/health")
			if err != nil {
				t.Fatal(err)
			}
			health.Body.Close()
			if health.StatusCode != http.StatusNoContent {
				t.Fatal("relay affected sibling route")
			}
		})
	}
}
