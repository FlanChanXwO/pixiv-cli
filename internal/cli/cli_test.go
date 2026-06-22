package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestRunNoArgsPrintsHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage: pixiv <command>") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunUnknownCommandReturnsError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "wat"}, strings.NewReader(""), &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit")
	}
	if !strings.Contains(stderr.String(), `unknown command "wat"`) {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunMCPDispatch(t *testing.T) {
	old := runMCPServer
	defer func() { runMCPServer = old }()
	called := false
	runMCPServer = func(context.Context, io.Writer) error {
		called = true
		return nil
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "mcp"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || !called {
		t.Fatalf("code=%d called=%v stderr=%s", code, called, stderr.String())
	}
}

func TestRunMCPDispatchError(t *testing.T) {
	old := runMCPServer
	defer func() { runMCPServer = old }()
	runMCPServer = func(context.Context, io.Writer) error {
		return errors.New("boom")
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "mcp"}, strings.NewReader(""), &stdout, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), "boom") {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
}
