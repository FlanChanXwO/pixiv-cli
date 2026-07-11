package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
	"github.com/FlanChanXwO/pixiv-cli/internal/utils/files"
)

const (
	checksumsAssetName = "checksums.txt"
	manifestAssetName  = "checksums.json"
)

// ReleaseBinaryChecker 在替换前验证下载二进制实际报告的版本。
// 实现必须只在 archive、checksum 和签名都已通过后调用二进制。
type ReleaseBinaryChecker interface {
	Check(context.Context, string, string) error
}

// ReleaseFileReplacer 抽象最终的同目录原子替换，供受控测试和平台实现复用。
type ReleaseFileReplacer interface {
	Replace(string, string) error
}

// ReleaseInstallerOptions 允许生产组装与受控测试分别注入信任根和系统依赖。
// TrustedKeys 只允许 Ed25519 公钥；私钥绝不能进入生产组装或此 API。
type ReleaseInstallerOptions struct {
	HTTPClient     *http.Client
	TrustedKeys    map[string]ed25519.PublicKey
	ExecutablePath func() (string, error)
	GOOS           string
	GOARCH         string
	BinaryChecker  ReleaseBinaryChecker
	Replacer       ReleaseFileReplacer
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
	goos := options.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := options.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	checker := options.BinaryChecker
	if checker == nil {
		checker = processReleaseBinaryChecker{}
	}
	replacer := options.Replacer
	if replacer == nil {
		replacer = releaseFileReplacer{goos: goos}
	}
	keys := make(map[string]ed25519.PublicKey, len(options.TrustedKeys))
	for keyID, publicKey := range options.TrustedKeys {
		keys[keyID] = append(ed25519.PublicKey(nil), publicKey...)
	}
	return &releaseInstaller{
		httpClient:        httpClient,
		trustedKeys:       keys,
		executablePath:    executablePath,
		goos:              goos,
		goarch:            goarch,
		checker:           checker,
		replacer:          replacer,
		assetURLValidator: validateOfficialGitHubReleaseAssetURL,
	}
}

type releaseInstaller struct {
	httpClient        *http.Client
	trustedKeys       map[string]ed25519.PublicKey
	executablePath    func() (string, error)
	goos              string
	goarch            string
	checker           ReleaseBinaryChecker
	replacer          ReleaseFileReplacer
	assetURLValidator func(Release, ReleaseAsset) error
}

