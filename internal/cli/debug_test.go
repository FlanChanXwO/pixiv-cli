package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestDebugOnlyWritesNarrativeToStderr(t *testing.T) {
	useTempPaths(t)

	var quietOut, quietErr bytes.Buffer
	if code := Run([]string{"pixiv", "config", "path"}, strings.NewReader(""), &quietOut, &quietErr); code != 0 {
		t.Fatalf("quiet config path code=%d stderr=%q", code, quietErr.String())
	}
	if quietErr.Len() != 0 {
		t.Fatalf("quiet command wrote diagnostics: %q", quietErr.String())
	}

	var debugOut, debugErr bytes.Buffer
	if code := Run([]string{"pixiv", "config", "path", "--debug"}, strings.NewReader(""), &debugOut, &debugErr); code != 0 {
		t.Fatalf("debug config path code=%d stderr=%q", code, debugErr.String())
	}
	if debugOut.String() != quietOut.String() {
		t.Fatalf("debug changed stdout: quiet=%q debug=%q", quietOut.String(), debugOut.String())
	}
	if !strings.Contains(debugErr.String(), "[Pixiv CLI]") || !strings.Contains(debugErr.String(), "Started") || !strings.Contains(debugErr.String(), "completed successfully") {
		t.Fatalf("debug narrative missing lifecycle: %q", debugErr.String())
	}
}

func TestDebugUnknownOptionDoesNotCreateScope(t *testing.T) {
	useTempPaths(t)
	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "config", "path", "--debug", "--unknown"}, strings.NewReader(""), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("unknown option code=%d stderr=%q", code, stderr.String())
	}
	if got := stderr.String(); got != "error: unknown option '--unknown'\n" {
		t.Fatalf("unknown option output=%q", got)
	}
}

func TestDebugAuthExportPreservesSecretOutputContract(t *testing.T) {
	authPath, _ := useTempPaths(t)
	const token = "debug-export-secret"
	if err := saveTestAuthStore(t, authPath, testAuthStore{
		DefaultUserID: 7,
		Accounts:      []testAuthAccount{{UserID: 7, RefreshToken: token}},
	}); err != nil {
		t.Fatalf("seed auth store: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"pixiv", "--debug", "auth", "export"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || stdout.String() != token+"\n" || stderr.Len() != 0 {
		t.Fatalf("debug auth export changed contract: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestDebugWriterFailureDoesNotCancelCommand(t *testing.T) {
	useTempPaths(t)
	var stdout bytes.Buffer
	writerErr := errors.New("diagnostic writer closed")
	code := Run([]string{"pixiv", "config", "path", "--debug"}, strings.NewReader(""), &stdout, &diagnosticFailingWriter{err: writerErr})
	if code == 0 {
		t.Fatal("writer failure must be visible as a process-level failure")
	}
	if stdout.Len() == 0 {
		t.Fatal("writer failure cancelled the command before stdout was produced")
	}
}

type diagnosticFailingWriter struct{ err error }

func (w *diagnosticFailingWriter) Write([]byte) (int, error) { return 0, w.err }
