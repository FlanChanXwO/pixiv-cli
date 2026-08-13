package pixiv_test

import "github.com/FlanChanXwO/pixiv-cli/internal/storage/config"

type testPixivDefaults struct{}

func (testPixivDefaults) ReadPixivDefaultUserID() (int64, bool, error) {
	return config.ReadPixivDefaultUserID()
}

func (testPixivDefaults) SetPixivDefaultUserID(userID int64) error {
	return config.SetPixivDefaultUserID(userID)
}

func (testPixivDefaults) ClearPixivDefaultUserID() error {
	return config.ClearPixivDefaultUserID()
}
