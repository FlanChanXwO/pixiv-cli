package fanbox_test

import (
	accountfanbox "github.com/FlanChanXwO/pixiv-cli/internal/account/fanbox"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/config"
)

var _ accountfanbox.DefaultStore = testFanboxDefaults{}

type testFanboxDefaults struct{}

func (testFanboxDefaults) ReadFanboxDefaultUserID() (int64, bool, error) {
	return config.ReadFanboxDefaultUserID()
}

func (testFanboxDefaults) SetFanboxDefaultUserID(userID int64) error {
	return config.SetFanboxDefaultUserID(userID)
}

func (testFanboxDefaults) ClearFanboxDefaultUserID() error {
	return config.ClearFanboxDefaultUserID()
}
