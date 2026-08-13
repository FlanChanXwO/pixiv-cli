package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/diagnostics"
)

var (
	// ErrSolverUnavailable means the direct control request could not reach the
	// configured FlareSolverr service. It is distinct from a solver response
	// that explicitly reports failure.
	ErrSolverUnavailable = errors.New("fanbox: FlareSolverr service unavailable")
	// ErrSolverFailed means FlareSolverr responded but did not solve the
	// anonymous homepage challenge.
	ErrSolverFailed = errors.New("fanbox: FlareSolverr could not solve challenge")
	// ErrMalformedSolverResponse means the response could not satisfy the
	// clearance-only state contract.
	ErrMalformedSolverResponse = errors.New("fanbox: malformed FlareSolverr response")
)

type solverState struct {
	userAgent string
	clearance string
	expiresAt time.Time
	hasExpiry bool
}

type solverCall struct {
	ctx     context.Context
	cancel  context.CancelFunc
	waiters int
	done    chan struct{}
	state   solverState
	err     error
}

type solverResponse struct {
	Status   string          `json:"status"`
	Solution *solverSolution `json:"solution"`
}

type solverSolution struct {
	UserAgent string         `json:"userAgent"`
	Cookies   []solverCookie `json:"cookies"`
}

type solverCookie struct {
	Name    string          `json:"name"`
	Value   string          `json:"value"`
	Expires json.RawMessage `json:"expires"`
	Expiry  json.RawMessage `json:"expiry"`
}

func (s *Session) nativeState() (string, string) {
	if s == nil {
		return userAgent, ""
	}
	s.solverMu.Lock()
	defer s.solverMu.Unlock()
	if s.solverState != nil && s.solverState.hasExpiry && !time.Now().Before(s.solverState.expiresAt) {
		s.solverState = nil
	}
	if s.solverState == nil {
		return s.userAgent, ""
	}
	return s.solverState.userAgent, s.solverState.clearance
}

func nativeCookieHeader(session, clearance string) string {
	if clearance == "" {
		return session
	}
	if session == "" {
		return "cf_clearance=" + clearance
	}
	return session + "; cf_clearance=" + clearance
}

func (s *Session) invalidateSolverState() {
	if s == nil {
		return
	}
	s.solverMu.Lock()
	s.solverState = nil
	s.solverMu.Unlock()
}

func (s *Session) waitForSolver(ctx context.Context) (solverState, error) {
	if s == nil || s.flareSolverr == nil {
		return solverState{}, ErrChallenge
	}
	if err := ctx.Err(); err != nil {
		return solverState{}, err
	}

	s.solverMu.Lock()
	if s.solverState != nil && (!s.solverState.hasExpiry || time.Now().Before(s.solverState.expiresAt)) {
		state := *s.solverState
		s.solverMu.Unlock()
		return state, nil
	}
	s.solverState = nil
	call := s.solverActive
	if call == nil || call.waiters == 0 {
		if call != nil {
			call.cancel()
		}
		// The solver request owns its cancellation independently of any one
		// waiter, while context values (including an opt-in diagnostics scope)
		// remain available for the shared operation.
		callContext, cancel := context.WithCancel(context.WithoutCancel(ctx))
		call = &solverCall{ctx: callContext, cancel: cancel, done: make(chan struct{}), waiters: 1}
		s.solverActive = call
		go s.runSolver(call)
	} else {
		call.waiters++
	}
	s.solverMu.Unlock()

	select {
	case <-call.done:
		return call.state, call.err
	case <-ctx.Done():
		s.unregisterSolverWaiter(call)
		return solverState{}, ctx.Err()
	}
}

func (s *Session) unregisterSolverWaiter(call *solverCall) {
	s.solverMu.Lock()
	defer s.solverMu.Unlock()
	if s.solverActive != call || call.waiters == 0 {
		return
	}
	call.waiters--
	if call.waiters == 0 {
		// There is no caller left to consume this solve. Cancel the direct
		// request and let runSolver discard any completion that races with it.
		call.cancel()
	}
}

func (s *Session) runSolver(call *solverCall) {
	state, err := s.solveFlareSolverr(call.ctx)
	if err == nil {
		diagnostics.Emit(call.ctx, diagnostics.Event{
			Module:    diagnostics.ModuleFanboxSolver,
			Kind:      diagnostics.EventSolverCompleted,
			Operation: "clearance",
		})
	} else if call.ctx.Err() == nil {
		diagnostics.Emit(call.ctx, diagnostics.Event{
			Module:    diagnostics.ModuleFanboxSolver,
			Kind:      diagnostics.EventFailed,
			Operation: "challenge recovery",
			Reason:    solverDiagnosticReason(err),
		})
	}
	s.solverMu.Lock()
	call.state = state
	call.err = err
	if s.solverActive == call {
		if call.waiters > 0 && err == nil {
			cached := state
			s.solverState = &cached
		}
		s.solverActive = nil
	}
	close(call.done)
	s.solverMu.Unlock()
}

