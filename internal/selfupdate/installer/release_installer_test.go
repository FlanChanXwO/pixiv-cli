package installer_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/selfupdate/installer"
	"github.com/FlanChanXwO/pixiv-cli/internal/selfupdate/release"
	"github.com/stretchr/testify/require"
)

func TestChecksumFailurePreservesInstalledBinary(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	archive := fmt.Sprintf("pixiv-cli_1.2.3_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		archive = strings.TrimSuffix(archive, ".tar.gz") + ".zip"
	}
	checksums := []byte(strings.Repeat("0", 64) + "  " + archive + "\n")
	sum := sha256.Sum256(checksums)
	manifest, err := json.Marshal(map[string]string{"key_id": "fixture", "checksums_sha256": fmt.Sprintf("%x", sum), "signature": base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, checksums))})
	require.NoError(t, err)
	payloads := map[string][]byte{"checksums.txt": checksums, "checksums.json": manifest, archive: []byte("tampered archive")}
	api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(payloads[filepath.Base(r.URL.Path)]); err != nil {
			t.Error(err)
		}
	}))
	defer api.Close()
	httpClient := api.Client()
	transport := httpClient.Transport.(*http.Transport)
	transport.TLSClientConfig.ServerName = api.Listener.Addr().(*net.TCPAddr).IP.String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, api.Listener.Addr().String())
	}
	target := filepath.Join(t.TempDir(), "pixiv")
	require.NoError(t, os.WriteFile(target, []byte("old executable"), 0o700))
	updater := installer.NewReleaseInstaller(installer.ReleaseInstallerOptions{HTTPClient: httpClient, TrustedKeys: map[string]ed25519.PublicKey{"fixture": publicKey}, ExecutablePath: func() (string, error) { return target, nil }})
	rel := release.Release{TagName: "v1.2.3", Version: "1.2.3"}
	for _, name := range []string{archive, "checksums.txt", "checksums.json"} {
		rel.Assets = append(rel.Assets, release.ReleaseAsset{Name: name, DownloadURL: "https://github.com/FlanChanXwO/pixiv-cli/releases/download/v1.2.3/" + name})
	}
	require.ErrorContains(t, updater.Install(t.Context(), rel), "release archive "+fmt.Sprintf("%q", archive)+" SHA-256 does not match checksums.txt")
	body, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "old executable", string(body))
}
