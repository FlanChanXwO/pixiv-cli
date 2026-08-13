package protocol_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	postinfo "github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/post/info"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/protocol"
)

func TestChallengeRecoveryUsesIndependentSolverTransportAndReplaysRequest(t *testing.T) {
	var nativeCalls atomic.Int32
	var solverCalls atomic.Int32
	native := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		call := nativeCalls.Add(1)
		if call == 1 {
			return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"Cf-Mitigated": {"challenge"}}, Body: io.NopCloser(strings.NewReader("challenge"))}, nil
		}
		if got := request.Header.Get("User-Agent"); got != "solver-agent" {
			t.Errorf("replay User-Agent = %q", got)
		}
		if got := request.Header.Get("Cookie"); !strings.Contains(got, "FANBOXSESSID=session-canary") || !strings.Contains(got, "cf_clearance=clearance-canary") {
			t.Errorf("replay Cookie = %q", got)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"body":{"post":{"id":"post-1","title":"replayed"}}}`))}, nil
	})
	solver := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		solverCalls.Add(1)
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		if strings.Contains(string(body), "session-canary") || strings.Contains(string(body), "post.info") {
			t.Errorf("solver request contains business or credential data: %q", body)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"status":"ok","solution":{"userAgent":"solver-agent","cookies":[{"name":"cf_clearance","value":"clearance-canary"}]}}`))}, nil
	})
	session, err := protocol.NewSessionWithOptions("FANBOXSESSID=session-canary", protocol.SessionOptions{
		HTTPClient:       &http.Client{Transport: native},
		SolverHTTPClient: &http.Client{Transport: solver},
		FlareSolverr:     &protocol.FlareSolverrOptions{URL: "http://solver.example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := postinfo.New(session)
	post, err := endpoint.Get(context.Background(), postinfo.Request{PostID: "post-1"})
	if err != nil {
		t.Fatalf("post endpoint error = %v", err)
	}
	if post.ID != "post-1" || post.Title != "replayed" {
		t.Fatalf("post = %+v", post)
	}
	if got := solverCalls.Load(); got != 1 {
		t.Fatalf("solver calls = %d, want 1", got)
	}
	if _, err := endpoint.Get(context.Background(), postinfo.Request{PostID: "post-1"}); err != nil {
		t.Fatalf("cached replay error = %v", err)
	}
	if got := solverCalls.Load(); got != 1 {
		t.Fatalf("cached solver calls = %d, want 1", got)
	}
}

func TestOrdinaryForbiddenDoesNotCallSolver(t *testing.T) {
	var solverCalls atomic.Int32
	session, err := protocol.NewSessionWithOptions("FANBOXSESSID=session-canary", protocol.SessionOptions{
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":"challenge"}`))}, nil
		})},
		SolverHTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			solverCalls.Add(1)
			return nil, errors.New("solver should not be called")
		})},
		FlareSolverr: &protocol.FlareSolverrOptions{URL: "http://solver.example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = postinfo.New(session).Get(context.Background(), postinfo.Request{PostID: "post-1"})
	if !errors.Is(err, protocol.ErrForbidden) {
		t.Fatalf("error = %v, want forbidden", err)
	}
	if got := solverCalls.Load(); got != 0 {
		t.Fatalf("solver calls = %d, want 0", got)
	}
}
