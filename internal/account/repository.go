package account

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/credential"
)

var (
	// ErrPixivNotFound 表示本地没有请求的 Pixiv 账号。
	ErrPixivNotFound = errors.New("pixiv account not found")
	// ErrFanboxNotFound 表示本地没有请求的 FANBOX 账号。
	ErrFanboxNotFound = errors.New("fanbox account not found")
	// ErrCredentialConflict 表示保存轮换凭据时 revision 已被其他操作更新。
	ErrCredentialConflict = errors.New("pixiv account credential revision conflict")
)

func (s *Store) update(ctx context.Context, change func(*document) error) error {
	return s.withLock(ctx, func() error {
		doc, err := s.read(ctx)
		if err != nil {
			return err
		}
		if err := change(&doc); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return s.write(doc)
	})
}

// GetPixiv 读取指定账号的独立快照；不存在时返回 ErrPixivNotFound。
func (s *Store) GetPixiv(ctx context.Context, userID int64) (Pixiv, error) {
	accounts, err := s.ListPixiv(ctx)
	if err != nil {
		return Pixiv{}, err
	}
	for _, a := range accounts {
		if a.UserID == userID {
			return a, nil
		}
	}
	return Pixiv{}, ErrPixivNotFound
}

// GetFanbox 读取指定账号的独立快照；不存在时返回 ErrFanboxNotFound。
func (s *Store) GetFanbox(ctx context.Context, userID int64) (Fanbox, error) {
	accounts, err := s.ListFanbox(ctx)
	if err != nil {
		return Fanbox{}, err
	}
	for _, a := range accounts {
		if a.UserID == userID {
			return a, nil
		}
	}
	return Fanbox{}, ErrFanboxNotFound
}

// SavePixivCredential 保存已验证的凭据；新账号追加到末尾，已有账号递增 revision。
func (s *Store) SavePixivCredential(ctx context.Context, a Pixiv) error {
	return s.SavePixivCredentials(ctx, []Pixiv{a})
}

// SavePixivCredentials 原子保存整批账号；任何输入或落盘错误都不会留下部分修改。
func (s *Store) SavePixivCredentials(ctx context.Context, accounts []Pixiv) error {
	return s.update(ctx, func(doc *document) error {
		seen := make(map[int64]bool, len(accounts))
		now := time.Now().UTC()
		for _, a := range accounts {
			if a.UserID <= 0 || a.Credential.Empty() || seen[a.UserID] {
				return errors.New("account: invalid or duplicate Pixiv credential")
			}
			seen[a.UserID] = true
			index := slices.IndexFunc(doc.Pixiv, func(saved Pixiv) bool { return saved.UserID == a.UserID })
			if index < 0 {
				a.CredentialRevision = 1
				a.Pool.Enabled = true
				a.CreatedAt = now
				a.UpdatedAt = now
				doc.Pixiv = append(doc.Pixiv, a)
				continue
			}
			saved := &doc.Pixiv[index]
			saved.Username = a.Username
			saved.Credential = a.Credential
			saved.CredentialRevision++
			saved.UpdatedAt = now
		}
		return nil
	})
}

// UpdatePixivMetadata 保存用户名与会员检查结果，保持凭据和 revision 不变。
func (s *Store) UpdatePixivMetadata(ctx context.Context, userID int64, username string, premium *bool, checkedAt *time.Time) error {
	return s.update(ctx, func(doc *document) error {
		for i := range doc.Pixiv {
			a := &doc.Pixiv[i]
			if a.UserID == userID {
				a.Username = username
				a.PremiumStatus = premium
				a.PremiumCheckedAt = checkedAt
				a.UpdatedAt = time.Now().UTC()
				return nil
			}
		}
		return ErrPixivNotFound
	})
}

// RotatePixivCredentials 比较读取快照的 revision 与创建时间，防止删除重建后接受旧轮换。
func (s *Store) RotatePixivCredentials(ctx context.Context, expected Pixiv, refreshToken []byte) error {
	if expected.UserID <= 0 || expected.CredentialRevision <= 0 || len(refreshToken) == 0 {
		return errors.New("account: invalid rotation input")
	}
	return s.update(ctx, func(doc *document) error {
		for i := range doc.Pixiv {
			a := &doc.Pixiv[i]
			if a.UserID != expected.UserID {
				continue
			}
			if a.CredentialRevision != expected.CredentialRevision || !a.CreatedAt.Equal(expected.CreatedAt) {
				return ErrCredentialConflict
			}
			a.Credential = credential.New(refreshToken)
			a.CredentialRevision++
			a.UpdatedAt = time.Now().UTC()
			return nil
		}
		return ErrPixivNotFound
	})
}

// RemovePixiv 删除账号及其历史池选择标记，保持剩余账号顺序。
func (s *Store) RemovePixiv(ctx context.Context, userID int64) error {
	return s.update(ctx, func(doc *document) error {
		for i, a := range doc.Pixiv {
			if a.UserID == userID {
				doc.Pixiv = slices.Delete(doc.Pixiv, i, i+1)
				if doc.LastUserID == userID {
					doc.LastUserID = 0
				}
				return nil
			}
		}
		return ErrPixivNotFound
	})
}

// SaveFanboxCredential 保存已验证的 session；更新时保持显示顺序并递增 revision。
func (s *Store) SaveFanboxCredential(ctx context.Context, a Fanbox) error {
	if a.UserID <= 0 || a.Credential.Empty() || a.ValidatedAt.IsZero() {
		return errors.New("account: invalid FANBOX credential")
	}
	return s.update(ctx, func(doc *document) error {
		now := time.Now().UTC()
		for i, saved := range doc.Fanbox {
			if saved.UserID == a.UserID {
				a.CreatedAt = saved.CreatedAt
				a.UpdatedAt = now
				a.CredentialRevision = saved.CredentialRevision + 1
				doc.Fanbox[i] = a
				return nil
			}
		}
		a.CreatedAt = now
		a.UpdatedAt = now
		a.CredentialRevision = 1
		doc.Fanbox = append(doc.Fanbox, a)
		return nil
	})
}

// RemoveFanbox 删除账号，保持其他账号的显示顺序。
func (s *Store) RemoveFanbox(ctx context.Context, userID int64) error {
	return s.update(ctx, func(doc *document) error {
		for i, a := range doc.Fanbox {
			if a.UserID == userID {
				doc.Fanbox = slices.Delete(doc.Fanbox, i, i+1)
				return nil
			}
		}
		return ErrFanboxNotFound
	})
}
