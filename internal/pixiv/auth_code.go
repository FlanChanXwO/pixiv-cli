package pixiv

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

const DefaultOAuthRedirectURI = "https://app-api.pixiv.net/web/v1/users/auth/pixiv/callback"

type AuthCodeToken struct {
	AccessToken  string
	RefreshToken string
	UserID       int64
}

func (c *Client) ExchangeAuthorizationCode(ctx context.Context, code, codeVerifier string) (AuthCodeToken, error) {
	code = strings.TrimSpace(code)
	codeVerifier = strings.TrimSpace(codeVerifier)
	if code == "" {
		return AuthCodeToken{}, errors.New("authorization code cannot be empty")
	}
	if codeVerifier == "" {
		return AuthCodeToken{}, errors.New("code verifier cannot be empty")
	}

	form := url.Values{}
	form.Set("client_id", DefaultOAuthClientID)
	form.Set("client_secret", DefaultOAuthClientSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("include_policy", "true")
	form.Set("code", code)
	form.Set("code_verifier", codeVerifier)
	form.Set("redirect_uri", DefaultOAuthRedirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauthBase+"/auth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return AuthCodeToken{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("App-OS", DefaultAppOS)
	req.Header.Set("App-OS-Version", DefaultAppOSVersion)
	req.Header.Set("App-Version", DefaultAppVersion)

	var result authResponse
	if err := c.doJSON(req, &result); err != nil {
		return AuthCodeToken{}, err
	}
	out := authCodeTokenFromResponse(result)
	if out.RefreshToken == "" {
		return AuthCodeToken{}, errors.New("authorization_code response did not include refresh_token")
	}

	c.mu.Lock()
	c.accessToken = out.AccessToken
	c.refreshToken = out.RefreshToken
	c.userID = out.UserID
	c.mu.Unlock()
	return out, nil
}

func authCodeTokenFromResponse(result authResponse) AuthCodeToken {
	if result.Response.RefreshToken != "" || result.Response.AccessToken != "" || result.Response.User.ID != 0 {
		return AuthCodeToken{
			AccessToken:  result.Response.AccessToken,
			RefreshToken: result.Response.RefreshToken,
			UserID:       int64(result.Response.User.ID),
		}
	}
	return AuthCodeToken{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		UserID:       int64(result.User.ID),
	}
}
