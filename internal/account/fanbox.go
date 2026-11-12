package account

import (
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
)

// Fanbox 保存 FANBOX 身份与独立的 session 凭据；所有时间为 UTC。
type Fanbox struct {
	UserID             int64             `json:"user_id"`
	DisplayName        string            `json:"display_name"`
	CreatorID          string            `json:"creator_id"`
	CredentialRevision int64             `json:"revision"`
	ValidatedAt        time.Time         `json:"validated_at"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	Credential         credential.Secret `json:"-"`
}