func (s *Session) solveFlareSolverr(ctx context.Context) (solverState, error) {
	if s.flareSolverr == nil {
		return solverState{}, ErrChallenge
	}
	diagnostics.Emit(ctx, diagnostics.Event{
		Module:    diagnostics.ModuleFanboxSolver,
		Kind:      diagnostics.EventSolverStarted,
		Operation: "challenge recovery",
	})
	payload := []byte(`{"cmd":"request.get","url":"https://www.fanbox.cc/"}`)
	if s.flareSolverr.ProxyURL != "" {
		payload = fmt.Appendf(nil, `{"cmd":"request.get","url":"https://www.fanbox.cc/","proxy":%q}`, s.flareSolverr.ProxyURL)
	}
	endpoint := strings.TrimRight(s.flareSolverr.URL, "/") + "/v1"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return solverState{}, ErrSolverUnavailable
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	client := s.solverHTTPClient
	if client == nil {
		transport := &http.Transport{Proxy: nil}
		client = &http.Client{
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		defer transport.CloseIdleConnections()
	}
	response, err := client.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return solverState{}, ctxErr
		}
		return solverState{}, ErrSolverUnavailable
	}
	if response == nil || response.Body == nil {
		return solverState{}, ErrMalformedSolverResponse
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return solverState{}, ErrSolverFailed
	}
	var decoded solverResponse
	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(&decoded); err != nil {
		return solverState{}, ErrMalformedSolverResponse
	}
	if decoded.Status != "ok" {
		return solverState{}, ErrSolverFailed
	}
	return validateSolverSolution(decoded.Solution)
}

func solverDiagnosticReason(err error) diagnostics.Reason {
	switch {
	case errors.Is(err, ErrSolverUnavailable):
		return diagnostics.ReasonSolverUnavailable
	case errors.Is(err, ErrSolverFailed):
		return diagnostics.ReasonSolverFailed
	case errors.Is(err, ErrMalformedSolverResponse):
		return diagnostics.ReasonMalformedSolver
	default:
		return diagnostics.ReasonCommandFailed
	}
}

func validateSolverSolution(solution *solverSolution) (solverState, error) {
	if solution == nil || !validHeaderValue(solution.UserAgent) {
		return solverState{}, ErrMalformedSolverResponse
	}
	var clearance string
	var expiryRaw json.RawMessage
	count := 0
	for _, cookie := range solution.Cookies {
		if cookie.Name != "cf_clearance" {
			// Other solver cookies are deliberately discarded and are not
			// validated or counted toward the clearance contract.
			continue
		}
		count++
		if count > 1 || cookie.Value == "" || !validCookieValue(cookie.Value) {
			return solverState{}, ErrMalformedSolverResponse
		}
		clearance = cookie.Value
		if len(cookie.Expiry) != 0 && string(cookie.Expiry) != "null" {
			expiryRaw = cookie.Expiry
		} else if len(cookie.Expires) != 0 && string(cookie.Expires) != "null" {
			expiryRaw = cookie.Expires
		}
	}
	if count != 1 {
		return solverState{}, ErrMalformedSolverResponse
	}
	expiresAt, hasExpiry, err := parseSolverExpiry(expiryRaw)
	if err != nil {
		return solverState{}, ErrMalformedSolverResponse
	}
	return solverState{userAgent: solution.UserAgent, clearance: clearance, expiresAt: expiresAt, hasExpiry: hasExpiry}, nil
}

func validHeaderValue(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}

func parseSolverExpiry(raw json.RawMessage) (time.Time, bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}, false, nil
	}
	var numeric json.Number
	if err := json.Unmarshal(raw, &numeric); err == nil {
		value, err := numeric.Int64()
		if err != nil || value <= 0 {
			return time.Time{}, false, errors.New("solver expiry is invalid")
		}
		return time.Unix(value, 0), true, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil || text == "" {
		return time.Time{}, false, errors.New("solver expiry is invalid")
	}
	for _, layout := range []string{time.RFC3339, http.TimeFormat} {
		if value, err := time.Parse(layout, text); err == nil {
			return value, true, nil
		}
	}
	return time.Time{}, false, errors.New("solver expiry is invalid")
}
