package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunNoArgsPrintsHelp(t *testing.T) {
	useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.Contains(t, stdout.String(), "Usage:")
	assert.Contains(t, stdout.String(), "account")
	assert.Contains(t, stdout.String(), "config")
	assert.NotContains(t, stdout.String(), "completion")
}

func TestRunUnknownCommandReturnsError(t *testing.T) {
	useTempPaths(t)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "wat"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Contains(t, stderr.String(), `unknown command "wat"`)
}

func TestRunMCPDispatch(t *testing.T) {
	useTempPaths(t)

	old := runMCPServer
	t.Cleanup(func() { runMCPServer = old })

	called := false
	runMCPServer = func(context.Context, io.Writer) error {
		called = true
		return nil
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "mcp"}, strings.NewReader(""), &stdout, &stderr)

	require.Equal(t, 0, code, stderr.String())
	assert.True(t, called)
}

func TestRunMCPDispatchError(t *testing.T) {
	useTempPaths(t)

	old := runMCPServer
	t.Cleanup(func() { runMCPServer = old })
	runMCPServer = func(context.Context, io.Writer) error {
		return errors.New("boom")
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "mcp"}, strings.NewReader(""), &stdout, &stderr)

	require.NotZero(t, code)
	assert.Contains(t, stderr.String(), "boom")
}
