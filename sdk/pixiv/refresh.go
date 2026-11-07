package pixiv

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/api/oauth"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func (c *Client) credentials(ctx context.Context, rejected Credentials) (Credentials, error) {
	// The gate coordinates this client's refreshes; store methods release their
	// file/transaction locks before OAuth. Waiters can cancel independently.
	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return Credentials{}, newError("credentials", sdk.LocalStateError, "credential preparation canceled", ctx.Err())
	}
	defer func() { <-c.gate }()
	for {
		if err := ctx.Err(); err != nil {
			return Credentials{}, newError("credentials", sdk.LocalStateError, "credential preparation canceled", err)
		}
		// 1. Read the current revision before using or renewing its access token.
		latest, err := c.store.Load(ctx)
		if err != nil {
			return Credentials{}, newError("credentials.Load", sdk.LocalStateError, "credential read failed", err)
		}
		if err := validateCredentials(latest); err != nil {
			return Credentials{}, err
		}
		if err := c.bindIdentity(latest.UserID); err != nil {
			return Credentials{}, err
		}
		if c.cached.Revision == latest.Revision && c.cached.usable() {
			latest = c.cached
		}
		if latest.usable() && (rejected.AccessToken.Empty() || latest.Revision != rejected.Revision || latest.AccessToken.Value() != rejected.AccessToken.Value()) {
			c.cached = latest
			return latest, nil
		}
		// 2. Exchange credentials after Load has released its storage lock.
		rotated, err := c.refresh(ctx, latest)
		if err != nil {
			if ctx.Err() != nil {
				return Credentials{}, err
			}
			// A concurrent process can rotate the token while OAuth rejects ours.
			current, loadErr := c.store.Load(ctx)
			if loadErr != nil {
				return Credentials{}, newError("credentials.Load", sdk.LocalStateError, "could not reload credentials after refresh failed", errors.Join(err, loadErr))
			}
			if current.Revision > latest.Revision {
				continue
			}
			return Credentials{}, err
		}
		// 3. Publish the token only after persistence wins the revision comparison.
		swapped, err := c.store.CompareAndSwap(ctx, latest.Revision, rotated)
		if err != nil {
			return Credentials{}, newError("credentials.CompareAndSwap", sdk.LocalStateError, "credential save failed", err)
		}
		if !swapped {
			slog.DebugContext(ctx, "Credential revision changed; loading the saved credentials")
			continue
		}
		c.cached = rotated
		slog.DebugContext(ctx, "Refreshed credentials saved")
		return rotated, nil
	}
}

func (c *Client) refresh(ctx context.Context, previous Credentials) (Credentials, error) {
	if previous.RefreshToken.Empty() {
		return Credentials{}, newError("refresh", sdk.CredentialsExpired, "refresh token is required to renew credentials", nil)
	}
	if previous.Revision == math.MaxInt64 {
		return Credentials{}, newError("refresh", sdk.LocalStateError, "credential revision cannot advance", nil)
	}
	auth := oauth.New(previous.RefreshToken.Value(), oauth.Config{HTTPClient: c.http, Logger: protocolLogger()})
	if err := auth.Refresh(ctx); err != nil {
		return Credentials{}, classifyError("refresh", err, true)
	}
	if strings.TrimSpace(auth.AccessToken()) == "" {
		return Credentials{}, newError("refresh", sdk.MalformedUpstreamResponse, "OAuth response is missing an access token", nil)
	}
	if auth.UserID() <= 0 {
		return Credentials{}, newError("refresh", sdk.MalformedUpstreamResponse, "OAuth response is missing account identity", nil)
	}
	if err := c.bindIdentity(auth.UserID()); err != nil {
		return Credentials{}, err
	}
	result := Credentials{UserID: auth.UserID(), DisplayName: auth.UserName(), Revision: previous.Revision + 1, ExpiresAt: auth.Expiry()}
	result.SetTokens(auth.AccessToken(), auth.RefreshTokenValue())
	if err := validateCredentials(result); err != nil {
		return Credentials{}, err
	}
	return result, nil
}
