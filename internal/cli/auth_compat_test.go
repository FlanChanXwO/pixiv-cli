package cli

import (
	"io"
	"net/url"

	authcommands "github.com/FlanChanXwO/pixiv-cli/internal/cli/auth"
)

// Root-package integration tests keep their historical unexported spellings;
// these aliases route their decoded DTOs and pure login assertions to auth.
type accountOut = authcommands.AccountOut
type accountListOut = authcommands.AccountListOut
type accountPoolStatusOut = authcommands.AccountPoolStatusOut
type authBundleFileReadError = authcommands.AuthBundleFileReadError

func valueOrFalse(value *bool) bool { return authcommands.ValueOrFalse(value) }

func readRefreshTokenInput(reader io.Reader) (string, error) {
	return authcommands.ReadRefreshTokenInput(reader)
}

func writeAuthExportStdout(writer io.Writer, body []byte) error {
	return authcommands.WriteAuthExportStdout(writer, body)
}

func classifyAuthBundleFileReadError(path string, err error) error {
	return authcommands.ClassifyAuthBundleFileReadErrorForCLI(path, err)
}

type loginServerResult struct {
	code string
	err  error
}

type loginInputResult struct {
	loginServerResult
	relayed  bool
	relayURL string
}

func loginCodeFromInput(raw string, accepts func(string) bool) loginServerResult {
	result := authcommands.LoginCodeFromInput(raw, accepts)
	return loginServerResult{code: result.Code, err: result.Err}
}

func loginInputFromText(raw string, accepts func(string) bool, challenge string, openRelay func(string) error) loginInputResult {
	result := authcommands.LoginInputFromText(raw, accepts, challenge, openRelay)
	return loginInputResult{loginServerResult: loginServerResult{code: result.Code, err: result.Err}, relayed: result.Relayed, relayURL: result.RelayURL}
}

func pixivPostRedirectReturnTo(raw string) (string, bool) {
	return authcommands.PixivPostRedirectReturnTo(raw)
}

func pixivAuthStartMatchesChallenge(raw, challenge string) bool {
	return authcommands.PixivAuthStartMatchesChallenge(raw, challenge)
}

func isBrowserCallbackURL(parsed *url.URL) bool {
	return authcommands.IsBrowserCallbackURL(parsed)
}

func loginSSHTunnelCommand(address string) (string, error) {
	return authcommands.LoginSSHTunnelCommand(address)
}

func setOpenBrowserForTest(opener func(string) error) func() {
	return authcommands.SetOpenBrowserForCLI(opener)
}
