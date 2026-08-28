package installer

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"

	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/update/release"

	"github.com/FlanChanXwO/pixiv-cli/internal/update/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

// Release 与 ReleaseAsset 是 installer 包消费的 release 值类型别名，保持既有测试字面量不变。
type Release = release.Release

type ReleaseAsset = release.ReleaseAsset

const (
	testVersionOutputOverrideEnv = "PIXIV_INSTALLER_TEST_VERSION_OUTPUT_OVERRIDE"
	testVersionOutputEnv         = "PIXIV_INSTALLER_TEST_VERSION_OUTPUT"
	testVersionFailureEnv        = "PIXIV_INSTALLER_TEST_VERSION_FAILURE"
)

// TestMain 让复制到 fixture archive 的当前测试二进制可以作为真实更新二进制运行。
// 生产 checker 固定调用根 `--version`；测试环境变量仅用于覆盖子进程的错误路径。
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		if os.Getenv(testVersionFailureEnv) != "" {
			_, _ = fmt.Fprintln(os.Stderr, "fixture version failure")
			os.Exit(1)
		}
		if os.Getenv(testVersionOutputOverrideEnv) != "" {
			_, _ = fmt.Fprint(os.Stdout, os.Getenv(testVersionOutputEnv))
		} else {
			_, _ = fmt.Fprintln(os.Stdout, "pixiv v0.2.0")
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestProcessReleaseBinaryCheckerRequiresExactRootVersionOutput(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		failure   bool
		wantError string
	}{
		{name: "exact output", output: "pixiv v0.2.0\n"},
		{name: "wrong version", output: "pixiv v0.1.0\n", wantError: `reports version output "pixiv v0.1.0\n"`},
		{name: "missing newline", output: "pixiv v0.2.0", wantError: `reports version output "pixiv v0.2.0"`},
		{name: "trailing data", output: "pixiv v0.2.0\nextra\n", wantError: `reports version output "pixiv v0.2.0\nextra\n"`},
		{name: "legacy JSON", output: `{"version":"v0.2.0"}` + "\n", wantError: `reports version output "{\"version\":\"v0.2.0\"}\n"`},
		{name: "process failure", failure: true, wantError: "run --version"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(testVersionOutputOverrideEnv, "1")
			t.Setenv(testVersionOutputEnv, test.output)
			if test.failure {
				t.Setenv(testVersionFailureEnv, "1")
			}

			err := (processReleaseBinaryChecker{}).Check(context.Background(), os.Args[0], "v0.2.0")
			if test.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestVerifyArchiveChecksumAllowsFixedInstallerAssets(t *testing.T) {
	archive := []byte("verified platform archive")
	archiveSum := sha256.Sum256(archive)
	checksums := fmt.Sprintf(
		"%x  install.cmd\n%x  install.sh\n%x  pixiv-cli_1.2.3_linux_amd64.tar.gz\n",
		sha256.Sum256([]byte("cmd")),
		sha256.Sum256([]byte("sh")),
		archiveSum,
	)
	require.NoError(t, verifyArchiveChecksum([]byte(checksums), "pixiv-cli_1.2.3_linux_amd64.tar.gz", archive))
}

// TestReleaseInstallerInstallsOnlyVerifiedPlatformArchive 覆盖公开安装路径：选择当前平台资产，
// 验证带签名的 checksum manifest，再在内嵌版本证明目标 tag 后原子替换可执行文件。
func TestReleaseInstallerInstallsOnlyVerifiedPlatformArchive(t *testing.T) {
	t.Parallel()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	const keyID = "fixture-2026"
	const tag = "v0.2.0"
	archiveName := releaseArchiveName("0.2.0", runtime.GOOS, runtime.GOARCH)
	archive, executable := fixtureNativeExecutableArchive(t)
	checksum := sha256.Sum256(archive)
	checksums := []byte(hex.EncodeToString(checksum[:]) + "  " + archiveName + "\n")
	manifest := signedChecksumsManifest(t, keyID, privateKey, checksums)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + archiveName:
			_, _ = w.Write(archive)
		case "/checksums.txt":
			_, _ = w.Write(checksums)
		case "/checksums.json":
			_, _ = w.Write(manifest)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	target := filepath.Join(t.TempDir(), releaseBinaryName(runtime.GOOS))
	require.NoError(t, os.WriteFile(target, []byte("old executable"), 0o755))
	installer := NewReleaseInstaller(ReleaseInstallerOptions{
		HTTPClient:  server.Client(),
		TrustedKeys: map[string]ed25519.PublicKey{keyID: privateKey.Public().(ed25519.PublicKey)},
		ExecutablePath: func() (string, error) {
			return target, nil
		},
	}).(*releaseInstaller)
	installer.assetURLValidator = allowFixtureReleaseAssetURL

	err = installer.Install(context.Background(), Release{
		TagName: tag,
		Version: "0.2.0",
		Assets: []ReleaseAsset{
			{Name: archiveName, DownloadURL: server.URL + "/" + archiveName},
			{Name: "checksums.txt", DownloadURL: server.URL + "/checksums.txt"},
			{Name: "checksums.json", DownloadURL: server.URL + "/checksums.json"},
		},
	})
	require.NoError(t, err)

	installed, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, executable, installed)
}

// TestReleaseInstallerPreservesExecutableSymlink 确保自更新不会把用户或包管理器维护的
// 可执行文件软链接替换成普通文件；候选文件必须与其解析后的真实目标同目录 staging。
func TestReleaseInstallerPreservesExecutableSymlink(t *testing.T) {
	archive := fixtureRuntimeArchive(t, map[string]string{releaseBinaryName(runtime.GOOS): "new executable"})
	installer, release, originalTarget := verifiedFixtureInstaller(t, archive, nil)
	root := filepath.Dir(originalTarget)
	resolvedTarget := filepath.Join(root, "release", releaseBinaryName(runtime.GOOS))
	rawLink := filepath.Join(root, "bin", releaseBinaryName(runtime.GOOS))
	linkTarget := filepath.Join("..", "release", releaseBinaryName(runtime.GOOS))
	require.NoError(t, os.MkdirAll(filepath.Dir(resolvedTarget), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(rawLink), 0o755))
	require.NoError(t, os.Rename(originalTarget, resolvedTarget))
	require.NoError(t, os.Symlink(linkTarget, rawLink))
	expectedTarget, err := filepath.EvalSymlinks(rawLink)
	require.NoError(t, err)
	installer.executablePath = func() (string, error) { return rawLink, nil }
	installer.checker = releaseBinaryCheckerFunc(func(context.Context, string, string) error { return nil })

	defaultReplacer := installer.replacer
	var stagedPath, replacementTarget string
	installer.replacer = releaseFileReplacerFunc(func(source, target string) error {
		stagedPath = source
		replacementTarget = target
		return defaultReplacer.Replace(source, target)
	})

	require.NoError(t, installer.Install(context.Background(), release))
	linkInfo, err := os.Lstat(rawLink)
	require.NoError(t, err)
	require.True(t, linkInfo.Mode()&os.ModeSymlink != 0, "raw executable entry must remain a symlink")
	readLink, err := os.Readlink(rawLink)
	require.NoError(t, err)
	require.Equal(t, linkTarget, readLink)
	installed, err := os.ReadFile(expectedTarget)
	require.NoError(t, err)
	require.Equal(t, []byte("new executable"), installed)
	require.Equal(t, expectedTarget, replacementTarget)
	require.Equal(t, filepath.Dir(expectedTarget), filepath.Dir(stagedPath))
	requireNoUpdateTemporaryPaths(t, filepath.Dir(expectedTarget))
}

type preserveInstallerSourceError struct {
	err error
}

func (e preserveInstallerSourceError) Error() string { return e.err.Error() }

func (e preserveInstallerSourceError) Unwrap() error { return e.err }

func (preserveInstallerSourceError) PreserveReplacementSource() {}

func TestReleaseInstallerPreservesStagedSourceWhenReplacementRecoveryIsUnresolved(t *testing.T) {
	archive := fixtureRuntimeArchive(t, map[string]string{releaseBinaryName(runtime.GOOS): "new executable"})
	installer, release, target := verifiedFixtureInstaller(t, archive, nil)
	installer.checker = releaseBinaryCheckerFunc(func(context.Context, string, string) error { return nil })
	var stagedPath string
	replaceCause := errors.New("replacement recovery unresolved")
	installer.replacer = releaseFileReplacerFunc(func(source, _ string) error {
		stagedPath = source
		return preserveInstallerSourceError{err: replaceCause}
	})

	err := installer.Install(context.Background(), release)
	require.ErrorIs(t, err, replaceCause)

	oldBody, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	require.Equal(t, "old executable", string(oldBody))
	newBody, readErr := os.ReadFile(stagedPath)
	require.NoError(t, readErr)
	require.Equal(t, "new executable", string(newBody))
}

func TestReleaseInstallerRejectsUntrustedCachedAssetURLBeforeRequest(t *testing.T) {
	const tag = "v0.2.0"
	archiveName := releaseArchiveName("0.2.0", runtime.GOOS, runtime.GOARCH)
	assetRequests := 0
	assetServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assetRequests++
		http.Error(w, "must not receive release asset request", http.StatusTeapot)
	}))
	t.Cleanup(assetServer.Close)

	assets := []ReleaseAsset{
		{Name: archiveName, DownloadURL: "https://github.com/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + archiveName},
		{Name: checksumsAssetName, DownloadURL: assetServer.URL + "/checksums.txt"},
		{Name: manifestAssetName, DownloadURL: "https://github.com/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + manifestAssetName},
	}
	assetsJSON, err := json.Marshal(assets)
	require.NoError(t, err)
	apiRequests := 0
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiRequests++
		if apiRequests == 1 {
			w.Header().Set("ETag", `"cached-release-v1"`)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `[{"tag_name":%q,"draft":false,"prerelease":false,"assets":%s}]`, tag, assetsJSON)
			return
		}
		if got := r.Header.Get("If-None-Match"); got != `"cached-release-v1"` {
			t.Fatalf("cached release If-None-Match = %q, want cached ETag", got)
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	t.Cleanup(apiServer.Close)

	cacheDir := t.TempDir()
	client, err := release.NewGitHubReleaseClient(release.ReleaseClientOptions{APIBaseURL: apiServer.URL, Cache: NewFileReleaseCache(cacheDir, filepath.Join(cacheDir, release.CacheFilename))})
	require.NoError(t, err)
	_, err = client.Check(context.Background(), release.ReleaseCheckOptions{})
	require.NoError(t, err)
	checked, err := client.Check(context.Background(), release.ReleaseCheckOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, apiRequests)
	require.NotNil(t, checked.Release)

	target := filepath.Join(t.TempDir(), releaseBinaryName(runtime.GOOS))
	require.NoError(t, os.WriteFile(target, []byte("old executable"), 0o755))
	installer := NewReleaseInstaller(ReleaseInstallerOptions{
		HTTPClient:  assetServer.Client(),
		TrustedKeys: map[string]ed25519.PublicKey{"fixture": make(ed25519.PublicKey, ed25519.PublicKeySize)},
		ExecutablePath: func() (string, error) {
			return target, nil
		},
	})

	err = installer.Install(context.Background(), *checked.Release)
	require.ErrorContains(t, err, checksumsAssetName)
	require.Equal(t, 0, assetRequests, "untrusted cached URL must not receive an asset request")
	installed, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	require.Equal(t, []byte("old executable"), installed)
}

// TestReleaseInstallerFollowsOfficialGitHubDownloadRedirect 覆盖生产默认 URL 校验后的
// 完整签名、checksum、解包和替换链路。GitHub 初始下载 URL 合法时，HTTP client 仍可
// 跟随受 GitHub 控制的 CDN redirect，避免把最终 redirect host 误当作初始信任边界。
func TestReleaseInstallerFollowsOfficialGitHubDownloadRedirect(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	const keyID = "fixture-2026"
	const tag = "v0.2.0"
	archiveName := releaseArchiveName("0.2.0", runtime.GOOS, runtime.GOARCH)
	archive, executable := fixtureNativeExecutableArchive(t)
	checksum := sha256.Sum256(archive)
	checksums := []byte(hex.EncodeToString(checksum[:]) + "  " + archiveName + "\n")
	manifest := signedChecksumsManifest(t, keyID, privateKey, checksums)

	cdnRequests := 0
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cdnRequests++
		switch r.URL.Path {
		case "/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + archiveName:
			_, _ = w.Write(archive)
		case "/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + checksumsAssetName:
			_, _ = w.Write(checksums)
		case "/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + manifestAssetName:
			_, _ = w.Write(manifest)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(cdn.Close)
	transport := &githubAssetRedirectTransport{cdnBaseURL: cdn.URL}
	target := filepath.Join(t.TempDir(), releaseBinaryName(runtime.GOOS))
	require.NoError(t, os.WriteFile(target, []byte("old executable"), 0o755))
	installer := NewReleaseInstaller(ReleaseInstallerOptions{
		HTTPClient:  &http.Client{Transport: transport},
		TrustedKeys: map[string]ed25519.PublicKey{keyID: privateKey.Public().(ed25519.PublicKey)},
		ExecutablePath: func() (string, error) {
			return target, nil
		},
	})

	release := Release{
		TagName: tag,
		Version: "0.2.0",
		Assets: []ReleaseAsset{
			{Name: archiveName, DownloadURL: "https://github.com/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + archiveName},
			{Name: checksumsAssetName, DownloadURL: "https://github.com/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + checksumsAssetName},
			{Name: manifestAssetName, DownloadURL: "https://github.com/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + manifestAssetName},
		},
	}
	require.NoError(t, installer.Install(context.Background(), release))
	require.Equal(t, 3, cdnRequests)
	require.Equal(t, []string{
		"https://github.com/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + checksumsAssetName,
		"https://github.com/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + manifestAssetName,
		"https://github.com/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + archiveName,
	}, transport.githubRequests)
	installed, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, executable, installed)
}

func TestReleaseInstallerFailureKeepsExistingExecutable(t *testing.T) {
	t.Parallel()

	archive := fixtureRuntimeArchive(t, map[string]string{releaseBinaryName(runtime.GOOS): "new executable"})
	installer, release, target := verifiedFixtureInstaller(t, archive, []byte("tampered archive"))

	err := installer.Install(context.Background(), release)
	require.ErrorContains(t, err, "SHA-256 does not match")
	body, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	require.Equal(t, "old executable", string(body))
}

// TestReleaseInstallerStopsAfterCompletedArchiveDownloadWhenCanceled 确保 archive
// 已完整读入后发生的取消仍会在解包前终止安装；下载完成不能成为绕过取消语义的窗口。
func TestReleaseInstallerStopsAfterCompletedArchiveDownloadWhenCanceled(t *testing.T) {
	archive := fixtureRuntimeArchive(t, map[string]string{releaseBinaryName(runtime.GOOS): "new executable"})
	installer, release, target := verifiedFixtureInstaller(t, archive, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	installer.httpClient = &http.Client{Transport: &cancelAfterAssetResponseTransport{
		base:     installer.httpClient.Transport,
		assetURL: release.Assets[0].DownloadURL,
		cancel:   cancel,
	}}
	checkerCalled := false
	installer.checker = releaseBinaryCheckerFunc(func(context.Context, string, string) error {
		checkerCalled = true
		return fmt.Errorf("checker must not run after archive download cancellation")
	})
	replacerCalled := false
	installer.replacer = releaseFileReplacerFunc(func(string, string) error {
		replacerCalled = true
		return fmt.Errorf("replacer must not run after archive download cancellation")
	})

	err := installer.Install(ctx, release)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, checkerCalled)
	require.False(t, replacerCalled)
	installed, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	require.Equal(t, []byte("old executable"), installed)
	requireNoUpdateTemporaryPaths(t, filepath.Dir(target))
}

// TestReleaseInstallerRejectsNormalizedArchiveTraversalBeforePreflightAndReplace
// 确保 archive 原始名称中的 `..` 不会被 path.Clean 隐藏。该路径即使归一化后正好
// 是目标二进制，也必须在写入候选文件、版本预检及原子替换之前被拒绝。
func TestReleaseInstallerRejectsNormalizedArchiveTraversalBeforePreflightAndReplace(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		goos := goos
		t.Run(goos, func(t *testing.T) {
			binaryName := releaseBinaryName(goos)
			unsafeName := "nested/../" + binaryName
			archive := fixturePlatformArchiveEntries(t, goos, []tarFixtureEntry{
				{Name: unsafeName, Body: "must not become a candidate binary"},
			})
			installer, release, target := verifiedFixtureInstallerForPlatform(t, goos, "amd64", archive, nil)
			checkerCalled := false
			replacerCalled := false
			installer.checker = releaseBinaryCheckerFunc(func(context.Context, string, string) error {
				checkerCalled = true
				return nil
			})
			installer.replacer = releaseFileReplacerFunc(func(string, string) error {
				replacerCalled = true
				return fmt.Errorf("replacer must not run for unsafe archive path")
			})

			err := installer.Install(context.Background(), release)
			require.ErrorContains(t, err, "unsafe path")
			require.ErrorContains(t, err, unsafeName)
			require.False(t, checkerCalled, "unsafe archive path must fail before binary preflight")
			require.False(t, replacerCalled, "unsafe archive path must fail before replacement")
			installed, readErr := os.ReadFile(target)
			require.NoError(t, readErr)
			require.Equal(t, []byte("old executable"), installed)
		})
	}
}

func TestReleaseInstallerSelectsWindowsZIPAsset(t *testing.T) {
	t.Parallel()
	archive := fixtureZip(t, map[string]string{"LICENSE": "fixture", "pixiv.exe": "new windows binary"})
	installer, release, target := verifiedFixtureInstallerForPlatform(t, "windows", "amd64", archive, nil)
	installer.checker = releaseBinaryCheckerFunc(func(context.Context, string, string) error { return nil })

	require.NoError(t, installer.Install(context.Background(), release))
	installed, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "new windows binary", string(installed))
}

func TestSelectReleaseAssetsRejectsMissingAndDuplicateSidecars(t *testing.T) {
	t.Parallel()
	archiveName := releaseArchiveName("0.2.0", "linux", "amd64")
	_, err := selectReleaseAssets([]ReleaseAsset{
		{Name: archiveName, DownloadURL: "https://example.test/archive"},
		{Name: checksumsAssetName, DownloadURL: "https://example.test/checksums"},
	}, archiveName)
	require.ErrorContains(t, err, manifestAssetName)

	_, err = selectReleaseAssets([]ReleaseAsset{
		{Name: archiveName, DownloadURL: "https://example.test/archive"},
		{Name: checksumsAssetName, DownloadURL: "https://example.test/checksums"},
		{Name: manifestAssetName, DownloadURL: "https://example.test/manifest"},
		{Name: manifestAssetName, DownloadURL: "https://example.test/manifest-duplicate"},
	}, archiveName)
	require.ErrorContains(t, err, "duplicate")
}

func TestVerifyChecksumsManifestRejectsTampering(t *testing.T) {
	t.Parallel()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	checksums := []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  pixiv-cli_0.2.0_linux_amd64.tar.gz\n")
	manifest := signedChecksumsManifest(t, "fixture-2026", privateKey, checksums)

	require.NoError(t, verifyChecksumsManifest(manifest, checksums, map[string]ed25519.PublicKey{"fixture-2026": publicKey}))

	var decoded checksumsManifest
	require.NoError(t, json.Unmarshal(manifest, &decoded))
	decoded.Signature = base64.StdEncoding.EncodeToString([]byte("invalid signature"))
	invalidSignature, err := json.Marshal(decoded)
	require.NoError(t, err)
	require.ErrorContains(t, verifyChecksumsManifest(invalidSignature, checksums, map[string]ed25519.PublicKey{"fixture-2026": publicKey}), "signature verification failed")

	decoded = checksumsManifest{}
	require.NoError(t, json.Unmarshal(manifest, &decoded))
	decoded.ChecksumsSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	invalidHash, err := json.Marshal(decoded)
	require.NoError(t, err)
	require.ErrorContains(t, verifyChecksumsManifest(invalidHash, checksums, map[string]ed25519.PublicKey{"fixture-2026": publicKey}), "SHA-256 does not match")
}

func TestCleanupPendingWindowsUpdate(t *testing.T) {
	t.Parallel()
	target := filepath.Join(t.TempDir(), "pixiv.exe")
	oldPath := target + ".old"
	require.NoError(t, os.WriteFile(oldPath, []byte("old"), 0o600))

	require.NoError(t, cleanupPendingWindowsUpdate("windows", func() (string, error) { return target, nil }, os.Remove))
	_, err := os.Stat(oldPath)
	require.ErrorIs(t, err, os.ErrNotExist)

	want := fmt.Errorf("remove denied")
	err = cleanupPendingWindowsUpdate("windows", func() (string, error) { return target, nil }, func(string) error { return want })
	require.ErrorIs(t, err, want)
	require.NoError(t, cleanupPendingWindowsUpdate("linux", func() (string, error) { return "", fmt.Errorf("must not run") }, func(string) error { return fmt.Errorf("must not run") }))
}

func fixtureTarGz(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	tarEntries := make([]tarFixtureEntry, 0, len(entries))
	for name, value := range entries {
		tarEntries = append(tarEntries, tarFixtureEntry{Name: name, Body: value})
	}
	return fixtureTarGzEntries(t, tarEntries)
}

// fixtureNativeExecutableArchive 将当前 Go 测试二进制装进当前平台约定的 archive，
// 以真实执行 processReleaseBinaryChecker，而不是在 Windows 用 Unix shell fixture 降级。
func fixtureNativeExecutableArchive(t *testing.T) ([]byte, []byte) {
	t.Helper()
	executable, err := os.ReadFile(os.Args[0])
	require.NoError(t, err)
	return fixtureRuntimeArchive(t, map[string]string{
		"LICENSE":                       "fixture license\n",
		releaseBinaryName(runtime.GOOS): string(executable),
	}), executable
}

func fixtureRuntimeArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	return fixturePlatformArchive(t, runtime.GOOS, entries)
}

func fixturePlatformArchive(t *testing.T, goos string, entries map[string]string) []byte {
	t.Helper()
	if goos == "windows" {
		return fixtureZip(t, entries)
	}
	return fixtureTarGz(t, entries)
}

func fixturePlatformArchiveEntries(t *testing.T, goos string, entries []tarFixtureEntry) []byte {
	t.Helper()
	if goos != "windows" {
		return fixtureTarGzEntries(t, entries)
	}
	zipEntries := make([]zipFixtureEntry, 0, len(entries))
	for _, entry := range entries {
		mode := os.FileMode(0o755)
		if entry.Typeflag == tar.TypeSymlink || entry.Typeflag == tar.TypeLink {
			mode |= os.ModeSymlink
		}
		zipEntries = append(zipEntries, zipFixtureEntry{Name: entry.Name, Body: entry.Body, Mode: mode})
	}
	return fixtureZipEntries(t, zipEntries)
}

type tarFixtureEntry struct {
	Name     string
	Body     string
	Typeflag byte
	Linkname string
}

func fixtureTarGzEntries(t *testing.T, entries []tarFixtureEntry) []byte {
	t.Helper()
	var body bytes.Buffer
	compressed := gzip.NewWriter(&body)
	archive := tar.NewWriter(compressed)
	for _, entry := range entries {
		typeflag := entry.Typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		header := &tar.Header{Name: entry.Name, Linkname: entry.Linkname, Mode: 0o755, Size: int64(len(entry.Body)), Typeflag: typeflag}
		require.NoError(t, archive.WriteHeader(header))
		_, err := io.WriteString(archive, entry.Body)
		require.NoError(t, err)
	}
	require.NoError(t, archive.Close())
	require.NoError(t, compressed.Close())
	return body.Bytes()
}

func fixtureZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	zipEntries := make([]zipFixtureEntry, 0, len(entries))
	for name, value := range entries {
		zipEntries = append(zipEntries, zipFixtureEntry{Name: name, Body: value, Mode: 0o755})
	}
	return fixtureZipEntries(t, zipEntries)
}

