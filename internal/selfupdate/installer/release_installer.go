package installer

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/FlanChanXwO/pixiv-cli/internal/selfupdate/release"
	"github.com/FlanChanXwO/pixiv-cli/internal/selfupdate/source"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/atomicfile"
)

const (
	checksumsAssetName = "checksums.txt"
	manifestAssetName  = "checksums.json"
)

// ReleaseSigningKeyID 是生产 Release 签名使用的受信 Ed25519 公钥 ID。
const ReleaseSigningKeyID = "ed25519-2c27e77742d3c33a"

// ReleaseSigningPublicKey 是生产 Release 签名使用的受信 Ed25519 公钥。
var ReleaseSigningPublicKey = [...]byte{
	0xee, 0xb2, 0xe2, 0xef, 0xdd, 0x60, 0xb0, 0x61,
	0x6c, 0x0f, 0x61, 0x2b, 0xef, 0x59, 0xe1, 0xe5,
	0x55, 0x40, 0x5f, 0xf8, 0xab, 0xc0, 0xdb, 0xeb,
	0x59, 0x5c, 0x09, 0xdb, 0x9a, 0x78, 0xb1, 0x44,
}

// ReleaseInstaller 安装已核验的 Release。
type ReleaseInstaller interface {
	Install(context.Context, release.Release) error
}

// ReleaseInstallerOptions 允许生产组装与受控测试分别注入信任根和系统依赖。
// TrustedKeys 只允许 Ed25519 公钥；私钥绝不能进入生产组装或此 API。
type ReleaseInstallerOptions struct {
	HTTPClient                 *http.Client
	TrustedKeys                map[string]ed25519.PublicKey
	ExecutablePath             func() (string, error)
	EnablePublicReleaseSources bool
}

// NewReleaseInstaller 建立 Release 自更新器。未配置 trusted key 时仍返回实例，
// 这样 --check 不受影响；真正安装会明确报出缺少信任根，而不是假装更新成功。
func NewReleaseInstaller(options ReleaseInstallerOptions) ReleaseInstaller {
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	executablePath := options.ExecutablePath
	if executablePath == nil {
		executablePath = os.Executable
	}
	keys := make(map[string]ed25519.PublicKey, len(options.TrustedKeys))
	for keyID, publicKey := range options.TrustedKeys {
		keys[keyID] = append(ed25519.PublicKey(nil), publicKey...)
	}
	installer := &releaseInstaller{
		httpClient:     httpClient,
		trustedKeys:    keys,
		executablePath: executablePath,
		goos:           runtime.GOOS,
		goarch:         runtime.GOARCH,
	}
	if options.EnablePublicReleaseSources {
		installer.sourceSelector = source.NewDefaultReleaseSourceSelector(httpClient)
	}
	return installer
}

type releaseInstaller struct {
	httpClient     *http.Client
	trustedKeys    map[string]ed25519.PublicKey
	executablePath func() (string, error)
	goos           string
	goarch         string
	sourceSelector *source.ReleaseSourceSelector
}

// Install 下载并验证选中版本的唯一平台资产。任何下载、认证、解包、预检或替换失败
// 都发生在临时文件阶段，原可执行文件保持不变。
func (i *releaseInstaller) Install(ctx context.Context, rel release.Release) (err error) {
	if i == nil {
		return fmt.Errorf("release installer is nil")
	}
	if len(i.trustedKeys) == 0 {
		return fmt.Errorf("trusted release signing key is not configured")
	}
	if err := source.CheckContext(ctx, "prepare release update"); err != nil {
		return err
	}
	assets, err := i.prepareRelease(ctx, rel)
	if err != nil {
		return err
	}
	targetPath, workDir, err := i.stageRelease(ctx)
	if err != nil {
		return err
	}
	defer i.cleanupWorkDir(&err, workDir)
	candidatePath := filepath.Join(workDir, releaseBinaryName(i.goos))
	if err := extractReleaseBinary(assets.archivePayload, assets.archive.Name, candidatePath, releaseBinaryName(i.goos)); err != nil {
		return err
	}
	if err := source.CheckContext(ctx, "preflight downloaded executable"); err != nil {
		return err
	}
	if err := (processReleaseBinaryChecker{}).Check(ctx, candidatePath, rel.TagName); err != nil {
		return fmt.Errorf("preflight downloaded executable %q: %w", candidatePath, err)
	}
	return i.replaceExecutable(ctx, targetPath, candidatePath, workDir)
}

