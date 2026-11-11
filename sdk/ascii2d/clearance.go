package ascii2d

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk/internal/cloudflare"
)

type clearanceKey struct {
	proxy, solver, solverProxy, agent string
	connection                        *http.Client
}

// clearancePool retains only clearance between calls. The mutex protects entries,
// references and state; each search keeps its browser alive until its pages finish.
var clearancePool = struct {
	sync.Mutex
	entries map[clearanceKey]*clearanceEntry
}{entries: make(map[clearanceKey]*clearanceEntry)}

type clearanceEntry struct {
	key        clearanceKey
	browser    *cloudflare.Client
	users      int
	state      *cloudflare.Clearance
	generation uint64
}

func acquireClearance(options Options) (*clearanceEntry, *cloudflare.Client, error) {
	key := clearanceKey{options.ProxyURL, options.FlareSolverrURL, options.FlareSolverrProxyURL, options.UserAgent, options.HTTPClient}
	clearancePool.Lock()
	defer clearancePool.Unlock()
	// Reclaim idle expired states even when callers stop reusing their HTTP client.
	for key, entry := range clearancePool.entries {
		if entry.users == 0 && entry.state != nil && !entry.state.ExpiresAt.IsZero() && !time.Now().Before(entry.state.ExpiresAt) {
			delete(clearancePool.entries, key)
		}
	}
	entry := clearancePool.entries[key]
	if entry == nil {
		entry = &clearanceEntry{key: key}
	}
	if entry.browser == nil {
		browser, err := cloudflare.New(cloudflare.Options{ProxyURL: key.proxy, FlareSolverrURL: key.solver, FlareSolverrProxyURL: key.solverProxy, Origin: origin, PersistentSession: true})
		if err != nil {
			return nil, nil, err
		}
		entry.browser = browser
	}
	entry.users++
	clearancePool.entries[key] = entry
	return entry, entry.browser, nil
}

func (entry *clearanceEntry) release(ctx context.Context) error {
	clearancePool.Lock()
	entry.users--
	var browser *cloudflare.Client
	if entry.users == 0 {
		browser, entry.browser = entry.browser, nil
		if entry.state == nil || (!entry.state.ExpiresAt.IsZero() && !time.Now().Before(entry.state.ExpiresAt)) {
			delete(clearancePool.entries, entry.key)
		}
	}
	clearancePool.Unlock()
	if browser != nil {
		// Shared solves are joined and remote sessions are destroyed even after cancellation.
		return browser.Close(context.WithoutCancel(ctx))
	}
	return nil
}

func (entry *clearanceEntry) solve(ctx context.Context, browser *cloudflare.Client, target string) (cloudflare.Clearance, error) {
	clearancePool.Lock()
	if entry.state != nil && (entry.state.ExpiresAt.IsZero() || time.Now().Before(entry.state.ExpiresAt)) {
		state := *entry.state
		clearancePool.Unlock()
		return state, nil
	}
	entry.state = nil
	generation := entry.generation
	clearancePool.Unlock()
	state, err := browser.Solve(ctx, target)
	if err == nil {
		clearancePool.Lock()
		// A late shared-solve waiter must not restore clearance rejected by a replay.
		if entry.generation == generation {
			entry.state = &state
		}
		clearancePool.Unlock()
	}
	return state, err
}

func (entry *clearanceEntry) invalidate(browser *cloudflare.Client, rejected cloudflare.Clearance) {
	clearancePool.Lock()
	defer clearancePool.Unlock()
	if entry.state != nil && entry.state.Cookie.Value() == rejected.Cookie.Value() && entry.state.UserAgent == rejected.UserAgent && entry.state.ExpiresAt.Equal(rejected.ExpiresAt) {
		entry.state = nil
		entry.generation++
		browser.Invalidate()
	}
}