type zipFixtureEntry struct {
	Name string
	Body string
	Mode os.FileMode
}

func fixtureZipEntries(t *testing.T, entries []zipFixtureEntry) []byte {
	t.Helper()
	var body bytes.Buffer
	archive := zip.NewWriter(&body)
	for _, fixture := range entries {
		header := &zip.FileHeader{Name: fixture.Name}
		header.SetMode(fixture.Mode)
		entry, err := archive.CreateHeader(header)
		require.NoError(t, err)
		_, err = io.WriteString(entry, fixture.Body)
		require.NoError(t, err)
	}
	require.NoError(t, archive.Close())
	return body.Bytes()
}

func verifiedFixtureInstaller(t *testing.T, archive, servedArchive []byte) (*releaseInstaller, Release, string) {
	return verifiedFixtureInstallerForPlatform(t, runtime.GOOS, runtime.GOARCH, archive, servedArchive)
}

func verifiedFixtureInstallerForPlatform(t *testing.T, goos, goarch string, archive, servedArchive []byte) (*releaseInstaller, Release, string) {
	t.Helper()
	if servedArchive == nil {
		servedArchive = archive
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	const keyID = "fixture-2026"
	const tag = "v0.2.0"
	archiveName := releaseArchiveName("0.2.0", goos, goarch)
	checksum := sha256.Sum256(archive)
	checksums := []byte(hex.EncodeToString(checksum[:]) + "  " + archiveName + "\n")
	manifest := signedChecksumsManifest(t, keyID, privateKey, checksums)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + archiveName:
			_, _ = w.Write(servedArchive)
		case "/checksums.txt":
			_, _ = w.Write(checksums)
		case "/checksums.json":
			_, _ = w.Write(manifest)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	target := filepath.Join(t.TempDir(), releaseBinaryName(goos))
	require.NoError(t, os.WriteFile(target, []byte("old executable"), 0o755))
	installer := NewReleaseInstaller(ReleaseInstallerOptions{
		HTTPClient:  server.Client(),
		TrustedKeys: map[string]ed25519.PublicKey{keyID: privateKey.Public().(ed25519.PublicKey)},
		ExecutablePath: func() (string, error) {
			return target, nil
		},
		GOOS:   goos,
		GOARCH: goarch,
	}).(*releaseInstaller)
	installer.assetURLValidator = allowFixtureReleaseAssetURL
	return installer, Release{
		TagName: tag,
		Version: "0.2.0",
		Assets: []ReleaseAsset{
			{Name: archiveName, DownloadURL: server.URL + "/" + archiveName},
			{Name: checksumsAssetName, DownloadURL: server.URL + "/checksums.txt"},
			{Name: manifestAssetName, DownloadURL: server.URL + "/checksums.json"},
		},
	}, target
}

type releaseBinaryCheckerFunc func(context.Context, string, string) error

// allowFixtureReleaseAssetURL 仅供本包 archive fixture 使用；生产构造未注入该规则，
// 必然采用固定 GitHub URL 验证器。
func allowFixtureReleaseAssetURL(Release, ReleaseAsset) error {
	return nil
}

func (f releaseBinaryCheckerFunc) Check(ctx context.Context, path, tag string) error {
	return f(ctx, path, tag)
}

type releaseFileReplacerFunc func(string, string) error

func (f releaseFileReplacerFunc) Replace(source, target string) error {
	return f(source, target)
}

// cancelAfterAssetResponseTransport 会在指定 asset 的 response body 读到 EOF 后取消
// 调用方 context，从而精确模拟“archive 已下载完成、尚未开始解包”的边界。
type cancelAfterAssetResponseTransport struct {
	base     http.RoundTripper
	assetURL string
	cancel   context.CancelFunc
}

func (t *cancelAfterAssetResponseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	response, err := base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if request.URL.String() == t.assetURL {
		response.Body = &cancelOnEOFReadCloser{ReadCloser: response.Body, cancel: t.cancel}
	}
	return response, nil
}

type cancelOnEOFReadCloser struct {
	io.ReadCloser
	cancel   context.CancelFunc
	canceled bool
}

func (r *cancelOnEOFReadCloser) Read(body []byte) (int, error) {
	n, err := r.ReadCloser.Read(body)
	if errors.Is(err, io.EOF) && !r.canceled {
		r.canceled = true
		r.cancel()
	}
	return n, err
}

func requireNoUpdateTemporaryPaths(t *testing.T, directory string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(directory, ".pixiv-update-*"))
	require.NoError(t, err)
	require.Empty(t, paths)
}

