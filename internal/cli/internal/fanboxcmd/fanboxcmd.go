// Package fanboxcmd owns the wiring shared by FANBOX subcommands: proxy parsing
// and the client lease boundary. It holds no aggregate dependency type.

package fanboxcmd

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/cli/internal/command"
	"github.com/FlanChanXwO/pixiv-cli/internal/lifecycle"
	"github.com/FlanChanXwO/pixiv-cli/internal/session"
	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
	"github.com/spf13/cobra"
)

// ProxyOverride reads the FANBOX group's persistent --proxy/--no-proxy flags.
func ProxyOverride(cmd *cobra.Command) (*string, error) {
	proxy, _ := cmd.Flags().GetString("proxy")
	noProxy, _ := cmd.Flags().GetBool("no-proxy")
	return command.ProxyOverride(cmd, command.ProxyOptions{Proxy: proxy, NoProxy: noProxy})
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
	return service.Open(ctx, session.OpenRequest{ProxyOverride: proxyOverride})
}

// UseClient keeps the lease boundary local to one command callback and
// preserves both callback and close errors.
func UseClient(cmd *cobra.Command, open func() (*session.Fanbox, error), use func(*fanbox.Client) error) error {
	lease, err := Open(cmd.Context(), open, cmd)
	if err != nil {
		return err
	}
	useErr := use(lease.Value())
	return errors.Join(useErr, lease.Close())
}