// Install 下载并验证选中版本的唯一平台资产。任何下载、认证、解包、预检或替换失败
// 都发生在临时文件阶段，原可执行文件保持不变。
func (i *releaseInstaller) Install(ctx context.Context, release Release) (err error) {
	if i == nil {
		return fmt.Errorf("release installer is nil")
	}
	if len(i.trustedKeys) == 0 {
		return fmt.Errorf("trusted release signing key is not configured")
	}
	if err := checkContext(ctx, "prepare release update"); err != nil {
		return err
	}
	version, err := parseSemanticVersion(release.TagName)
	if err != nil {
		return fmt.Errorf("parse release tag %q: %w", release.TagName, err)
	}
	if release.Version == "" || release.Version != version.string() {
		return fmt.Errorf("release version %q does not match tag %q", release.Version, release.TagName)
	}
	assets, err := selectReleaseAssets(release.Assets, releaseArchiveName(release.Version, i.goos, i.goarch))
	if err != nil {
		return err
	}
	if err := i.validateAssetURLs(release, assets); err != nil {
		return err
	}
	checksums, err := i.download(ctx, assets.checksums)
	if err != nil {
		return fmt.Errorf("download %s: %w", checksumsAssetName, err)
	}
	manifestBytes, err := i.download(ctx, assets.manifest)
	if err != nil {
		return fmt.Errorf("download %s: %w", manifestAssetName, err)
	}
	if err := verifyChecksumsManifest(manifestBytes, checksums, i.trustedKeys); err != nil {
		return err
	}
	if err := verifyArchiveChecksum(checksums, assets.archive.Name, nil); err != nil {
		return err
	}
	archive, err := i.download(ctx, assets.archive)
	if err != nil {
		return fmt.Errorf("download release archive %q: %w", assets.archive.Name, err)
	}
	if err := verifyArchiveChecksum(checksums, assets.archive.Name, archive); err != nil {
		return err
	}
	targetPath, err := i.executablePath()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}
	targetPath, err = filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("resolve current executable %q: %w", targetPath, err)
	}
	workDir, err := os.MkdirTemp(filepath.Dir(targetPath), ".pixiv-update-")
	if err != nil {
		return fmt.Errorf("create update temporary directory beside %q: %w", targetPath, err)
	}
	defer func() {
		if err != nil {
			if removeErr := os.RemoveAll(workDir); removeErr != nil && !os.IsNotExist(removeErr) {
				err = errors.Join(err, fmt.Errorf("remove update temporary directory %q: %w", workDir, removeErr))
			}
		}
	}()
	candidatePath := filepath.Join(workDir, releaseBinaryName(i.goos))
	if err := extractReleaseBinary(archive, assets.archive.Name, candidatePath, releaseBinaryName(i.goos)); err != nil {
		return err
	}
	if err := i.checker.Check(ctx, candidatePath, release.TagName); err != nil {
		return fmt.Errorf("preflight downloaded executable %q: %w", candidatePath, err)
	}
	staged, err := os.CreateTemp(filepath.Dir(targetPath), ".pixiv-update-stage-")
	if err != nil {
		return fmt.Errorf("create staged update file beside %q: %w", targetPath, err)
	}
	stagedPath := staged.Name()
	defer func() {
		if err != nil {
			if removeErr := os.Remove(stagedPath); removeErr != nil && !os.IsNotExist(removeErr) {
				err = errors.Join(err, fmt.Errorf("remove staged update file %q: %w", stagedPath, removeErr))
			}
		}
	}()
	if err := staged.Close(); err != nil {
		return fmt.Errorf("close staged update file %q: %w", stagedPath, err)
	}
	if err := os.Remove(stagedPath); err != nil {
		return fmt.Errorf("prepare staged update file %q: %w", stagedPath, err)
	}
	if err := os.Rename(candidatePath, stagedPath); err != nil {
		return fmt.Errorf("stage verified update file %q: %w", stagedPath, err)
	}
	if err := os.Remove(workDir); err != nil {
		return fmt.Errorf("remove verified update temporary directory %q: %w", workDir, err)
	}
	if err := i.replacer.Replace(stagedPath, targetPath); err != nil {
		return fmt.Errorf("atomically replace executable %q: %w", targetPath, err)
	}
	return nil
}

type selectedReleaseAssets struct {
	archive   ReleaseAsset
	checksums ReleaseAsset
	manifest  ReleaseAsset
}

// validateAssetURLs 必须在任何 sidecar 或 archive 请求前执行完全部验证，防止被篡改的
// Releases API 响应或 ETag cache 把安装器引向非项目控制的来源。
func (i *releaseInstaller) validateAssetURLs(release Release, assets selectedReleaseAssets) error {
	for _, asset := range []ReleaseAsset{assets.archive, assets.checksums, assets.manifest} {
		if err := i.assetURLValidator(release, asset); err != nil {
			return err
		}
	}
	return nil
}

// validateOfficialGitHubReleaseAssetURL 只接受固定仓库、选中 tag 和精确 asset 名组成的
// GitHub HTTPS 下载端点。完整字符串相等也拒绝 query、fragment、userinfo、port 与任何
// encoded 或歧义 path；真正下载时仍交由 HTTP client 跟随 GitHub 的合法重定向。
func validateOfficialGitHubReleaseAssetURL(release Release, asset ReleaseAsset) error {
	expected := "https://github.com/" + defaultGitHubRepository + "/releases/download/" + release.TagName + "/" + asset.Name
	if asset.DownloadURL != expected {
		return fmt.Errorf("release asset %q has untrusted download URL %q: expected exact GitHub HTTPS release URL %q", asset.Name, asset.DownloadURL, expected)
	}
	return nil
}