// prepareRelease validates the release identity and downloads/verifies its assets.
func (i *releaseInstaller) prepareRelease(ctx context.Context, rel release.Release) (selectedReleaseAssets, error) {
	version, err := release.ParseSemanticVersion(rel.TagName)
	if err != nil {
		return selectedReleaseAssets{}, fmt.Errorf("parse release tag %q: %w", rel.TagName, err)
	}
	if rel.Version == "" || rel.Version != version.String() {
		return selectedReleaseAssets{}, fmt.Errorf("release version %q does not match tag %q", rel.Version, rel.TagName)
	}
	assets, err := selectReleaseAssets(rel.Assets, releaseArchiveName(rel.Version, i.goos, i.goarch))
	if err != nil {
		return selectedReleaseAssets{}, err
	}
	if err := i.validateAssetURLs(rel, assets); err != nil {
		return selectedReleaseAssets{}, err
	}
	assetSources, err := i.assetSources(ctx, assets.checksums)
	if err != nil {
		return selectedReleaseAssets{}, err
	}
	checksums, err := i.download(ctx, assets.checksums, assetSources)
	if err != nil {
		return selectedReleaseAssets{}, fmt.Errorf("download %s: %w", checksumsAssetName, err)
	}
	manifestBytes, err := i.download(ctx, assets.manifest, assetSources)
	if err != nil {
		return selectedReleaseAssets{}, fmt.Errorf("download %s: %w", manifestAssetName, err)
	}
	if err := verifyChecksumsManifest(manifestBytes, checksums, i.trustedKeys); err != nil {
		return selectedReleaseAssets{}, err
	}
	if err := verifyArchiveChecksum(checksums, assets.archive.Name, nil); err != nil {
		return selectedReleaseAssets{}, err
	}
	assets.archivePayload, err = i.download(ctx, assets.archive, assetSources)
	if err != nil {
		return selectedReleaseAssets{}, fmt.Errorf("download release archive %q: %w", assets.archive.Name, err)
	}
	if err := source.CheckContext(ctx, "extract release archive"); err != nil {
		return selectedReleaseAssets{}, err
	}
	if err := verifyArchiveChecksum(checksums, assets.archive.Name, assets.archivePayload); err != nil {
		return selectedReleaseAssets{}, err
	}
	return assets, nil
}

// stageRelease locates the live binary and creates its sibling temporary workspace.
func (i *releaseInstaller) stageRelease(ctx context.Context) (string, string, error) {
	targetPath, err := i.executablePath()
	if err != nil {
		return "", "", fmt.Errorf("locate current executable: %w", err)
	}
	targetPath, err = resolveReleaseExecutablePath(targetPath)
	if err != nil {
		return "", "", err
	}
	workDir, err := os.MkdirTemp(filepath.Dir(targetPath), ".pixiv-update-")
	if err != nil {
		return "", "", fmt.Errorf("create update temporary directory beside %q: %w", targetPath, err)
	}
	return targetPath, workDir, nil
}

func (i *releaseInstaller) cleanupWorkDir(err *error, workDir string) {
	if *err == nil {
		return
	}
	if removeErr := os.RemoveAll(workDir); removeErr != nil && !os.IsNotExist(removeErr) {
		*err = errors.Join(*err, fmt.Errorf("remove update temporary directory %q: %w", workDir, removeErr))
	}
}

