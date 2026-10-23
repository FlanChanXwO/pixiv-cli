// Package system registers every browser cookie provider behind the
// protocol-neutral browsercookies root, so one import can create any supported
// browser provider. The root stays provider-agnostic.
package system

import (
	// Importing the providers runs their init registration.
	_ "github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/chromium"
	_ "github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/firefox"
	_ "github.com/FlanChanXwO/pixiv-cli/internal/browsercookies/safari"

	"github.com/FlanChanXwO/pixiv-cli/internal/browsercookies"
)

// New creates a registered provider by browser name.
func New(browser string) (browsercookies.Provider, error) { return browsercookies.New(browser) }
