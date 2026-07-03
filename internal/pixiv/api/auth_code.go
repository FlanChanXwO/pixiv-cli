package api

import (
	"context"
	"errors"
	"net/http"
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

	form := map[string]string{
		"client_id":      DefaultOAuthClientID,
		"client_secret":  DefaultOAuthClientSecret,
		"grant_type":     "authorization_code",
		"include_policy": "true",
		"code":           code,
		"code_verifier":  codeVerifier,
		"redirect_uri":   DefaultOAuthRedirectURI,
	}

	var result authResponse
	if err := c.doJSON(ctx, http.MethodPost, c.oauthBase+"/auth/token", requestOptions{
		Headers: oauthHeaders(),
		Form:    form,
	}, &result); err != nil {
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