type rejectingAssetTransport struct {
	requests int
}

func (t *rejectingAssetTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.requests++
	return nil, fmt.Errorf("asset URL validator should have rejected the request")
}

type githubAssetRedirectTransport struct {
	cdnBaseURL     string
	githubRequests []string
}

func (t *githubAssetRedirectTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme == "https" && request.URL.Host == "github.com" {
		t.githubRequests = append(t.githubRequests, request.URL.String())
		return &http.Response{
			StatusCode: http.StatusFound,
			Header:     http.Header{"Location": []string{t.cdnBaseURL + request.URL.EscapedPath()}},
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Request:    request,
		}, nil
	}
	return http.DefaultTransport.RoundTrip(request)
}

func signedChecksumsManifest(t *testing.T, keyID string, privateKey ed25519.PrivateKey, checksums []byte) []byte {
	t.Helper()
	hash := sha256.Sum256(checksums)
	manifest, err := json.Marshal(struct {
		KeyID           string `json:"key_id"`
		ChecksumsSHA256 string `json:"checksums_sha256"`
		Signature       string `json:"signature"`
	}{
		KeyID:           keyID,
		ChecksumsSHA256: hex.EncodeToString(hash[:]),
		Signature:       base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, checksums)),
	})
	require.NoError(t, err)
	return manifest
}

