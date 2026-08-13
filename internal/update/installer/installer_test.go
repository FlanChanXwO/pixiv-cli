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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/FlanChanXwO/pixiv-cli/internal/update/release"
)

// Release 与 ReleaseAsset 是 installer 包消费的 release 值类型别名，保持既有测试字面量不变。
type Release = release.Release

type ReleaseAsset = release.ReleaseAsset

// TestMain 让复制到 fixture archive 的当前测试二进制可以作为真实更新二进制运行。
// 生产 checker 固定调用 `version --json`，此分支模拟该公开 CLI 契约，而常规测试不受影响。
func TestMain(m *testing.M) {
	if len(os.Args) == 3 && os.Args[1] == "version" && os.Args[2] == "--json" {
		_, _ = fmt.Fprintln(os.Stdout, `{"version":"v0.2.0"}`)
		os.Exit(0)
	}
	os.Exit(m.Run())
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

func (e preserveInstallerSourceError) Error() string            { return e.err.Error() }
func (e preserveInstallerSourceError) Unwrap() error            { return e.err }
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

func TestReleaseInstallerCleansStagedSourceAfterOrdinaryReplacementFailure(t *testing.T) {
	archive := fixtureRuntimeArchive(t, map[string]string{releaseBinaryName(runtime.GOOS): "new executable"})
	installer, release, target := verifiedFixtureInstaller(t, archive, nil)
	installer.checker = releaseBinaryCheckerFunc(func(context.Context, string, string) error { return nil })
	var stagedPath string
	replaceCause := errors.New("replacement unchanged")
	installer.replacer = releaseFileReplacerFunc(func(source, _ string) error {
		stagedPath = source
		return replaceCause
	})

	err := installer.Install(context.Background(), release)
	require.ErrorIs(t, err, replaceCause)
	_, statErr := os.Stat(stagedPath)
	require.ErrorIs(t, statErr, os.ErrNotExist)
	oldBody, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	require.Equal(t, "old executable", string(oldBody))
}

// TestReleaseInstallerRejectsBrokenExecutableSymlink 确保断开的软链接在版本预检、
// staging 和替换之前就暴露真实解析错误，不能被更新流程悄然替换为普通文件。
func TestReleaseInstallerRejectsBrokenExecutableSymlink(t *testing.T) {
	archive := fixtureRuntimeArchive(t, map[string]string{releaseBinaryName(runtime.GOOS): "new executable"})
	installer, release, untouchedTarget := verifiedFixtureInstaller(t, archive, nil)
	root := filepath.Dir(untouchedTarget)
	rawLink := filepath.Join(root, "bin", releaseBinaryName(runtime.GOOS))
	linkTarget := filepath.Join("..", "missing-release", releaseBinaryName(runtime.GOOS))
	require.NoError(t, os.MkdirAll(filepath.Dir(rawLink), 0o755))
	require.NoError(t, os.Symlink(linkTarget, rawLink))
	installer.executablePath = func() (string, error) { return rawLink, nil }
	checkerCalled := false
	installer.checker = releaseBinaryCheckerFunc(func(context.Context, string, string) error {
		checkerCalled = true
		return fmt.Errorf("checker must not run for a broken executable symlink")
	})
	replacerCalled := false
	installer.replacer = releaseFileReplacerFunc(func(string, string) error {
		replacerCalled = true
		return fmt.Errorf("replacer must not run for a broken executable symlink")
	})

	err := installer.Install(context.Background(), release)
	require.ErrorContains(t, err, "resolve executable symlink")
	// 错误上下文以 %q 呈现路径；Windows 路径中的反斜杠会随 Go 字符串规则转义。
	require.ErrorContains(t, err, fmt.Sprintf("%q", rawLink))
	require.ErrorIs(t, err, os.ErrNotExist)
	require.False(t, checkerCalled)
	require.False(t, replacerCalled)
	linkInfo, lstatErr := os.Lstat(rawLink)
	require.NoError(t, lstatErr)
	require.True(t, linkInfo.Mode()&os.ModeSymlink != 0, "broken link entry must remain a symlink")
	readLink, readLinkErr := os.Readlink(rawLink)
	require.NoError(t, readLinkErr)
	require.Equal(t, linkTarget, readLink)
	_, targetErr := os.Stat(filepath.Join(filepath.Dir(rawLink), linkTarget))
	require.ErrorIs(t, targetErr, os.ErrNotExist)
	untouched, readErr := os.ReadFile(untouchedTarget)
	require.NoError(t, readErr)
	require.Equal(t, []byte("old executable"), untouched)
	requireNoUpdateTemporaryPaths(t, filepath.Dir(rawLink))
	requireNoUpdateTemporaryPaths(t, filepath.Dir(untouchedTarget))
}

// TestReleaseInstallerRejectsUntrustedCachedAssetURLBeforeRequest 覆盖真实的
// GitHub Releases API → ETag cache → Check → Install 路径。即使缓存中的
// browser_download_url 被篡改为本地服务，安装器也必须在发出任何 asset 请求前失败。
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

// TestReleaseInstallerRejectsEveryUnexpectedGitHubAssetSource 确保默认信任规则不会因
// 同 host 的歧义 URL、跨仓库、跨 tag 或 asset 名错配而放宽；下载 transport 同样证明
// 安装器在失败前不向任一候选 URL 发请求。
func TestReleaseInstallerRejectsEveryUnexpectedGitHubAssetSource(t *testing.T) {
	const tag = "v0.2.0"
	archiveName := releaseArchiveName("0.2.0", runtime.GOOS, runtime.GOARCH)
	expectedArchiveURL := "https://github.com/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + archiveName
	tests := []struct {
		name string
		url  string
	}{
		{name: "non HTTPS scheme", url: "http://github.com/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + archiveName},
		{name: "different host", url: "https://downloads.example.test/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + archiveName},
		{name: "explicit port", url: "https://github.com:443/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + archiveName},
		{name: "userinfo", url: "https://attacker@github.com/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + archiveName},
		{name: "different repository", url: "https://github.com/FlanChanXwO/other/releases/download/" + tag + "/" + archiveName},
		{name: "different tag", url: "https://github.com/FlanChanXwO/pixiv-cli/releases/download/v0.2.1/" + archiveName},
		{name: "different asset path", url: "https://github.com/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/other.tar.gz"},
		{name: "query", url: expectedArchiveURL + "?redirect=attacker"},
		{name: "fragment", url: expectedArchiveURL + "#attacker"},
		{name: "encoded path", url: "https://github.com/FlanChanXwO/pixiv-cli/releases/download/" + tag + "%2F" + archiveName},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := &rejectingAssetTransport{}
			installer := NewReleaseInstaller(ReleaseInstallerOptions{
				HTTPClient:  &http.Client{Transport: transport},
				TrustedKeys: map[string]ed25519.PublicKey{"fixture": make(ed25519.PublicKey, ed25519.PublicKeySize)},
			})
			release := Release{
				TagName: tag,
				Version: "0.2.0",
				Assets: []ReleaseAsset{
					{Name: archiveName, DownloadURL: test.url},
					{Name: checksumsAssetName, DownloadURL: "https://github.com/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + checksumsAssetName},
					{Name: manifestAssetName, DownloadURL: "https://github.com/FlanChanXwO/pixiv-cli/releases/download/" + tag + "/" + manifestAssetName},
				},
			}

			err := installer.Install(context.Background(), release)
			require.ErrorContains(t, err, archiveName)
			require.ErrorContains(t, err, test.url)
			require.Equal(t, 0, transport.requests, "invalid source must fail before an asset request")
		})
	}
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

	tests := []struct {
		name          string
		archive       []byte
		servedArchive []byte
		binaryChecker ReleaseBinaryChecker
		unknownKeyID  bool
		replacer      ReleaseFileReplacer
		wantError     string
	}{
		{
			name:          "checksum mismatch",
			archive:       fixtureRuntimeArchive(t, map[string]string{releaseBinaryName(runtime.GOOS): "new"}),
			servedArchive: []byte("tampered archive"),
			wantError:     "SHA-256 does not match",
		},
		{
			name:      "corrupted archive",
			archive:   []byte("not a gzip archive"),
			wantError: runtimeArchiveOpenError(),
		},
		{
			name: "path traversal",
			archive: fixtureRuntimeArchive(t, map[string]string{
				"../" + releaseBinaryName(runtime.GOOS): "new",
			}),
			wantError: "unsafe path",
		},
		{
			name: "duplicate binary",
			archive: fixtureRuntimeArchiveEntries(t, []tarFixtureEntry{
				{Name: releaseBinaryName(runtime.GOOS), Body: "first"},
				{Name: "nested/" + releaseBinaryName(runtime.GOOS), Body: "second"},
			}),
			wantError: "duplicate binary",
		},
		{
			name:         "unknown signing key",
			archive:      fixtureRuntimeArchive(t, map[string]string{releaseBinaryName(runtime.GOOS): "new"}),
			unknownKeyID: true,
			wantError:    "unknown key ID",
		},
		{
			name:          "version preflight",
			archive:       fixtureRuntimeArchive(t, map[string]string{releaseBinaryName(runtime.GOOS): "new"}),
			binaryChecker: releaseBinaryCheckerFunc(func(context.Context, string, string) error { return fmt.Errorf("reported v0.1.0") }),
			wantError:     "reported v0.1.0",
		},
		{
			name: "link binary",
			archive: fixtureRuntimeArchiveEntries(t, []tarFixtureEntry{
				{Name: releaseBinaryName(runtime.GOOS), Typeflag: tar.TypeSymlink, Linkname: "elsewhere"},
			}),
			wantError: "link entry",
		},
		{
			name:          "replace failure",
			archive:       fixtureRuntimeArchive(t, map[string]string{releaseBinaryName(runtime.GOOS): "new"}),
			binaryChecker: releaseBinaryCheckerFunc(func(context.Context, string, string) error { return nil }),
			replacer:      releaseFileReplacerFunc(func(string, string) error { return fmt.Errorf("target is read-only") }),
			wantError:     "target is read-only",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			installer, release, target := verifiedFixtureInstaller(t, test.archive, test.servedArchive)
			if test.binaryChecker != nil {
				installer.checker = test.binaryChecker
			}
			if test.replacer != nil {
				installer.replacer = test.replacer
			}
			if test.unknownKeyID {
				installer.trustedKeys = map[string]ed25519.PublicKey{"other-key": make(ed25519.PublicKey, ed25519.PublicKeySize)}
			}

			err := installer.Install(context.Background(), release)
			require.ErrorContains(t, err, test.wantError)
			body, readErr := os.ReadFile(target)
			require.NoError(t, readErr)
			require.Equal(t, "old executable", string(body))
		})
	}
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

// TestReleaseInstallerStopsBeforeStagingWhenCheckerCancels 确保版本预检即使返回
// 成功，只要它期间调用方取消更新，安装器也不会继续创建 staged 文件或调用替换器。
func TestReleaseInstallerStopsBeforeStagingWhenCheckerCancels(t *testing.T) {
	archive := fixtureRuntimeArchive(t, map[string]string{releaseBinaryName(runtime.GOOS): "new executable"})
	installer, release, target := verifiedFixtureInstaller(t, archive, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	installer.checker = releaseBinaryCheckerFunc(func(context.Context, string, string) error {
		cancel()
		return nil
	})
	replacerCalled := false
	installer.replacer = releaseFileReplacerFunc(func(string, string) error {
		replacerCalled = true
		return fmt.Errorf("replacer must not run after preflight cancellation")
	})

	err := installer.Install(ctx, release)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, replacerCalled)
	installed, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	require.Equal(t, []byte("old executable"), installed)
	requireNoUpdateTemporaryPaths(t, filepath.Dir(target))
}

// TestReleaseInstallerStopsBeforeReplaceWhenCanceledAfterStaging 覆盖 staging 已完成、
// 准备最终原子替换的最后一个取消边界，确保取消不会被替换器观察为一次有效安装。
func TestReleaseInstallerStopsBeforeReplaceWhenCanceledAfterStaging(t *testing.T) {
	archive := fixtureRuntimeArchive(t, map[string]string{releaseBinaryName(runtime.GOOS): "new executable"})
	installer, release, target := verifiedFixtureInstaller(t, archive, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	installer.checker = releaseBinaryCheckerFunc(func(context.Context, string, string) error { return nil })
	installer.afterStaging = cancel
	replacerCalled := false
	installer.replacer = releaseFileReplacerFunc(func(string, string) error {
		replacerCalled = true
		return fmt.Errorf("replacer must not run after staging cancellation")
	})

	err := installer.Install(ctx, release)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, replacerCalled)
	installed, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	require.Equal(t, []byte("old executable"), installed)
	requireNoUpdateTemporaryPaths(t, filepath.Dir(target))
}

// TestReleaseInstallerClosesStagedFileBeforeCancellationCleanupOnWindows 模拟 Windows
// 平台的 ZIP 安装路径：Windows 不能删除仍打开的 staged 文件，因此 CreateTemp 后取消也
// 必须先关闭文件，才能让失败清理不遗留 `.pixiv-update-stage-*`。
func TestReleaseInstallerClosesStagedFileBeforeCancellationCleanupOnWindows(t *testing.T) {
	archive := fixturePlatformArchive(t, "windows", map[string]string{"pixiv.exe": "new executable"})
	installer, release, target := verifiedFixtureInstallerForPlatform(t, "windows", "amd64", archive, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	installer.checker = releaseBinaryCheckerFunc(func(context.Context, string, string) error { return nil })
	stagedFileClosed := false
	installer.afterStagedCreate = cancel
	installer.afterStagedClose = func() { stagedFileClosed = true }
	replacerCalled := false
	installer.replacer = releaseFileReplacerFunc(func(string, string) error {
		replacerCalled = true
		return fmt.Errorf("replacer must not run after staged-file cancellation")
	})

	err := installer.Install(ctx, release)
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, stagedFileClosed, "cancellation cleanup must close staged file before removal")
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

// TestReleaseInstallerRejectsLateTraversalBeforeCandidateWrite 确保 archive 的每个
// path 都先通过验证。即使合法 binary 排在前面，后续不参与二进制识别的遍历条目也不能
// 让解包器提前写出 candidate。
func TestReleaseInstallerRejectsLateTraversalBeforeCandidateWrite(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		goos := goos
		t.Run(goos, func(t *testing.T) {
			binaryName := releaseBinaryName(goos)
			unsafeName := "nested/../not-a-binary"
			archive := fixturePlatformArchiveEntries(t, goos, []tarFixtureEntry{
				{Name: binaryName, Body: "candidate must not be written"},
				{Name: unsafeName, Body: "unsafe later entry"},
			})
			destination := filepath.Join(t.TempDir(), binaryName)

			err := extractReleaseBinary(archive, releaseArchiveName("0.2.0", goos, "amd64"), destination, binaryName)
			require.ErrorContains(t, err, "unsafe path")
			require.ErrorContains(t, err, unsafeName)
			_, statErr := os.Stat(destination)
			require.ErrorIs(t, statErr, os.ErrNotExist, "unsafe archive path must fail before candidate creation")

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

			err = installer.Install(context.Background(), release)
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

// TestReleaseInstallerAcceptsNestedArchiveBinary 保留发布 archive 的普通目录布局：
// 仍应经过签名、checksum、版本预检和原子替换，而不是因目录名被安全检查误伤。
func TestReleaseInstallerAcceptsNestedArchiveBinary(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		goos := goos
		t.Run(goos, func(t *testing.T) {
			binaryName := releaseBinaryName(goos)
			archive := fixturePlatformArchiveEntries(t, goos, []tarFixtureEntry{
				{Name: "nested/" + binaryName, Body: "new nested executable"},
			})
			installer, release, target := verifiedFixtureInstallerForPlatform(t, goos, "amd64", archive, nil)
			checkerCalled := false
			replacerCalled := false
			installer.checker = releaseBinaryCheckerFunc(func(context.Context, string, string) error {
				checkerCalled = true
				return nil
			})
			defaultReplacer := installer.replacer
			installer.replacer = releaseFileReplacerFunc(func(source, destination string) error {
				replacerCalled = true
				return defaultReplacer.Replace(source, destination)
			})

			require.NoError(t, installer.Install(context.Background(), release))
			require.True(t, checkerCalled, "verified binary must be preflighted")
			require.True(t, replacerCalled, "verified binary must be atomically replaced")
			installed, err := os.ReadFile(target)
			require.NoError(t, err)
			require.Equal(t, []byte("new nested executable"), installed)
		})
	}
}

func TestReleaseInstallerReportsUnwritableExecutablePathBeforeReplacing(t *testing.T) {
	t.Parallel()
	installer, release, target := verifiedFixtureInstaller(t, fixtureRuntimeArchive(t, map[string]string{releaseBinaryName(runtime.GOOS): "new"}), nil)
	installer.checker = releaseBinaryCheckerFunc(func(context.Context, string, string) error { return nil })
	blockedParent := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(blockedParent, []byte("not a directory"), 0o600))
	installer.executablePath = func() (string, error) { return filepath.Join(blockedParent, "pixiv"), nil }

	err := installer.Install(context.Background(), release)
	require.ErrorContains(t, err, "create update temporary directory")
	body, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	require.Equal(t, "old executable", string(body))
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

func TestPlatformFixtureArchivesMatchReleaseContract(t *testing.T) {
	t.Parallel()
	for _, goos := range []string{"linux", "windows"} {
		goos := goos
		t.Run(goos, func(t *testing.T) {
			t.Parallel()
			binaryName := releaseBinaryName(goos)
			archive := fixturePlatformArchive(t, goos, map[string]string{binaryName: "fixture binary"})
			destination := filepath.Join(t.TempDir(), binaryName)
			require.NoError(t, extractReleaseBinary(archive, releaseArchiveName("0.2.0", goos, "amd64"), destination, binaryName))
			installed, err := os.ReadFile(destination)
			require.NoError(t, err)
			require.Equal(t, "fixture binary", string(installed))
		})
	}
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

// TestCleanupPendingWindowsUpdateRemovesResolvedSymlinkTargetBackup 确保 Windows
// ReplaceFileW 在真实目标旁留下的备份不会因启动入口是软链接而永久滞留。
func TestCleanupPendingWindowsUpdateRemovesResolvedSymlinkTargetBackup(t *testing.T) {
	root := t.TempDir()
	resolvedTarget := filepath.Join(root, "release", "pixiv.exe")
	rawLink := filepath.Join(root, "bin", "pixiv.exe")
	linkTarget := filepath.Join("..", "release", "pixiv.exe")
	require.NoError(t, os.MkdirAll(filepath.Dir(resolvedTarget), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(rawLink), 0o755))
	require.NoError(t, os.WriteFile(resolvedTarget, []byte("current executable"), 0o755))
	require.NoError(t, os.Symlink(linkTarget, rawLink))
	resolvedBackup := resolvedTarget + ".old"
	require.NoError(t, os.WriteFile(resolvedBackup, []byte("pending old executable"), 0o600))

	require.NoError(t, cleanupPendingWindowsUpdate("windows", func() (string, error) { return rawLink, nil }, os.Remove))
	_, statErr := os.Stat(resolvedBackup)
	require.ErrorIs(t, statErr, os.ErrNotExist)
	linkInfo, lstatErr := os.Lstat(rawLink)
	require.NoError(t, lstatErr)
	require.True(t, linkInfo.Mode()&os.ModeSymlink != 0, "cleanup must preserve the executable symlink")
	readLink, readLinkErr := os.Readlink(rawLink)
	require.NoError(t, readLinkErr)
	require.Equal(t, linkTarget, readLink)
}

// TestCleanupPendingWindowsUpdateRejectsBrokenExecutableSymlink 确保断链不会被当作
// raw 启动入口清理，避免误删入口旁的文件并保留可诊断的解析根因。
func TestCleanupPendingWindowsUpdateRejectsBrokenExecutableSymlink(t *testing.T) {
	root := t.TempDir()
	rawLink := filepath.Join(root, "bin", "pixiv.exe")
	linkTarget := filepath.Join("..", "missing-release", "pixiv.exe")
	missingTarget := filepath.Join(root, "missing-release", "pixiv.exe")
	require.NoError(t, os.MkdirAll(filepath.Dir(rawLink), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(missingTarget), 0o755))
	require.NoError(t, os.Symlink(linkTarget, rawLink))
	rawBackup := rawLink + ".old"
	resolvedBackup := missingTarget + ".old"
	require.NoError(t, os.WriteFile(rawBackup, []byte("raw backup"), 0o600))
	require.NoError(t, os.WriteFile(resolvedBackup, []byte("resolved backup"), 0o600))
	removeCalled := false

	err := cleanupPendingWindowsUpdate("windows", func() (string, error) { return rawLink, nil }, func(string) error {
		removeCalled = true
		return fmt.Errorf("remove must not run for a broken executable symlink")
	})
	require.ErrorContains(t, err, "resolve executable symlink")
	// 错误上下文以 %q 呈现路径；Windows 路径中的反斜杠会随 Go 字符串规则转义。
	require.ErrorContains(t, err, fmt.Sprintf("%q", rawLink))
	require.ErrorIs(t, err, os.ErrNotExist)
	require.False(t, removeCalled)
	linkInfo, lstatErr := os.Lstat(rawLink)
	require.NoError(t, lstatErr)
	require.True(t, linkInfo.Mode()&os.ModeSymlink != 0, "cleanup must preserve the broken executable symlink")
	readLink, readLinkErr := os.Readlink(rawLink)
	require.NoError(t, readLinkErr)
	require.Equal(t, linkTarget, readLink)
	_, rawBackupErr := os.Stat(rawBackup)
	require.NoError(t, rawBackupErr)
	_, resolvedBackupErr := os.Stat(resolvedBackup)
	require.NoError(t, resolvedBackupErr)
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

func fixtureRuntimeArchiveEntries(t *testing.T, entries []tarFixtureEntry) []byte {
	t.Helper()
	return fixturePlatformArchiveEntries(t, runtime.GOOS, entries)
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

func runtimeArchiveOpenError() string {
	if runtime.GOOS == "windows" {
		return "open zip release archive"
	}
	return "open gzip release archive"
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
