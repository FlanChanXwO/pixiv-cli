package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/platform/localstate"
	config "github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
)

func TestAuthDefaultUserIDRoundTrip(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	restore := localstate.SetConfigFilePathForTest(configPath)
	defer restore()
	t.Cleanup(func() { _ = os.Remove(configPath) })

	if _, ok, err := config.ReadPixivDefaultUserID(); err != nil || ok {
		t.Fatalf("initial pixiv default: ok=%v err=%v", ok, err)
	}
	if err := config.SetPixivDefaultUserID(123); err != nil {
		t.Fatalf("set pixiv: %v", err)
	}
	if userID, ok, err := config.ReadPixivDefaultUserID(); err != nil || !ok || userID != 123 {
		t.Fatalf("read pixiv = %d, %v, %v", userID, ok, err)
	}
	if err := config.ClearPixivDefaultUserID(); err != nil {
		t.Fatalf("clear pixiv: %v", err)
	}
	if _, ok, err := config.ReadPixivDefaultUserID(); err != nil || ok {
		t.Fatalf("after clear: ok=%v err=%v", ok, err)
	}

	if err := config.SetFanboxDefaultUserID(456); err != nil {
		t.Fatalf("set fanbox: %v", err)
	}
	if userID, ok, err := config.ReadFanboxDefaultUserID(); err != nil || !ok || userID != 456 {
		t.Fatalf("read fanbox = %d, %v, %v", userID, ok, err)
	}
	if err := config.ClearFanboxDefaultUserID(); err != nil {
		t.Fatalf("clear fanbox: %v", err)
	}
}

func TestAuthDefaultUserIDRejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	restore := localstate.SetConfigFilePathForTest(filepath.Join(dir, "config.toml"))
	defer restore()
	if err := config.SetPixivDefaultUserID(0); err == nil {
		t.Fatal("zero user id should be rejected")
	}
	if err := config.SetPixivDefaultUserID(-5); err == nil {
		t.Fatal("negative user id should be rejected")
	}
}
