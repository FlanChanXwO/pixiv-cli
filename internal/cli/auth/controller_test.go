package auth

import (
	"testing"

	"github.com/spf13/cobra"
)

// A --proxy value must reach the request; binding to a temporary struct used
// to drop it silently and connect directly.
func TestLoginProxyFlagReachesRequest(t *testing.T) {
	opts := accountLoginOptions{}
	cmd := &cobra.Command{Use: "login"}
	controller{}.bindProxyFlags(cmd, &opts.ProxyOptions)
	if err := cmd.Flags().Parse([]string{"--proxy", "http://127.0.0.1:7890"}); err != nil {
		t.Fatal(err)
	}
	override, err := proxyOverrideFromFlags(cmd, opts.ProxyOptions)
	if err != nil {
		t.Fatal(err)
	}
	if override == nil || *override != "http://127.0.0.1:7890" {
		t.Fatalf("proxy override = %v, want the flag value", override)
	}
}
