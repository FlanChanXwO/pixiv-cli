package pixiv

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestLocalStateClassifiesPermissionBeforeStage(t *testing.T) {
	t.Parallel()
	raw := markLocalState(localStateStageConfig, &fs.PathError{Op: "open-operation-secret", Path: "permission-path-secret", Err: fs.ErrPermission})
	mapped := localSnapshotError(OperationConfigGet, raw)
	assertClassifiedLocalStateError(t, mapped, OperationConfigGet, LocalStateKindPermissionDenied, "local state permission denied", "operation-secret", "permission-path-secret")
}

func TestLocalStateClassifiesNotFoundBeforeStage(t *testing.T) {
	t.Parallel()
	raw := markLocalState(localStateStageAuth, &fs.PathError{Op: "read-operation-secret", Path: "not-found-path-secret", Err: fs.ErrNotExist})
	mapped := localSnapshotError(OperationListAccounts, raw)
	assertClassifiedLocalStateError(t, mapped, OperationListAccounts, LocalStateKindNotFound, "local state was not found", "operation-secret", "not-found-path-secret")
}

func TestLocalStateClassifiesOtherPathFailureUnavailable(t *testing.T) {
	t.Parallel()
	raw := markLocalState(localStateStageAuth, &fs.PathError{Op: "read-operation-secret", Path: "unavailable-path-secret", Err: errors.New("filesystem-cause-secret")})
	mapped := localSnapshotError(OperationListAccounts, raw)
	assertClassifiedLocalStateError(t, mapped, OperationListAccounts, LocalStateKindUnavailable, "local state is unavailable", "operation-secret", "unavailable-path-secret", "filesystem-cause-secret")
}

func TestLocalStateClassifiesLinkFailureUnavailable(t *testing.T) {
	t.Parallel()
	raw := markLocalState(localStateStageAuth, &os.LinkError{
		Op:  "rename-operation-secret",
		Old: "old-auth-path-secret",
		New: "new-auth-path-secret",
		Err: errors.New("link-cause-secret"),
	})
	mapped := localSnapshotError(OperationImportAccount, raw)
	assertClassifiedLocalStateError(t, mapped, OperationImportAccount, LocalStateKindUnavailable, "local state is unavailable", "operation-secret", "old-auth-path-secret", "new-auth-path-secret", "link-cause-secret")
}

func TestLocalStateClassifiesSyscallFailureUnavailable(t *testing.T) {
	t.Parallel()
	raw := markLocalState(localStateStageAuth, &os.SyscallError{
		Syscall: "replace-file-operation-secret",
		Err:     errors.New("syscall-cause-secret"),
	})
	mapped := localSnapshotError(OperationCompleteLogin, raw)
	assertClassifiedLocalStateError(t, mapped, OperationCompleteLogin, LocalStateKindUnavailable, "local state is unavailable", "operation-secret", "syscall-cause-secret")
}

func TestLocalStateClassifiesWrappedErrnoUnavailable(t *testing.T) {
	t.Parallel()
	raw := markLocalState(localStateStageAuth, fmt.Errorf("errno-wrapper-secret: %w", syscall.Errno(12345)))
	mapped := localSnapshotError(OperationRemoveAccount, raw)
	assertClassifiedLocalStateError(t, mapped, OperationRemoveAccount, LocalStateKindUnavailable, "local state is unavailable", "errno-wrapper-secret", "unknown error 12345")
}

func TestLocalStateClassifiesPathResolutionUnavailable(t *testing.T) {
	t.Parallel()
	mapped := localSnapshotError(OperationConfigGet, markLocalState(localStateStagePath, errors.New("home-resolution-secret")))
	assertClassifiedLocalStateError(t, mapped, OperationConfigGet, LocalStateKindUnavailable, "local state is unavailable", "home-resolution-secret")
}

func TestResourceErrorForOperationPreservesLocalStateKind(t *testing.T) {
	t.Parallel()
	source := localSnapshotError(OperationOpenResource, markLocalState(localStateStageConfig, errors.New("download-remap-secret")))
	mapped := resourceErrorForOperation(source, OperationDownload)
	assertClassifiedLocalStateError(t, mapped, OperationDownload, LocalStateKindConfigMalformed, "local configuration is malformed", "download-remap-secret")
}

func assertClassifiedLocalStateError(t *testing.T, mapped error, operation Operation, kind LocalStateKind, safeCause string, canaries ...string) {
	t.Helper()
	var typed *Error
	if !errors.As(mapped, &typed) {
		t.Fatalf("error type = %T, want *Error", mapped)
	}
	if typed.Code != CodeInvalidArgument || typed.Operation != operation || typed.Backend != "" || typed.UserID != 0 || typed.Retryable || typed.LocalStateKind != kind {
		t.Fatalf("mapped error = %#v", typed)
	}
	cause := errors.Unwrap(mapped)
	if cause == nil || cause.Error() != safeCause {
		t.Fatalf("mapped cause = %v", cause)
	}
	for _, rendered := range []string{mapped.Error(), cause.Error(), fmt.Sprintf("%+v", typed)} {
		for _, canary := range canaries {
			if strings.Contains(rendered, canary) {
				t.Fatalf("local state canary %q leaked: %q", canary, rendered)
			}
		}
	}
}
