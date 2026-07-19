package e2e

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPixivBinarySyntheticAuthTransfer(t *testing.T) {
	fixture := newSyntheticAuthBinaryFixture(t)

	t.Run("positional direct import uses synthetic OAuth and persists rotation", func(t *testing.T) {
		// Go 在 Darwin/Windows 上通过系统 API 校验证书，不接受测试进程提供的
		// SSL_CERT_FILE；不能为 e2e 降低 TLS 校验或修改用户 trust store。
		if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
			t.Skip("synthetic OAuth CA requires SSL_CERT_FILE support from the platform verifier")
		}
		const inputToken = "synthetic-direct-input-secret"
		const rotatedToken = "synthetic-direct-rotated-secret"
		const accessToken = "synthetic-direct-access-secret"
		const userID = int64(303)
		directFixture := fixture.newDirectImportFixture(t, inputToken, rotatedToken, accessToken)
		oauth := newSyntheticOAuthProxy(t, rotatedToken, accessToken, userID)
		env := oauth.trustedEnv(directFixture.env)

		textResult := directFixture.runWithEnv(t, env, nil, "auth", "import", inputToken, "--proxy", oauth.proxyURL())
		directFixture.requireSuccess(t, textResult)
		directFixture.requireNoSecrets(t, textResult.stdout, "direct import text stdout")
		directFixture.requireNoSecrets(t, textResult.stderr, "direct import text stderr")
		directFixture.requireEmpty(t, textResult.stderr, "direct import text stderr")
		if want := []byte("added uid:303\nusername:synthetic-import-user\n"); !bytes.Equal(textResult.stdout, want) {
			t.Fatalf("direct import text report mismatch: got bytes=%d; want bytes=%d", len(textResult.stdout), len(want))
		}

		jsonResult := directFixture.runWithEnv(t, env, nil, "auth", "import", inputToken, "--proxy", oauth.proxyURL(), "--json")
		directFixture.requireSuccess(t, jsonResult)
		directFixture.requireNoSecrets(t, jsonResult.stdout, "direct import JSON stdout")
		directFixture.requireNoSecrets(t, jsonResult.stderr, "direct import JSON stderr")
		directFixture.requireEmpty(t, jsonResult.stderr, "direct import JSON stderr")
		var report struct {
			UserID   int64  `json:"user_id"`
			Username string `json:"username"`
			Status   string `json:"status"`
		}
		if err := json.Unmarshal(jsonResult.stdout, &report); err != nil {
			t.Fatalf("decode direct import JSON report: %v; body omitted", err)
		}
		if report.UserID != userID || report.Username != "synthetic-import-user" || report.Status != "updated" {
			t.Fatalf("direct import JSON report mismatch: uid=%d username_bytes=%d status=%q", report.UserID, len(report.Username), report.Status)
		}

		oauth.requireReceivedToken(t, inputToken, 2)
		directFixture.requireOnlyStoredToken(t, directFixture.authPath, userID, rotatedToken)
	})

	t.Run("removed direct-token entries fail and stay absent from auth help", func(t *testing.T) {
		const addToken = "synthetic-removed-add-secret"
		const flagToken = "synthetic-removed-flag-secret"
		fixture.secrets = append(fixture.secrets, []byte(addToken), []byte(flagToken))
		for name, args := range map[string][]string{
			"auth add":          {"auth", "add", addToken},
			"auth token":        {"auth", "token"},
			"auth import token": {"auth", "import", "--token", flagToken},
		} {
			result := fixture.run(t, nil, args...)
			fixture.requireFailure(t, result, name)
			fixture.requireNoSecrets(t, result.stdout, name+" stdout")
			fixture.requireNoSecrets(t, result.stderr, name+" stderr")
		}

		help := fixture.run(t, nil, "auth", "--help")
		fixture.requireSuccess(t, help)
		fixture.requireNoSecrets(t, help.stdout, "auth help stdout")
		fixture.requireNoSecrets(t, help.stderr, "auth help stderr")
		fixture.requireEmpty(t, help.stderr, "auth help stderr")
		for _, line := range strings.Split(string(help.stdout), "\n") {
			command := strings.TrimSpace(line)
			if strings.HasPrefix(command, "add ") || strings.HasPrefix(command, "token ") || strings.HasPrefix(command, "--token") {
				t.Fatalf("auth help listed a removed entry: line bytes=%d", len(line))
			}
		}
	})

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

func TestSyntheticAuthProcessEnvDisablesAutomaticUpdateBeforeFirstCommand(t *testing.T) {
	env := newSyntheticAuthProcessEnv(t)
	configRoot := env.configRoot
	if runtime.GOOS == "darwin" {
		configRoot = filepath.Join(env.home, "Library", "Application Support")
	}
	configPath := filepath.Join(configRoot, "pixiv", "config.toml")
	body, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read synthetic auth config: %v", err)
	}
	if want := []byte("[update]\ncheck_enabled = false\n"); !bytes.Equal(body, want) {
		t.Fatalf("synthetic auth config mismatch: got bytes=%d; want bytes=%d", len(body), len(want))
	}
}