func TestReleaseAssetDownloadRetriesRemainingSourcesAfterPreferredSourceFails(t *testing.T) {
	t.Parallel()

	const canonical = "https://github.com/FlanChanXwO/pixiv-cli/releases/download/v1.2.3/checksums.txt"
	preferred := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusBadGateway)
	}))
	t.Cleanup(preferred.Close)
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "verified checksums")
	}))
	t.Cleanup(fallback.Close)

	sources := mustTestReleaseSources(t,
		"preferred|-|"+preferred.URL+"?target={url_query}",
		"fallback|-|"+fallback.URL+"?target={url_query}",
	)
	installer := NewReleaseInstaller(ReleaseInstallerOptions{HTTPClient: preferred.Client()}).(*releaseInstaller)
	body, err := installer.download(context.Background(), release.ReleaseAsset{Name: checksumsAssetName, DownloadURL: canonical}, sources)
	if err != nil {
		t.Fatalf("download() error = %v", err)
	}
	if got := string(body); got != "verified checksums" {
		t.Fatalf("downloaded body = %q, want fallback body", got)
	}
}

func TestReleaseAssetDownloadReportsEverySourceFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)
	sources := mustTestReleaseSources(t,
		"first|-|"+server.URL+"?target={url_query}",
		"second|-|"+server.URL+"?target={url_query}",
	)
	installer := NewReleaseInstaller(ReleaseInstallerOptions{HTTPClient: server.Client()}).(*releaseInstaller)
	_, err := installer.download(context.Background(), release.ReleaseAsset{Name: checksumsAssetName, DownloadURL: "https://github.com/FlanChanXwO/pixiv-cli/releases/download/v1.2.3/checksums.txt"}, sources)
	if err == nil {
		t.Fatal("download() succeeded, want aggregate source failure")
	}
	for _, sourceID := range []string{"first", "second"} {
		if !strings.Contains(err.Error(), sourceID) {
			t.Fatalf("download() error = %v, missing source %q", err, sourceID)
		}
	}
}