func selectReleaseAssets(assets []ReleaseAsset, archiveName string) (selectedReleaseAssets, error) {
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
	for _, asset := range []ReleaseAsset{selected.archive, selected.checksums, selected.manifest} {
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

func (i *releaseInstaller) download(ctx context.Context, asset ReleaseAsset) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.DownloadURL, nil)
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

type checksumsManifest struct {
	KeyID           string `json:"key_id"`
	ChecksumsSHA256 string `json:"checksums_sha256"`
	Signature       string `json:"signature"`
}

func verifyChecksumsManifest(body, checksums []byte, trustedKeys map[string]ed25519.PublicKey) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var manifest checksumsManifest
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("decode checksums manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode checksums manifest: contains more than one JSON value")
		}
		return fmt.Errorf("decode checksums manifest trailing data: %w", err)
	}
	publicKey, ok := trustedKeys[manifest.KeyID]
	if !ok {
		return fmt.Errorf("checksums manifest references unknown key ID %q", manifest.KeyID)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("trusted public key %q has invalid length", manifest.KeyID)
	}
	hash := sha256.Sum256(checksums)
	if manifest.ChecksumsSHA256 != hex.EncodeToString(hash[:]) {
		return fmt.Errorf("checksums manifest SHA-256 does not match checksums.txt")
	}
	signature, err := base64.StdEncoding.DecodeString(manifest.Signature)
	if err != nil {
		return fmt.Errorf("decode checksums manifest signature: %w", err)
	}
	if !ed25519.Verify(publicKey, checksums, signature) {
		return fmt.Errorf("checksums manifest Ed25519 signature verification failed for key ID %q", manifest.KeyID)
	}
	return nil
}

func verifyArchiveChecksum(checksums []byte, filename string, archive []byte) error {
	var expected string
	for _, line := range strings.Split(string(checksums), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return fmt.Errorf("parse checksums.txt line %q", line)
		}
		if strings.TrimPrefix(fields[1], "*") != filename {
			continue
		}
		if expected != "" {
			return fmt.Errorf("checksums.txt has duplicate entry for %q", filename)
		}
		if len(fields[0]) != sha256.Size*2 || strings.ToLower(fields[0]) != fields[0] {
			return fmt.Errorf("checksums.txt has invalid SHA-256 for %q", filename)
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			return fmt.Errorf("checksums.txt has invalid SHA-256 for %q: %w", filename, err)
		}
		expected = fields[0]
	}
	if expected == "" {
		return fmt.Errorf("checksums.txt has no entry for %q", filename)
	}
	if archive == nil {
		return nil
	}
	actual := sha256.Sum256(archive)
	if expected != hex.EncodeToString(actual[:]) {
		return fmt.Errorf("release archive %q SHA-256 does not match checksums.txt", filename)
	}
	return nil
}

func extractReleaseBinary(archive []byte, archiveName, destination, binaryName string) error {
	if strings.HasSuffix(archiveName, ".tar.gz") {
		return extractTarGzBinary(archive, destination, binaryName)
	}
	if strings.HasSuffix(archiveName, ".zip") {
		return extractZipBinary(archive, destination, binaryName)
	}
	return fmt.Errorf("unsupported release archive %q", archiveName)
}

