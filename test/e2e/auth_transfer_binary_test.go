package e2e

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPixivBinarySyntheticAuthTransfer(t *testing.T) {
	fixture := newSyntheticAuthBinaryFixture(t)

	t.Run("default and explicit export print exact raw tokens", func(t *testing.T) {
		defaultResult := fixture.run(t, nil, "auth", "export")
		fixture.requireSuccess(t, defaultResult)
		fixture.requireExactSecretOutput(t, defaultResult.stdout, []byte(fixture.defaultToken+"\n"), "default export")
		fixture.requireEmpty(t, defaultResult.stderr, "default export stderr")

		explicitResult := fixture.run(t, nil, "auth", "export", "202")
		fixture.requireSuccess(t, explicitResult)
		fixture.requireExactSecretOutput(t, explicitResult.stdout, []byte(fixture.explicitToken+"\n"), "explicit export")
		fixture.requireEmpty(t, explicitResult.stderr, "explicit export stderr")
	})

	t.Run("all export prints a decodable versioned bundle", func(t *testing.T) {
		result := fixture.run(t, nil, "auth", "export", "--all")
		fixture.requireSuccess(t, result)
		fixture.requireEmpty(t, result.stderr, "all export stderr")
		fixture.requireAllBundle(t, result.stdout)
	})

	t.Run("output is a safe bundle summary with no clobber unless forced", func(t *testing.T) {
		outputPath := filepath.Join(t.TempDir(), "auth-export.json")
		created := fixture.run(t, nil, "auth", "export", "--output", outputPath)
		fixture.requireSuccess(t, created)
		fixture.requireSafeOutputSummary(t, created, outputPath, 1)
		fixture.requireSingleBundleFile(t, outputPath, 101, fixture.defaultToken)

		const oldBody = "existing destination marker"
		if err := os.WriteFile(outputPath, []byte(oldBody), 0o600); err != nil {
			t.Fatalf("prepare existing export destination: %v", err)
		}
		rejected := fixture.run(t, nil, "auth", "export", "--output", outputPath)
		fixture.requireFailure(t, rejected, "existing output")
		fixture.requireNoSecrets(t, rejected.stdout, "existing output stdout")
		fixture.requireNoSecrets(t, rejected.stderr, "existing output stderr")
		preserved, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("read rejected export destination: %v", err)
		}
		if !bytes.Equal(preserved, []byte(oldBody)) {
			t.Fatalf("existing export destination changed: bytes=%d", len(preserved))
		}

		forced := fixture.run(t, nil, "auth", "export", "--output", outputPath, "--force")
		fixture.requireSuccess(t, forced)
		fixture.requireSafeOutputSummary(t, forced, outputPath, 1)
		fixture.requireSingleBundleFile(t, outputPath, 101, fixture.defaultToken)
	})

	t.Run("file import restores into another isolated store offline", func(t *testing.T) {
		bundleResult := fixture.run(t, nil, "auth", "export", "--all")
		fixture.requireSuccess(t, bundleResult)
		bundlePath := filepath.Join(t.TempDir(), "transfer.json")
		if err := os.WriteFile(bundlePath, bundleResult.stdout, 0o600); err != nil {
			t.Fatalf("write synthetic transfer bundle: %v", err)
		}

		destination := fixture.newOfflineEnv(t)
		restored := fixture.runWithEnv(t, destination, nil, "auth", "import", "--file", bundlePath)
		fixture.requireSuccess(t, restored)
		fixture.requireSafeRestoreReport(t, restored)
		fixture.requireRestoredAccountList(t, destination)
	})

	t.Run("stdin import restores into another isolated store offline", func(t *testing.T) {
		bundleResult := fixture.run(t, nil, "auth", "export", "--all")
		fixture.requireSuccess(t, bundleResult)
		destination := fixture.newOfflineEnv(t)

		restored := fixture.runWithEnv(t, destination, bundleResult.stdout, "auth", "import", "--file", "-")
		fixture.requireSuccess(t, restored)
		fixture.requireSafeRestoreReport(t, restored)
		fixture.requireRestoredAccountList(t, destination)
	})

	t.Run("legacy token command is unknown", func(t *testing.T) {
		result := fixture.run(t, nil, "auth", "token")
		fixture.requireFailure(t, result, "legacy token command")
		fixture.requireEmpty(t, result.stdout, "legacy token stdout")
		fixture.requireNoSecrets(t, result.stderr, "legacy token stderr")
		if len(result.stderr) == 0 {
			t.Fatal("legacy token command failure did not write a safe diagnostic")
		}
	})
}