func mustTestReleaseSources(t *testing.T, lines ...string) []source.ReleaseSource {
	t.Helper()
	sources, err := source.ParseReleaseSources([]byte(joinReleaseSourceLines(lines)))
	if err != nil {
		t.Fatalf("ParseReleaseSources() error = %v", err)
	}
	return sources
}

func joinReleaseSourceLines(lines []string) string {
	result := ""
	for _, line := range lines {
		result += line + "\n"
	}
	return result
}

const pixivCLIModulePath = "github.com/FlanChanXwO/pixiv-cli"

func TestDetectInstallSourceDevelopmentAvoidsSystemAccess(t *testing.T) {
	deps := sourceDetector{
		executable: func() (string, error) {
			t.Fatal("development build must not inspect the executable path")
			return "", nil
		},
		evalSymlinks: func(string) (string, error) {
			t.Fatal("development build must not resolve symlinks")
			return "", nil
		},
		readFile: func(string) ([]byte, error) {
			t.Fatal("development build must not read the filesystem")
			return nil, nil
		},
		readBuildInfo: func() (*debug.BuildInfo, bool) {
			t.Fatal("development build must not read Go build info")
			return nil, false
		},
	}

	got, err := detectInstallSource(buildinfo.Info{Version: "dev"}, deps)
	if err != nil {
		t.Fatalf("detectInstallSource() error = %v", err)
	}
	if got != InstallSourceDevelopment {
		t.Fatalf("detectInstallSource() = %q, want %q", got, InstallSourceDevelopment)
	}
}

