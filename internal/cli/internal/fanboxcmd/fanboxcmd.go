// Package fanboxcmd owns the wiring shared by FANBOX subcommands: proxy parsing
// and the client lease boundary. It holds no aggregate dependency type.

package fanboxcmd

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/spf13/cobra"
)

// ProxyOverride reads the FANBOX group's persistent --proxy/--no-proxy flags.
func ProxyOverride(cmd *cobra.Command) (*string, error) {
	if cmd == nil {
		return nil, nil
	}
	proxyFlag := cmd.Flags().Lookup("proxy")
	noProxyFlag := cmd.Flags().Lookup("no-proxy")
	proxyChanged := cmd.Flags().Changed("proxy")
	noProxyChanged := cmd.Flags().Changed("no-proxy")
	if proxyChanged && noProxyChanged {
		return nil, errors.New("use either --proxy or --no-proxy, not both")
	}
	if noProxyChanged && noProxyFlag != nil && noProxyFlag.Value.String() == "true" {
		empty := ""
		return &empty, nil
	}
	if proxyChanged && proxyFlag != nil {
		value := proxyFlag.Value.String()
		return &value, nil
	}
	return nil, nil
}

// Open opens one FANBOX client lease for the current command. The session is
// resolved lazily so building the command tree opens no account store.
func Open(ctx context.Context, open func() (*session.Fanbox, error), cmd *cobra.Command) (*lifecycle.Lease[*fanbox.Client], error) {
	proxyOverride, err := ProxyOverride(cmd)
	if err != nil {
		return nil, err
	}
	service, err := open()
	if err != nil {
		return nil, err
	}
	if service == nil {
		return nil, errors.New("fanbox service is not configured")
	}
	return service.Open(ctx, session.OpenRequest{ProxyOverride: proxyOverride})
}

// UseClient keeps the lease boundary local to one command callback and
// preserves both callback and close errors.
func UseClient(cmd *cobra.Command, open func() (*session.Fanbox, error), use func(*fanbox.Client) error) error {
	lease, err := Open(cmd.Context(), open, cmd)
	if err != nil {
		return err
	}
	if lease == nil {
		return errors.New("fanbox client lease is nil")
	}
	useErr := use(lease.Value())
	return errors.Join(useErr, lease.Close())
}

// BrowserProvider reads one FANBOXSESSID from a browser profile.
type BrowserProvider interface {
	ReadSession(context.Context, string, string) (string, error)
}
