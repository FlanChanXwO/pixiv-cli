package update

import (
	"crypto/ed25519"
	"fmt"
	"io"
	"net/http"

	"github.com/FlanChanXwO/pixiv-cli/internal/httpclient"
	"github.com/FlanChanXwO/pixiv-cli/internal/selfupdate"
)

// NewCoordinator 构造生产用 update.UpdateCoordinator。
func NewCoordinator(proxy string, out, errOut io.Writer) (*selfupdate.UpdateCoordinator, error) {
	httpClient, releaseClient, err := newReleaseClient(proxy)
	if err != nil {
		return nil, err
	}
	return selfupdate.NewUpdateCoordinator(selfupdate.UpdateCoordinatorOptions{
		SourceDetector: selfupdate.SourceDetectorFunc(selfupdate.DetectInstallSource),
		ReleaseChecker: releaseClient,
		CommandRunner:  selfupdate.NewCommandRunner(out, errOut),
		ReleaseInstaller: selfupdate.NewReleaseInstaller(selfupdate.ReleaseInstallerOptions{
			HTTPClient: httpClient,
			TrustedKeys: map[string]ed25519.PublicKey{
				selfupdate.ReleaseSigningKeyID: append(ed25519.PublicKey(nil), selfupdate.ReleaseSigningPublicKey[:]...),
			},
			EnablePublicReleaseSources: proxy == "",
		}),
	})
}

// NewAutomaticChecker 构造生产用 update.AutomaticUpdateChecker。
func NewAutomaticChecker(proxy string) (*selfupdate.AutomaticUpdateChecker, error) {
	_, releaseClient, err := newReleaseClient(proxy)
	if err != nil {
		return nil, err
	}
	return selfupdate.NewAutomaticUpdateChecker(selfupdate.AutomaticUpdateCheckerOptions{
		SourceDetector: selfupdate.SourceDetectorFunc(selfupdate.DetectInstallSource),
		ReleaseChecker: releaseClient,
	})
}

// newReleaseClient 只有在没有显式代理时才启用公共 release 镜像源。
func newReleaseClient(proxy string) (*http.Client, selfupdate.ReleaseChecker, error) {
	httpClient, err := httpclient.HTTPClient(proxy)
	if err != nil {
		return nil, nil, fmt.Errorf("parse update proxy URL: %w", err)
	}
	releaseClient, err := selfupdate.NewGitHubReleaseClient(selfupdate.ReleaseClientOptions{HTTPClient: httpClient, EnablePublicReleaseSources: proxy == ""})
	if err != nil {
		return nil, nil, fmt.Errorf("create GitHub release client: %w", err)
	}
	return httpClient, releaseClient, nil
}