// replaceExecutable stages the verified binary next to the target and atomically replaces it.
func (i *releaseInstaller) replaceExecutable(ctx context.Context, targetPath, candidatePath, workDir string) (err error) {
	if err := source.CheckContext(ctx, "extract release archive"); err != nil {
		return err
	}
	if err := source.CheckContext(ctx, "stage verified update"); err != nil {
		return err
	}
	staged, err := os.CreateTemp(filepath.Dir(targetPath), ".pixiv-update-stage-")
	if err != nil {
		return fmt.Errorf("create staged update file beside %q: %w", targetPath, err)
	}
	stagedPath := staged.Name()
	defer func() {
		if err != nil && !atomicfile.MustPreserveReplacementSource(err) {
			if removeErr := os.Remove(stagedPath); removeErr != nil && !os.IsNotExist(removeErr) {
				err = errors.Join(err, fmt.Errorf("remove staged update file %q: %w", stagedPath, removeErr))
			}
		}
	}()
	if err := staged.Close(); err != nil {
		return fmt.Errorf("close staged update file %q: %w", stagedPath, err)
	}
	if err := source.CheckContext(ctx, "stage verified update"); err != nil {
		return err
	}
	if err := os.Remove(stagedPath); err != nil {
		return fmt.Errorf("prepare staged update file %q: %w", stagedPath, err)
	}
	if err := source.CheckContext(ctx, "stage verified update"); err != nil {
		return err
	}
	if err := os.Rename(candidatePath, stagedPath); err != nil {
		return fmt.Errorf("stage verified update file %q: %w", stagedPath, err)
	}
	if err := source.CheckContext(ctx, "replace release executable"); err != nil {
		return err
	}
	if err := os.Remove(workDir); err != nil {
		return fmt.Errorf("remove verified update temporary directory %q: %w", workDir, err)
	}
	if err := source.CheckContext(ctx, "replace release executable"); err != nil {
		return err
	}
	if err := (releaseFileReplacer{goos: i.goos}).Replace(stagedPath, targetPath); err != nil {
		return fmt.Errorf("atomically replace executable %q: %w", targetPath, err)
	}
	return nil
}

// resolveReleaseExecutablePath 把已有的可执行文件软链接收敛到真实目标。Lstat 失败时
// 保留绝对化后的原路径，交由调用方维持既有的文件操作与错误语义。
func resolveReleaseExecutablePath(executablePath string) (string, error) {
	targetPath, err := filepath.Abs(executablePath)
	if err != nil {
		return "", fmt.Errorf("resolve current executable %q: %w", executablePath, err)
	}
	fileInfo, lstatErr := os.Lstat(targetPath)
	if lstatErr != nil || fileInfo.Mode()&os.ModeSymlink == 0 {
		return targetPath, nil
	}
	resolvedTargetPath, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		return "", fmt.Errorf("resolve executable symlink %q: %w", targetPath, err)
	}
	targetPath, err = filepath.Abs(resolvedTargetPath)
	if err != nil {
		return "", fmt.Errorf("resolve executable symlink target %q: %w", resolvedTargetPath, err)
	}
	return targetPath, nil
}

type selectedReleaseAssets struct {
	archive        release.ReleaseAsset
	checksums      release.ReleaseAsset
	manifest       release.ReleaseAsset
	archivePayload []byte
}

// validateAssetURLs 必须在任何 sidecar 或 archive 请求前执行完全部验证，防止被篡改的
// Releases API 响应或 ETag cache 把安装器引向非项目控制的来源。
func (i *releaseInstaller) validateAssetURLs(rel release.Release, assets selectedReleaseAssets) error {
	for _, asset := range []release.ReleaseAsset{assets.archive, assets.checksums, assets.manifest} {
		if err := release.ValidateOfficialGitHubReleaseAssetURL(rel, asset); err != nil {
			return err
		}
	}
	return nil
}

