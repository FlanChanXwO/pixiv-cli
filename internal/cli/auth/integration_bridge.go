package auth

import (
	"io"
	"net/url"
)

// These aliases keep package-level CLI integration tests focused on the public
// command wiring while the implementation lives in this command package.
type AccountOut = accountOut
type AccountListOut = accountListOut
type AccountPoolStatusOut = accountPoolStatusOut

func ValueOrFalse(value *bool) bool { return valueOrFalse(value) }

func ReadRefreshTokenInput(reader io.Reader) (string, error) {
	return readRefreshTokenInput(reader)
}

func WriteAuthExportStdout(writer io.Writer, body []byte) error {
	return writeAuthExportStdout(writer, body)
}

func ClassifyAuthBundleFileReadErrorForCLI(path string, err error) error {
	return ClassifyAuthBundleFileReadError(path, err)
}

// CLIInputResult is the stable test-facing projection of login input parsing;
// internal parser state remains private to the auth command package.
type CLIInputResult struct {
	Code     string
	Err      error
	Relayed  bool
	RelayURL string
}

func LoginCodeFromInput(raw string, accepts func(string) bool) CLIInputResult {
	result := loginCodeFromInput(raw, accepts)
	return CLIInputResult{Code: result.code, Err: result.err}
}

func LoginInputFromText(raw string, accepts func(string) bool, challenge string, openRelay func(string) error) CLIInputResult {
	result := loginInputFromText(raw, accepts, challenge, openRelay)
	return CLIInputResult{Code: result.code, Err: result.err, Relayed: result.relayed, RelayURL: result.relayURL}
}

func PixivPostRedirectReturnTo(raw string) (string, bool) {
	return pixivPostRedirectReturnTo(raw)
}

func PixivAuthStartMatchesChallenge(raw, challenge string) bool {
	return pixivAuthStartMatchesChallenge(raw, challenge)
}

func IsBrowserCallbackURL(parsed *url.URL) bool { return isBrowserCallbackURL(parsed) }

func LoginSSHTunnelCommand(address string) (string, error) {
	return loginSSHTunnelCommand(address)
}

func SetOpenBrowserForCLI(opener func(string) error) func() {
	return setOpenBrowserForTest(opener)
}
