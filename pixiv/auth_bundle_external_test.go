package pixiv_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	storageauth "github.com/FlanChanXwO/pixiv-cli/internal/storage/auth"
	storagefiles "github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestExportAuthBundleSnapshotsDefaultAccountAndRoundTripsExactCodec(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	const storedAuth = `{"default_user_id":7,"accounts":[{"user_id":7,"username":"alice","refresh_token":"opaque/default-secret"},{"user_id":8,"username":"bob","refresh_token":"opaque/other-secret"}]}`
	if err := os.WriteFile(authPath, []byte(storedAuth), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := pixiv.NewClient(pixiv.Options{AuthFilePath: authPath})
	if err != nil {
		t.Fatal(err)
	}

	bundle, err := client.ExportAuthBundle(pixiv.AuthExportSelection{})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := pixiv.EncodeAuthExportBundle(bundle)
	if err != nil {
		t.Fatal(err)
	}
	const wantWire = `{
  "schema": "pixiv-cli.auth-export",
  "version": 1,
  "default_user_id": 7,
  "accounts": [
    {
      "user_id": 7,
      "username": "alice",
      "refresh_token": "opaque/default-secret"
    }
  ]
}
`
	if string(wire) != wantWire {
		t.Fatalf("encoded bundle:\n%s", wire)
	}

	decoded, err := pixiv.DecodeAuthExportBundle(wire)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := pixiv.EncodeAuthExportBundle(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(roundTrip) != wantWire {
		t.Fatalf("round-trip bundle:\n%s", roundTrip)
	}
	if got, err := os.ReadFile(authPath); err != nil || string(got) != storedAuth {
		t.Fatalf("auth store changed: %s err=%v", got, err)
	}
}

func TestExportAuthBundleSnapshotsAllAccountsInStoreOrder(t *testing.T) {
	t.Parallel()
	authPath := filepath.Join(t.TempDir(), "auth.json")
	const storedAuth = `{"default_user_id":8,"accounts":[{"user_id":7,"username":"alice","refresh_token":"opaque/first-secret"},{"user_id":8,"username":"bob","refresh_token":"opaque/default-secret"}]}`
	if err := os.WriteFile(authPath, []byte(storedAuth), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := pixiv.NewClient(pixiv.Options{AuthFilePath: authPath})
	if err != nil {
		t.Fatal(err)
	}

	bundle, err := client.ExportAuthBundle(pixiv.AuthExportSelection{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.DefaultUserID != 8 || len(bundle.Accounts) != 2 {
		t.Fatalf("bundle=%+v", bundle)
	}
	if bundle.Accounts[0].UserID != 7 || bundle.Accounts[0].RefreshToken != "opaque/first-secret" || bundle.Accounts[1].UserID != 8 || bundle.Accounts[1].RefreshToken != "opaque/default-secret" {
		t.Fatalf("accounts=%+v", bundle.Accounts)
	}
}

func TestExportAuthBundleSnapshotsOnlyExplicitAccount(t *testing.T) {
	t.Parallel()
	authPath := filepath.Join(t.TempDir(), "auth.json")
	const storedAuth = `{"default_user_id":7,"accounts":[{"user_id":7,"username":"default","refresh_token":"default-secret"},{"user_id":8,"username":"explicit","refresh_token":"explicit-secret"}]}`
	if err := os.WriteFile(authPath, []byte(storedAuth), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := pixiv.NewClient(pixiv.Options{AuthFilePath: authPath})
	if err != nil {
		t.Fatal(err)
	}

	bundle, err := client.ExportAuthBundle(pixiv.AuthExportSelection{UserID: 8})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.DefaultUserID != 8 || len(bundle.Accounts) != 1 {
		t.Fatalf("bundle=%+v", bundle)
	}
	account := bundle.Accounts[0]
	if account.UserID != 8 || account.Username != "explicit" || account.RefreshToken != "explicit-secret" {
		t.Fatalf("account=%+v", account)
	}
	if got, readErr := os.ReadFile(authPath); readErr != nil || string(got) != storedAuth {
		t.Fatalf("source changed: %s err=%v", got, readErr)
	}
}

func TestExportAuthBundleRejectsConflictingAllAndExplicitSelection(t *testing.T) {
	t.Parallel()
	client, err := pixiv.NewClient(pixiv.Options{AuthFilePath: filepath.Join(t.TempDir(), "auth.json")})
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ExportAuthBundle(pixiv.AuthExportSelection{UserID: 7, All: true})
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Code != pixiv.CodeInvalidArgument || typed.Operation != pixiv.OperationExportAuthBundle {
		t.Fatalf("error=%#v", err)
	}
}

func TestExportAuthBundleReportsMissingExplicitAccount(t *testing.T) {
	t.Parallel()
	client, err := pixiv.NewClient(pixiv.Options{AuthFilePath: filepath.Join(t.TempDir(), "auth.json")})
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ExportAuthBundle(pixiv.AuthExportSelection{UserID: 99})
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Code != pixiv.CodeInvalidArgument || typed.Operation != pixiv.OperationExportAuthBundle || typed.UserID != 99 {
		t.Fatalf("error=%#v", err)
	}
}

func TestExportAuthBundleRejectsNonPositiveExplicitUIDBeforeStoreAccess(t *testing.T) {
	t.Parallel()
	client, err := pixiv.NewClient(pixiv.Options{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ExportAuthBundle(pixiv.AuthExportSelection{UserID: -1})
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Code != pixiv.CodeInvalidArgument || typed.Operation != pixiv.OperationExportAuthBundle {
		t.Fatalf("error=%#v", err)
	}
	if cause := errors.Unwrap(err); cause == nil || cause.Error() != "user id must be positive" {
		t.Fatalf("cause=%v", cause)
	}
}

func TestRestoreAuthBundleMergesAccountsAndPreservesExistingDefault(t *testing.T) {
	t.Parallel()
	authPath := filepath.Join(t.TempDir(), "auth.json")
	const storedAuth = `{"default_user_id":10,"accounts":[{"user_id":10,"username":"kept","refresh_token":"kept-secret"},{"user_id":7,"username":"old","refresh_token":"old-secret"}]}`
	if err := os.WriteFile(authPath, []byte(storedAuth), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := pixiv.NewClient(pixiv.Options{AuthFilePath: authPath})
	if err != nil {
		t.Fatal(err)
	}
	bundle := &pixiv.AuthExportBundle{
		Schema:        pixiv.AuthExportBundleSchema,
		Version:       pixiv.AuthExportBundleVersion,
		DefaultUserID: 7,
		Accounts: []pixiv.AuthExportSecretAccount{
			{UserID: 7, Username: "updated", RefreshToken: "updated-secret"},
			{UserID: 8, Username: "added", RefreshToken: "added-secret"},
		},
	}

	result, err := client.RestoreAuthBundle(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if result.DefaultUserID != 10 || len(result.Updated) != 1 || result.Updated[0].UserID != 7 || len(result.Added) != 1 || result.Added[0].UserID != 8 {
		t.Fatalf("result=%+v", result)
	}
	listed, err := client.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if listed.DefaultUserID != 10 || len(listed.Accounts) != 3 || listed.Accounts[0].UserID != 10 || listed.Accounts[1].UserID != 7 || listed.Accounts[2].UserID != 8 {
		t.Fatalf("accounts=%+v", listed)
	}
	for userID, want := range map[int64]string{7: "updated-secret", 8: "added-secret", 10: "kept-secret"} {
		got, exportErr := client.ExportAccountRefreshToken(userID)
		if exportErr != nil || got != want {
			t.Fatalf("user %d token=%q err=%v", userID, got, exportErr)
		}
	}
}

func TestRestoreAuthBundleUsesSourceDefaultAndReportsRepeatAsUpdates(t *testing.T) {
	t.Parallel()
	authPath := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"accounts":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := pixiv.NewClient(pixiv.Options{AuthFilePath: authPath})
	if err != nil {
		t.Fatal(err)
	}
	bundle := &pixiv.AuthExportBundle{
		Schema:        pixiv.AuthExportBundleSchema,
		Version:       pixiv.AuthExportBundleVersion,
		DefaultUserID: 8,
		Accounts: []pixiv.AuthExportSecretAccount{
			{UserID: 7, Username: "first", RefreshToken: "first-secret"},
			{UserID: 8, Username: "default", RefreshToken: "default-secret"},
		},
	}

	first, err := client.RestoreAuthBundle(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if first.DefaultUserID != 8 || len(first.Added) != 2 || len(first.Updated) != 0 || first.Added[0].UserID != 7 || first.Added[1].UserID != 8 || !first.Added[1].Default {
		t.Fatalf("first=%+v", first)
	}
	second, err := client.RestoreAuthBundle(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if second.DefaultUserID != 8 || len(second.Added) != 0 || len(second.Updated) != 2 || second.Updated[0].UserID != 7 || second.Updated[1].UserID != 8 {
		t.Fatalf("second=%+v", second)
	}
	wire, err := json.Marshal([]*pixiv.AuthRestoreResult{first, second})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"first-secret", "default-secret"} {
		if strings.Contains(string(wire), secret) {
			t.Fatalf("restore result exposed %q", secret)
		}
	}
}

func TestAuthBundleOperationsAreOfflineAndIgnoreRuntimeCredentialOverrides(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "config.toml")
	const storedAuth = `{"default_user_id":7,"accounts":[{"user_id":7,"refresh_token":"stored-secret"}]}`
	if err := os.WriteFile(authPath, []byte(storedAuth), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("malformed = [toml"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIXIV_REFRESH_TOKEN", "environment-secret")
	var requests atomic.Int32
	client, err := pixiv.OpenDefault(pixiv.Options{
		AuthFilePath: authPath, ConfigFilePath: configPath, UserID: 99, RefreshToken: "option-secret",
		HTTPClient: &http.Client{Transport: accountRoundTripperFunc(func(*http.Request) (*http.Response, error) {
			requests.Add(1)
			return nil, errors.New("network must not be used")
		})},
	})
	if err != nil {
		t.Fatal(err)
	}

	bundle, err := client.ExportAuthBundle(pixiv.AuthExportSelection{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Accounts) != 1 || bundle.Accounts[0].RefreshToken != "stored-secret" {
		t.Fatalf("bundle=%+v", bundle)
	}
	if _, err := client.RestoreAuthBundle(bundle); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatalf("network requests=%d", requests.Load())
	}
}

func TestRestoreAuthBundleSaveFailureLeavesDestinationBytesUnchanged(t *testing.T) {
	authPath := filepath.Join(t.TempDir(), "auth.json")
	const storedAuth = `{"default_user_id":10,"accounts":[{"user_id":10,"refresh_token":"destination-secret"}]}`
	if err := os.WriteFile(authPath, []byte(storedAuth), 0o600); err != nil {
		t.Fatal(err)
	}
	restoreHook := storageauth.SetWriteAuthStoreFileForTest(authPath, func(path string, _ []byte) error {
		return &fs.PathError{Op: "write", Path: path, Err: fs.ErrPermission}
	})
	t.Cleanup(restoreHook)
	client, err := pixiv.NewClient(pixiv.Options{AuthFilePath: authPath})
	if err != nil {
		t.Fatal(err)
	}
	bundle := &pixiv.AuthExportBundle{
		Schema:        pixiv.AuthExportBundleSchema,
		Version:       pixiv.AuthExportBundleVersion,
		DefaultUserID: 7,
		Accounts:      []pixiv.AuthExportSecretAccount{{UserID: 7, RefreshToken: "bundle-secret"}},
	}

	_, err = client.RestoreAuthBundle(bundle)
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Operation != pixiv.OperationRestoreAuthBundle || typed.LocalStateKind != pixiv.LocalStateKindPermissionDenied || typed.LocalWriteCommitOutcome != pixiv.LocalWriteCommitOutcomeNotCommitted {
		t.Fatalf("error=%#v", err)
	}
	if got, readErr := os.ReadFile(authPath); readErr != nil || string(got) != storedAuth {
		t.Fatalf("destination changed: %s err=%v", got, readErr)
	}
}

func TestRestoreAuthBundleReportsCommittedAfterPostCommitSyncFailure(t *testing.T) {
	authPath := filepath.Join(t.TempDir(), "auth.json")
	const storedAuth = `{"default_user_id":10,"accounts":[{"user_id":10,"refresh_token":"destination-secret"}]}`
	if err := os.WriteFile(authPath, []byte(storedAuth), 0o600); err != nil {
		t.Fatal(err)
	}
	restoreHook := storageauth.SetWriteAuthStoreFileForTest(authPath, func(path string, body []byte) error {
		return storagefiles.WritePrivateFileWithSyncParentForTest(path, body, storageauth.DefaultAuthFileMode, func(string) error {
			return errors.New("parent sync failed")
		})
	})
	t.Cleanup(restoreHook)
	client, err := pixiv.NewClient(pixiv.Options{AuthFilePath: authPath})
	if err != nil {
		t.Fatal(err)
	}
	bundle := &pixiv.AuthExportBundle{
		Schema:        pixiv.AuthExportBundleSchema,
		Version:       pixiv.AuthExportBundleVersion,
		DefaultUserID: 7,
		Accounts:      []pixiv.AuthExportSecretAccount{{UserID: 7, RefreshToken: "bundle-secret"}},
	}

	_, err = client.RestoreAuthBundle(bundle)
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Operation != pixiv.OperationRestoreAuthBundle || typed.LocalWriteCommitOutcome != pixiv.LocalWriteCommitOutcomeCommitted {
		t.Fatalf("error=%#v", err)
	}
	if !strings.Contains(err.Error(), "local_write_commit_outcome=committed") {
		t.Fatalf("diagnostic=%q", err.Error())
	}
	token, exportErr := client.ExportAccountRefreshToken(7)
	if exportErr != nil || token != "bundle-secret" {
		t.Fatalf("committed token=%q err=%v", token, exportErr)
	}
}

func TestRestoreAuthBundleRejectsUnknownSchemaWithoutChangingDestination(t *testing.T) {
	t.Parallel()
	authPath := filepath.Join(t.TempDir(), "auth.json")
	const storedAuth = `{"default_user_id":10,"accounts":[{"user_id":10,"refresh_token":"destination-secret"}]}`
	if err := os.WriteFile(authPath, []byte(storedAuth), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := pixiv.NewClient(pixiv.Options{AuthFilePath: authPath})
	if err != nil {
		t.Fatal(err)
	}
	bundle := &pixiv.AuthExportBundle{
		Schema:        "future-schema-source-secret",
		Version:       pixiv.AuthExportBundleVersion,
		DefaultUserID: 7,
		Accounts:      []pixiv.AuthExportSecretAccount{{UserID: 7, RefreshToken: "bundle-secret"}},
	}

	_, err = client.RestoreAuthBundle(bundle)
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Code != pixiv.CodeInvalidArgument || typed.Operation != pixiv.OperationRestoreAuthBundle {
		t.Fatalf("error=%#v", err)
	}
	for _, secret := range []string{"future-schema-source-secret", "bundle-secret", "destination-secret"} {
		if strings.Contains(err.Error(), secret) || strings.Contains(errorCause(err), secret) {
			t.Fatalf("secret exposed in error: %q", secret)
		}
	}
	if got, readErr := os.ReadFile(authPath); readErr != nil || string(got) != storedAuth {
		t.Fatalf("destination changed: %s err=%v", got, readErr)
	}
}

func TestDecodeAuthExportBundleRejectsFutureVersion(t *testing.T) {
	t.Parallel()
	const body = `{"schema":"pixiv-cli.auth-export","version":2,"default_user_id":7,"accounts":[{"user_id":7,"username":"alice","refresh_token":"future-version-secret"}]}`

	_, err := pixiv.DecodeAuthExportBundle([]byte(body))
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Code != pixiv.CodeInvalidArgument || typed.Operation != pixiv.OperationDecodeAuthBundle {
		t.Fatalf("error=%#v", err)
	}
	if strings.Contains(err.Error(), "future-version-secret") || strings.Contains(errorCause(err), "future-version-secret") {
		t.Fatal("decode error exposed secret")
	}
}

func TestDecodeAuthExportBundleRejectsUnknownFieldsWithoutExposingThem(t *testing.T) {
	t.Parallel()
	const body = `{"schema":"pixiv-cli.auth-export","version":1,"default_user_id":7,"accounts":[{"user_id":7,"username":"alice","refresh_token":"bundle-secret"}],"unknown-source-secret":"private-content"}`

	_, err := pixiv.DecodeAuthExportBundle([]byte(body))
	var typed *pixiv.Error
	if !errors.As(err, &typed) || typed.Code != pixiv.CodeInvalidArgument || typed.Operation != pixiv.OperationDecodeAuthBundle {
		t.Fatalf("error=%#v", err)
	}
	for _, secret := range []string{"unknown-source-secret", "private-content", "bundle-secret"} {
		if strings.Contains(err.Error(), secret) || strings.Contains(errorCause(err), secret) {
			t.Fatalf("decode error exposed %q", secret)
		}
	}
}

func TestDecodeAuthExportBundleRejectsDuplicateObjectKeysRecursively(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"top-level schema":  `{"schema":"pixiv-cli.auth-export","schema":"pixiv-cli.auth-export","version":1,"default_user_id":7,"accounts":[{"user_id":7,"username":"alice","refresh_token":"bundle-secret"}]}`,
		"top-level version": `{"schema":"pixiv-cli.auth-export","version":1,"version":1,"default_user_id":7,"accounts":[{"user_id":7,"username":"alice","refresh_token":"bundle-secret"}]}`,
		"top-level default": `{"schema":"pixiv-cli.auth-export","version":1,"default_user_id":7,"default_user_id":7,"accounts":[{"user_id":7,"username":"alice","refresh_token":"bundle-secret"}]}`,
		"account user id":   `{"schema":"pixiv-cli.auth-export","version":1,"default_user_id":7,"accounts":[{"user_id":7,"user_id":7,"username":"alice","refresh_token":"bundle-secret"}]}`,
		"account token":     `{"schema":"pixiv-cli.auth-export","version":1,"default_user_id":7,"accounts":[{"user_id":7,"username":"alice","refresh_token":"bundle-secret","refresh_token":"replacement-secret"}]}`,
	}
	for name, body := range cases {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bundle, err := pixiv.DecodeAuthExportBundle([]byte(body))
			var typed *pixiv.Error
			if bundle != nil || !errors.As(err, &typed) || typed.Code != pixiv.CodeInvalidArgument || typed.Operation != pixiv.OperationDecodeAuthBundle {
				t.Fatalf("bundle=%+v error=%#v", bundle, err)
			}
			for _, secret := range []string{"bundle-secret", "replacement-secret"} {
				if strings.Contains(err.Error(), secret) || strings.Contains(errorCause(err), secret) {
					t.Fatalf("decode error exposed %q", secret)
				}
			}
		})
	}
}

func TestEncodeAuthExportBundleRejectsInvalidAccountGraph(t *testing.T) {
	t.Parallel()
	valid := func() *pixiv.AuthExportBundle {
		return &pixiv.AuthExportBundle{
			Schema:        pixiv.AuthExportBundleSchema,
			Version:       pixiv.AuthExportBundleVersion,
			DefaultUserID: 7,
			Accounts:      []pixiv.AuthExportSecretAccount{{UserID: 7, Username: "alice", RefreshToken: "bundle-secret"}},
		}
	}
	cases := map[string]*pixiv.AuthExportBundle{
		"nil bundle":           nil,
		"empty accounts":       {Schema: pixiv.AuthExportBundleSchema, Version: pixiv.AuthExportBundleVersion, DefaultUserID: 0, Accounts: []pixiv.AuthExportSecretAccount{}},
		"non-positive uid":     {Schema: pixiv.AuthExportBundleSchema, Version: pixiv.AuthExportBundleVersion, DefaultUserID: -1, Accounts: []pixiv.AuthExportSecretAccount{{UserID: -1, RefreshToken: "bundle-secret"}}},
		"duplicate uid":        {Schema: pixiv.AuthExportBundleSchema, Version: pixiv.AuthExportBundleVersion, DefaultUserID: 7, Accounts: []pixiv.AuthExportSecretAccount{{UserID: 7, RefreshToken: "bundle-secret"}, {UserID: 7, RefreshToken: "other-secret"}}},
		"empty token":          {Schema: pixiv.AuthExportBundleSchema, Version: pixiv.AuthExportBundleVersion, DefaultUserID: 7, Accounts: []pixiv.AuthExportSecretAccount{{UserID: 7, RefreshToken: " \t"}}},
		"missing default":      {Schema: pixiv.AuthExportBundleSchema, Version: pixiv.AuthExportBundleVersion, Accounts: []pixiv.AuthExportSecretAccount{{UserID: 7, RefreshToken: "bundle-secret"}}},
		"unreferenced default": valid(),
	}
	cases["unreferenced default"].DefaultUserID = 8

	for name, bundle := range cases {
		name, bundle := name, bundle
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			wire, err := pixiv.EncodeAuthExportBundle(bundle)
			var typed *pixiv.Error
			if wire != nil || !errors.As(err, &typed) || typed.Code != pixiv.CodeInvalidArgument || typed.Operation != pixiv.OperationEncodeAuthBundle {
				t.Fatalf("wire=%q error=%#v", wire, err)
			}
			for _, secret := range []string{"bundle-secret", "other-secret"} {
				if strings.Contains(err.Error(), secret) || strings.Contains(errorCause(err), secret) {
					t.Fatalf("encode error exposed %q", secret)
				}
			}
		})
	}
}

func errorCause(err error) string {
	if cause := errors.Unwrap(err); cause != nil {
		return cause.Error()
	}
	return ""
}
