package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAuthDefaultUserIDRoundTrip(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	restore := SetFilePathForTest(configPath)
	defer restore()
	t.Cleanup(func() { _ = os.Remove(configPath) })

	if _, ok, err := ReadPixivDefaultUserID(); err != nil || ok {
		t.Fatalf("initial pixiv default: ok=%v err=%v", ok, err)
	}
	if err := SetPixivDefaultUserID(123); err != nil {
		t.Fatalf("set pixiv: %v", err)
	}
	if userID, ok, err := ReadPixivDefaultUserID(); err != nil || !ok || userID != 123 {
		t.Fatalf("read pixiv = %d, %v, %v", userID, ok, err)
	}
	if err := ClearPixivDefaultUserID(); err != nil {
		t.Fatalf("clear pixiv: %v", err)
	}
	if _, ok, err := ReadPixivDefaultUserID(); err != nil || ok {
		t.Fatalf("after clear: ok=%v err=%v", ok, err)
	}

	if err := SetFanboxDefaultUserID(456); err != nil {
		t.Fatalf("set fanbox: %v", err)
	}
	if userID, ok, err := ReadFanboxDefaultUserID(); err != nil || !ok || userID != 456 {
		t.Fatalf("read fanbox = %d, %v, %v", userID, ok, err)
	}
	if err := ClearFanboxDefaultUserID(); err != nil {
		t.Fatalf("clear fanbox: %v", err)
	}
}

func TestAuthDefaultUserIDRejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	restore := SetFilePathForTest(filepath.Join(dir, "config.toml"))
	defer restore()
	if err := SetPixivDefaultUserID(0); err == nil {
		t.Fatal("zero user id should be rejected")
	}
	if err := SetPixivDefaultUserID(-5); err == nil {
		t.Fatal("negative user id should be rejected")
	}
}
