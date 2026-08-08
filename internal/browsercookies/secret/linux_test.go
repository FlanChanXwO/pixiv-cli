//go:build linux

package secret

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSecretServiceUsesFixedLookupAndRedactsCommandFailure(t *testing.T) {
	command := filepath.Join(t.TempDir(), "secret-tool")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nprintf '%s\\n' 'fixture-password'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	password, err := (SecretService{command: command}).GetPassword(context.Background(), "chrome")
	if err != nil {
		t.Fatal(err)
	}
	if string(password) != "fixture-password" {
		t.Fatalf("password = %q, want fixture-password", password)
	}

	failing := filepath.Join(t.TempDir(), "secret-tool")
	if err := os.WriteFile(failing, []byte("#!/bin/sh\nprintf '%s' 'secret-output' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err = (SecretService{command: failing}).GetPassword(context.Background(), "chrome")
	if !errors.Is(err, ErrSecretService) {
		t.Fatalf("err = %v, want ErrSecretService", err)
	}
	if err.Error() == "secret-output" {
		t.Fatal("command output leaked into error")
	}
}

func TestSecretServiceRejectsUnknownApplication(t *testing.T) {
	_, err := (SecretService{command: "/does/not/run"}).GetPassword(context.Background(), "unknown")
	if !errors.Is(err, ErrInvalidItem) {
		t.Fatalf("err = %v, want ErrInvalidItem", err)
	}
}