func selectReleaseAssets(assets []release.ReleaseAsset, archiveName string) (selectedReleaseAssets, error) {
	var selected selectedReleaseAssets
	for _, asset := range assets {
		switch asset.Name {
		case archiveName:
			if selected.archive.Name != "" {
				return selectedReleaseAssets{}, fmt.Errorf("release contains duplicate archive asset %q", archiveName)
			}
			selected.archive = asset
		case checksumsAssetName:
			if selected.checksums.Name != "" {
				return selectedReleaseAssets{}, fmt.Errorf("release contains duplicate asset %q", checksumsAssetName)
			}
			selected.checksums = asset
		case manifestAssetName:
			if selected.manifest.Name != "" {
				return selectedReleaseAssets{}, fmt.Errorf("release contains duplicate asset %q", manifestAssetName)
			}
			selected.manifest = asset
		}
	}
	if selected.archive.Name == "" {
		return selectedReleaseAssets{}, fmt.Errorf("release has no platform archive asset %q", archiveName)
	}
	if selected.checksums.Name == "" {
		return selectedReleaseAssets{}, fmt.Errorf("release has no asset %q", checksumsAssetName)
	}
	if selected.manifest.Name == "" {
		return selectedReleaseAssets{}, fmt.Errorf("release has no asset %q", manifestAssetName)
	}
	for _, asset := range []release.ReleaseAsset{selected.archive, selected.checksums, selected.manifest} {
		if asset.DownloadURL == "" {
			return selectedReleaseAssets{}, fmt.Errorf("release asset %q has no download URL", asset.Name)
		}
	}
	return selected, nil
}

func releaseArchiveName(version, goos, goarch string) string {
	extension := ".tar.gz"
	if goos == "windows" {
		extension = ".zip"
	}
	return "pixiv-cli_" + version + "_" + goos + "_" + goarch + extension
}

func releaseBinaryName(goos string) string {
	if goos == "windows" {
		return "pixiv.exe"
	}
	return "pixiv"
}

func (i *releaseInstaller) assetSources(ctx context.Context, probe release.ReleaseAsset) ([]source.ReleaseSource, error) {
	if i.sourceSelector == nil {
		return nil, nil
	}
	sources, err := i.sourceSelector.Ordered(ctx, source.ReleaseSourceAsset, probe.DownloadURL)
	if err != nil {
		return nil, fmt.Errorf("select source for release assets: %w", err)
	}
	return sources, nil
}

func (i *releaseInstaller) download(ctx context.Context, asset release.ReleaseAsset, sources []source.ReleaseSource) ([]byte, error) {
	if len(sources) == 0 {
		return i.downloadURL(ctx, asset, asset.DownloadURL)
	}
	errorsBySource := make([]error, 0, len(sources))
	for _, releaseSource := range sources {
		requestURL, err := releaseSource.AssetURL(asset.DownloadURL)
		if err != nil {
			errorsBySource = append(errorsBySource, fmt.Errorf("release source %q: %w", releaseSource.ID(), err))
			continue
		}
		body, err := i.downloadURL(ctx, asset, requestURL)
		if err == nil {
			return body, nil
		}
		errorsBySource = append(errorsBySource, fmt.Errorf("release source %q: %w", releaseSource.ID(), err))
	}
	return nil, fmt.Errorf("download asset %q from every release source: %w", asset.Name, errors.Join(errorsBySource...))
}

func (i *releaseInstaller) downloadURL(ctx context.Context, asset release.ReleaseAsset, requestURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request for asset %q: %w", asset.Name, err)
	}
	response, err := i.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request asset %q: %w", asset.Name, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("asset %q returned HTTP %s", asset.Name, response.Status)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read asset %q: %w", asset.Name, err)
	}
	return body, nil
}

type processReleaseBinaryChecker struct{}

func (processReleaseBinaryChecker) Check(ctx context.Context, executablePath, expectedTag string) error {
	command := exec.CommandContext(ctx, executablePath, "--version")
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("run --version: %w", err)
	}
	want := []byte("pixiv " + expectedTag + "\n")
	if !bytes.Equal(output, want) {
		return fmt.Errorf("downloaded executable reports version output %q, want %q", output, want)
	}
	return nil
}

type releaseFileReplacer struct {
	goos string
}

func (r releaseFileReplacer) Replace(sourcePath, targetPath string) error {
	if r.goos == "windows" {
		return atomicfile.ReplaceFileWithBackup(sourcePath, targetPath, targetPath+".old")
	}
	return atomicfile.ReplaceFile(sourcePath, targetPath)
}