func extractTarGzBinary(archive []byte, destination, binaryName string) error {
	if err := validateTarGzArchivePaths(archive); err != nil {
		return err
	}
	compressed, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("open gzip release archive: %w", err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	found := false
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar release archive: %w", err)
		}
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			return fmt.Errorf("release archive contains link entry %q", header.Name)
		}
		if path.Base(header.Name) != binaryName {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return fmt.Errorf("release archive binary %q is not a regular file", header.Name)
		}
		if found {
			return fmt.Errorf("release archive contains duplicate binary %q", binaryName)
		}
		if err := writeExtractedBinary(destination, reader); err != nil {
			return err
		}
		found = true
	}
	if !found {
		return fmt.Errorf("release archive has no regular binary %q", binaryName)
	}
	return nil
}

// validateTarGzArchivePaths 在任何候选文件写入前扫描整个 tar stream，防止后置的
// 不安全路径在合法二进制已经写出后才被发现。
func validateTarGzArchivePaths(archive []byte) error {
	compressed, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("open gzip release archive: %w", err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar release archive: %w", err)
		}
		if err := validateArchivePath(header.Name); err != nil {
			return err
		}
	}
}

func extractZipBinary(archive []byte, destination, binaryName string) error {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return fmt.Errorf("open zip release archive: %w", err)
	}
	for _, entry := range reader.File {
		if err := validateArchivePath(entry.Name); err != nil {
			return err
		}
	}
	found := false
	for _, entry := range reader.File {
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("release archive contains link entry %q", entry.Name)
		}
		if path.Base(entry.Name) != binaryName {
			continue
		}
		if entry.FileInfo().IsDir() || !entry.Mode().IsRegular() {
			return fmt.Errorf("release archive binary %q is not a regular file", entry.Name)
		}
		if found {
			return fmt.Errorf("release archive contains duplicate binary %q", binaryName)
		}
		body, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open release archive binary %q: %w", entry.Name, err)
		}
		writeErr := writeExtractedBinary(destination, body)
		closeErr := body.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return fmt.Errorf("close release archive binary %q: %w", entry.Name, closeErr)
		}
		found = true
	}
	if !found {
		return fmt.Errorf("release archive has no regular binary %q", binaryName)
	}
	return nil
}

func validateArchivePath(name string) error {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("release archive has unsafe path %q", name)
	}
	// 必须在 path.Clean 前检查原始 segment，避免 `nested/../pixiv` 被归一化后
	// 伪装成可接受的候选二进制路径。
	for _, segment := range strings.Split(name, "/") {
		if segment == ".." {
			return fmt.Errorf("release archive has unsafe path %q", name)
		}
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("release archive has unsafe path %q", name)
	}
	return nil
}

func writeExtractedBinary(destination string, source io.Reader) (err error) {
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return fmt.Errorf("create extracted release binary %q: %w", destination, err)
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close extracted release binary %q: %w", destination, closeErr)
		}
	}()
	if _, err := io.Copy(file, source); err != nil {
		return fmt.Errorf("write extracted release binary %q: %w", destination, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync extracted release binary %q: %w", destination, err)
	}
	if err := file.Chmod(0o755); err != nil {
		return fmt.Errorf("set extracted release binary permissions %q: %w", destination, err)
	}
	return nil
}

type processReleaseBinaryChecker struct{}

func (processReleaseBinaryChecker) Check(ctx context.Context, executablePath, expectedTag string) error {
	command := exec.CommandContext(ctx, executablePath, "version", "--json")
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("run version --json: %w", err)
	}
	var info buildinfo.Info
	if err := json.Unmarshal(output, &info); err != nil {
		return fmt.Errorf("decode version --json output: %w", err)
	}
	if info.Version != expectedTag {
		return fmt.Errorf("downloaded executable reports version %q, want %q", info.Version, expectedTag)
	}
	return nil
}

type releaseFileReplacer struct {
	goos string
}

func (r releaseFileReplacer) Replace(sourcePath, targetPath string) error {
	if r.goos == "windows" {
		return files.ReplaceFileWithBackup(sourcePath, targetPath, targetPath+".old")
	}
	return files.ReplaceFile(sourcePath, targetPath)
}