func TestDetectInstallSourceHomebrewKeg(t *testing.T) {
	tests := []struct {
		name    string
		formula string
		want    InstallSource
	}{
		{name: "stable formula", formula: "pixiv-cli", want: InstallSourceHomebrewStable},
		{name: "beta formula", formula: "pixiv-cli-beta", want: InstallSourceHomebrewBeta},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rawPath, actualPath := homebrewExecutableFixture(t, test.formula, homebrewReceipt(test.formula))
			deps := testDetector(rawPath, actualPath, map[string]string{})
			deps.evalSymlinks = filepath.EvalSymlinks
			deps.readFile = os.ReadFile

			got, err := detectInstallSource(buildinfo.Info{Version: "v0.1.0"}, deps)
			if err != nil {
				t.Fatalf("detectInstallSource() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("detectInstallSource() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDetectInstallSourceGoInstallWithExplicitGOBIN(t *testing.T) {
	gobin := filepath.Join(t.TempDir(), "bin")
	executable := filepath.Join(gobin, pixivExecutableName(runtime.GOOS))
	deps := testDetector(executable, executable, map[string]string{"GOBIN": gobin})
	deps.readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Path: pixivCLIModulePath + "/cmd/pixiv", Main: debug.Module{Path: pixivCLIModulePath}}, true
	}

	got, err := detectInstallSource(buildinfo.Info{Version: "v0.1.0"}, deps)
	if err != nil {
		t.Fatalf("detectInstallSource() error = %v", err)
	}
	if got != InstallSourceGoInstall {
		t.Fatalf("detectInstallSource() = %q, want %q", got, InstallSourceGoInstall)
	}
}

func TestDetectInstallSourceDoesNotMistakeGoBuildForGoInstall(t *testing.T) {
	tests := []struct {
		name          string
		executable    string
		buildInfo     *debug.BuildInfo
		buildInfoOkay bool
		env           map[string]string
	}{
		{
			name:          "matching module outside GOBIN",
			executable:    filepath.Join(t.TempDir(), "pixiv"),
			buildInfo:     &debug.BuildInfo{Path: pixivCLIModulePath + "/cmd/pixiv", Main: debug.Module{Path: pixivCLIModulePath}},
			buildInfoOkay: true,
			env:           map[string]string{"GOBIN": filepath.Join(t.TempDir(), "bin")},
		},
		{
			name:          "GOBIN executable with another main module",
			buildInfo:     &debug.BuildInfo{Path: pixivCLIModulePath, Main: debug.Module{Path: "example.com/not-pixiv"}},
			buildInfoOkay: true,
			env:           map[string]string{},
		},
		{
			name:          "GOBIN executable without build info",
			buildInfoOkay: false,
			env:           map[string]string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gobin := filepath.Join(t.TempDir(), "bin")
			if test.env == nil {
				test.env = map[string]string{}
			}
			if test.env["GOBIN"] == "" {
				test.env["GOBIN"] = gobin
			}
			if test.executable == "" {
				test.executable = filepath.Join(test.env["GOBIN"], pixivExecutableName(runtime.GOOS))
			}
			deps := testDetector(test.executable, test.executable, test.env)
			deps.readBuildInfo = func() (*debug.BuildInfo, bool) {
				return test.buildInfo, test.buildInfoOkay
			}

			got, err := detectInstallSource(buildinfo.Info{Version: "v0.1.0"}, deps)
			if err != nil {
				t.Fatalf("detectInstallSource() error = %v", err)
			}
			if got != InstallSourceRelease {
				t.Fatalf("detectInstallSource() = %q, want %q", got, InstallSourceRelease)
			}
		})
	}
}

func homebrewExecutableFixture(t *testing.T, formula, receipt string) (rawPath, actualPath string) {
	t.Helper()
	root := t.TempDir()
	actualPath = filepath.Join(root, "Cellar", formula, "0.1.0", "bin", pixivExecutableName(runtime.GOOS))
	if err := os.MkdirAll(filepath.Dir(actualPath), 0o755); err != nil {
		t.Fatalf("create keg fixture: %v", err)
	}
	if err := os.WriteFile(actualPath, []byte("fixture"), 0o755); err != nil {
		t.Fatalf("write keg executable fixture: %v", err)
	}
	receiptPath := filepath.Join(filepath.Dir(filepath.Dir(actualPath)), "INSTALL_RECEIPT.json")
	if err := os.WriteFile(receiptPath, []byte(receipt), 0o644); err != nil {
		t.Fatalf("write receipt fixture: %v", err)
	}
	rawPath = filepath.Join(root, "bin", pixivExecutableName(runtime.GOOS))
	if err := os.MkdirAll(filepath.Dir(rawPath), 0o755); err != nil {
		t.Fatalf("create raw executable directory: %v", err)
	}
	if err := os.Symlink(actualPath, rawPath); err != nil {
		t.Fatalf("create Homebrew executable symlink: %v", err)
	}
	return rawPath, actualPath
}

func homebrewReceipt(formula string) string {
	return `{"source":{"path":"Formula/` + formula + `.rb"}}`
}

func testDetector(executable, resolvedPath string, env map[string]string) sourceDetector {
	return sourceDetector{
		executable: func() (string, error) { return executable, nil },
		evalSymlinks: func(path string) (string, error) {
			if path == executable {
				return resolvedPath, nil
			}
			return path, nil
		},
		readFile: func(string) ([]byte, error) {
			return nil, errors.New("unexpected Homebrew receipt read")
		},
		readBuildInfo: func() (*debug.BuildInfo, bool) { return nil, false },
		getenv:        func(key string) string { return env[key] },
		goos:          runtime.GOOS,
	}
}

func TestWriteReleaseCachePreservesNewSourceWhenReplacementRecoveryIsUnresolved(t *testing.T) {
	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, release.CacheFilename)
	fileCache := NewFileReleaseCache(cacheDir, cachePath).(*fileReleaseCache)
	require.NoError(t, os.WriteFile(cachePath, []byte("old cache"), 0o600))
	var source string
	replaceCause := errors.New("replacement recovery unresolved")
	fileCache.replaceFile = func(sourcePath, _ string) error {
		source = sourcePath
		return preserveInstallerSourceError{err: replaceCause}
	}

	err := fileCache.Write(context.Background(), []byte(`{"schema_version":2,"checked_at":"2026-07-11T12:00:00Z"}`+"\n"))
	require.ErrorIs(t, err, replaceCause)

	oldBody, readErr := os.ReadFile(cachePath)
	require.NoError(t, readErr)
	assert.Equal(t, "old cache", string(oldBody))
	newBody, readErr := os.ReadFile(source)
	require.NoError(t, readErr)
	assert.Contains(t, string(newBody), `"schema_version":2`)
	assert.Equal(t, cacheDir, filepath.Dir(source))
}
