package update

import (
	"crypto/ed25519"
	"fmt"
	"io"
	"net/http"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/network"
	updateapp "github.com/FlanChanXwO/pixiv-cli/internal/update"
)

// NewCoordinator 构造生产用 update.UpdateCoordinator。
func NewCoordinator(proxy string, out, errOut io.Writer) (*updateapp.UpdateCoordinator, error) {
	httpClient, err := network.HTTPClient(proxy)
	if err != nil {
		return nil, fmt.Errorf("parse update proxy URL: %w", err)
	}
	usePublicReleaseSources := proxy == ""
	releaseClient, err := updateapp.NewGitHubReleaseClient(updateapp.ReleaseClientOptions{HTTPClient: httpClient, EnablePublicReleaseSources: usePublicReleaseSources})
	if err != nil {
		return nil, fmt.Errorf("create GitHub release client: %w", err)
	}
	return updateapp.NewUpdateCoordinator(updateapp.UpdateCoordinatorOptions{
		SourceDetector:   updateapp.SourceDetectorFunc(updateapp.DetectInstallSource),
		ReleaseChecker:   releaseClient,
		CommandRunner:    updateapp.NewCommandRunner(out, errOut),
		ReleaseInstaller: updateapp.NewReleaseInstaller(withPublicReleaseSources(productionReleaseInstallerOptions(httpClient), usePublicReleaseSources)),
	})
}

// NewAutomaticChecker 构造生产用 update.AutomaticUpdateChecker。
func NewAutomaticChecker(proxy string) (*updateapp.AutomaticUpdateChecker, error) {
	httpClient, err := network.HTTPClient(proxy)
	if err != nil {
		return nil, fmt.Errorf("parse update proxy URL: %w", err)
	}
	usePublicReleaseSources := proxy == ""
	releaseClient, err := updateapp.NewGitHubReleaseClient(updateapp.ReleaseClientOptions{HTTPClient: httpClient, EnablePublicReleaseSources: usePublicReleaseSources})
	if err != nil {
		return nil, fmt.Errorf("create GitHub release client: %w", err)
	}
	return updateapp.NewAutomaticUpdateChecker(updateapp.AutomaticUpdateCheckerOptions{
		SourceDetector: updateapp.SourceDetectorFunc(updateapp.DetectInstallSource),
		ReleaseChecker: releaseClient,
	})
}

func withPublicReleaseSources(options updateapp.ReleaseInstallerOptions, enabled bool) updateapp.ReleaseInstallerOptions {
	options.EnablePublicReleaseSources = enabled
	return options
}

func productionReleaseInstallerOptions(httpClient *http.Client) updateapp.ReleaseInstallerOptions {
	return updateapp.ReleaseInstallerOptions{
		HTTPClient: httpClient,
		TrustedKeys: map[string]ed25519.PublicKey{
			updateapp.ReleaseSigningKeyID: append(ed25519.PublicKey(nil), updateapp.ReleaseSigningPublicKey[:]...),
		},
	}
}
