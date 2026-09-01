package ascii2d

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	reversesearch "github.com/FlanChanXwO/pixiv-cli/internal/services/reversesearch"
	"github.com/stretchr/testify/require"
)

type challengeReadErrorBody struct {
	readCalls int
}

func (body *challengeReadErrorBody) Read([]byte) (int, error) {
	body.readCalls++
	return 0, errors.New("challenge body read failed")
}

func (body *challengeReadErrorBody) Close() error { return nil }

func TestClassifyResponseErrorPrioritizesChallengeHeader(t *testing.T) {
	body := &challengeReadErrorBody{}
	err := classifyResponseError(context.Background(), &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{"Cf-Mitigated": []string{"challenge"}, "Content-Type": []string{"text/html"}},
		Body:       body,
	}, "ordinary upstream failure")

	require.ErrorIs(t, err, errChallengeDetected)
	require.Equal(t, reversesearch.CodeUpstreamHTTPStatus, reversesearch.CodeOf(err))
	require.EqualError(t, err, "ascii2d challenge detected")
	require.Zero(t, body.readCalls)
}

func TestClassifyResponseErrorScansHTMLChallengeAsAStream(t *testing.T) {
	htmlBody := strings.Repeat("ordinary prefix ", 5000) + `<html><head><title>Just a moment...</title></head><body></body></html>`
	err := classifyResponseError(context.Background(), &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(htmlBody)),
	}, "ordinary upstream failure")

	require.ErrorIs(t, err, errChallengeDetected)
	require.Equal(t, reversesearch.CodeUpstreamHTTPStatus, reversesearch.CodeOf(err))
}

func TestClassifyResponseErrorLeavesOrdinaryForbiddenAndNonHTMLErrorsUnchanged(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		header     http.Header
		body       string
	}{
		{
			name:       "ordinary html forbidden",
			statusCode: http.StatusForbidden,
			header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
			body:       `<html><body><h1>Forbidden</h1><p>Access denied.</p></body></html>`,
		},
		{
			name:       "non html body",
			statusCode: http.StatusForbidden,
			header:     http.Header{"Content-Type": []string{"application/json"}},
			body:       `{"error":"challenge"}`,
		},
		{
			name:       "non forbidden html status",
			statusCode: http.StatusInternalServerError,
			header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
			body:       `<html><head><title>Just a moment...</title></head></html>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := classifyResponseError(context.Background(), &http.Response{
				StatusCode: test.statusCode,
				Header:     test.header,
				Body:       io.NopCloser(strings.NewReader(test.body)),
			}, "ordinary upstream failure")

			require.NotErrorIs(t, err, errChallengeDetected)
			require.Equal(t, reversesearch.CodeUpstreamHTTPStatus, reversesearch.CodeOf(err))
			require.EqualError(t, err, "ordinary upstream failure")
		})
	}
}

func TestClassifyResponseErrorMapsBodyReadFailureAndPreservesCancellation(t *testing.T) {
	t.Run("body read failure", func(t *testing.T) {
		err := classifyResponseError(context.Background(), &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     http.Header{"Content-Type": []string{"text/html"}},
			Body:       &challengeReadErrorBody{},
		}, "ordinary upstream failure")

		require.Equal(t, reversesearch.CodeProviderFailed, reversesearch.CodeOf(err))
		require.EqualError(t, err, "could not read ascii2d error response")
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		body := readerFunc(func([]byte) (int, error) {
			cancel()
			return 0, errors.New("request canceled while reading challenge body")
		})

		err := classifyResponseError(ctx, &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     http.Header{"Content-Type": []string{"text/html"}},
			Body:       body,
		}, "ordinary upstream failure")

		require.ErrorIs(t, err, context.Canceled)
	})
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

func (readerFunc) Close() error { return nil }
