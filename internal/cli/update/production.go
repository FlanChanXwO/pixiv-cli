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
	httpClient, err := httpclient.HTTPClient(proxy)
	if err != nil {
		return nil, fmt.Errorf("parse update proxy URL: %w", err)
	}
	usePublicReleaseSources := proxy == ""
	releaseClient, err := selfupdate.NewGitHubReleaseClient(selfupdate.ReleaseClientOptions{HTTPClient: httpClient, EnablePublicReleaseSources: usePublicReleaseSources})
	if err != nil {
		return nil, fmt.Errorf("create GitHub release client: %w", err)
	}
	return selfupdate.NewUpdateCoordinator(selfupdate.UpdateCoordinatorOptions{
		SourceDetector:   selfupdate.SourceDetectorFunc(selfupdate.DetectInstallSource),
		ReleaseChecker:   releaseClient,
		CommandRunner:    selfupdate.NewCommandRunner(out, errOut),
		ReleaseInstaller: selfupdate.NewReleaseInstaller(withPublicReleaseSources(productionReleaseInstallerOptions(httpClient), usePublicReleaseSources)),
	})
}

// NewAutomaticChecker 构造生产用 update.AutomaticUpdateChecker。
func NewAutomaticChecker(proxy string) (*selfupdate.AutomaticUpdateChecker, error) {
	httpClient, err := httpclient.HTTPClient(proxy)
	if err != nil {
		return nil, fmt.Errorf("parse update proxy URL: %w", err)
	}
	usePublicReleaseSources := proxy == ""
	releaseClient, err := selfupdate.NewGitHubReleaseClient(selfupdate.ReleaseClientOptions{HTTPClient: httpClient, EnablePublicReleaseSources: usePublicReleaseSources})
	if err != nil {
		return nil, fmt.Errorf("create GitHub release client: %w", err)
	}
	return selfupdate.NewAutomaticUpdateChecker(selfupdate.AutomaticUpdateCheckerOptions{
		SourceDetector: selfupdate.SourceDetectorFunc(selfupdate.DetectInstallSource),
		ReleaseChecker: releaseClient,
	})
}

func withPublicReleaseSources(options selfupdate.ReleaseInstallerOptions, enabled bool) selfupdate.ReleaseInstallerOptions {
	options.EnablePublicReleaseSources = enabled
	return options
}

func productionReleaseInstallerOptions(httpClient *http.Client) selfupdate.ReleaseInstallerOptions {
	return selfupdate.ReleaseInstallerOptions{
		HTTPClient: httpClient,
		TrustedKeys: map[string]ed25519.PublicKey{
			selfupdate.ReleaseSigningKeyID: append(ed25519.PublicKey(nil), selfupdate.ReleaseSigningPublicKey[:]...),
		},
	}
}
