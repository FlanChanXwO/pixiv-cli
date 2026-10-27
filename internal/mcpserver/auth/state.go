package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"slices"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/atomicfile"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/filelock"
)

var (
	// ErrNotInitialized 表示状态文件尚不存在，调用方需要先执行 pixiv mcp auth init。
	ErrNotInitialized = errors.New("MCP owner is not initialized; run pixiv mcp auth init")
	// ErrAlreadyInitialized 表示状态文件已存在而本次初始化没有要求 reset。
	ErrAlreadyInitialized = errors.New("MCP owner is already initialized; use pixiv mcp auth init --reset")
	// ErrInvalidState 表示状态文件内容不满足 Read 校验的结构。
	ErrInvalidState = errors.New("MCP state is invalid")
)

// State 持久化 auth 的全部状态。OwnerVerifier 同时充当内存中授权会话与 code 的
// generation，reset 后旧会话自然失效。
type State struct {
	Version             int               `json:"version"`
	OwnerVerifier       string            `json:"owner_verifier"`
	Clients             map[string]Client `json:"clients"`
	Grants              map[string]Grant  `json:"grants"`
	SelectedPixivUserID int64             `json:"selected_pixiv_user_id"`
}

// Client 保存一个已注册客户端的公开元数据；map key 即 client ID，也没有 client secret。
type Client struct {
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
	// OwnerApproved 只由 owner 同意流程设置，避免匿名注册把已签发授权码的客户端挤掉。
	OwnerApproved bool `json:"owner_approved,omitempty"`
}

// Grant 保存 token 的 verifier 而不保存原值；UsedRefreshHashes 记录已用过的 refresh
// token 摘要，重放时据此撤销整条 grant。
type Grant struct {
	ClientID          string               `json:"client_id"`
	Resource          string               `json:"resource"`
	Scope             string               `json:"scope"`
	AccessTokens      map[string]time.Time `json:"access_tokens"`
	RefreshHash       string               `json:"refresh_hash"`
	UsedRefreshHashes []string             `json:"used_refresh_hashes"`
	Revoked           bool                 `json:"revoked"`
}

// Usable 报告 grant 未撤销、资源匹配且 scope 为 mcp；令牌是否过期由调用方另查。
func (g Grant) Usable(resource string) bool {
	return !g.Revoked && g.Resource == resource && g.Scope == "mcp"
}

// Store 只保存状态文件路径，每次操作都从文件读最新快照，不缓存 verifier 或 grant。
type Store struct{ Path string }

// Read 返回当前快照。状态文件不存在时返回 ErrNotInitialized，不会当作空 owner 放行。
func (s Store) Read(ctx context.Context) (State, error) {
	if s.Path == "" {
		return State{}, errors.New("MCP state path is required")
	}
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	info, err := os.Lstat(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, ErrNotInitialized
	}
	if err != nil {
		return State{}, err
	}
	// 拒绝链接与设备文件，避免读写命中不同目标；权限错误直接返回。
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return State{}, errors.New("MCP state must be a private regular file (0600)")
	}
	body, err := os.ReadFile(s.Path)
	if err != nil {
		return State{}, err
	}
	// selected=0 表示尚未选择；字段缺失或为 null 视为损坏状态。
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return State{}, ErrInvalidState
	}
	for _, key := range []string{"version", "owner_verifier", "clients", "grants", "selected_pixiv_user_id"} {
		if len(fields[key]) == 0 || bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
			return State{}, ErrInvalidState
		}
	}
	// revoked 必须显式出现，缺字段视为损坏状态；used_refresh_hashes 可为 null。
	var grants map[string]struct {
		Revoked           *bool           `json:"revoked"`
		UsedRefreshHashes json.RawMessage `json:"used_refresh_hashes"`
	}
	if err := json.Unmarshal(fields["grants"], &grants); err != nil {
		return State{}, ErrInvalidState
	}
	for _, grant := range grants {
		if grant.Revoked == nil || len(grant.UsedRefreshHashes) == 0 {
			return State{}, ErrInvalidState
		}
	}
	var state State
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	// JSON 错误可能带出原始字段值，所以只返回安全的错误类别。
	if err := decoder.Decode(&state); err != nil {
		return State{}, ErrInvalidState
	}
	if state.Version != 1 {
		return State{}, errors.New("unsupported MCP state version")
	}
	if !validHash(state.OwnerVerifier) || state.SelectedPixivUserID < 0 {
		return State{}, ErrInvalidState
	}
	for id, client := range state.Clients {
		if id == "" || len(client.RedirectURIs) == 0 {
			return State{}, ErrInvalidState
		}
		for _, redirect := range client.RedirectURIs {
			if redirect == "" {
				return State{}, ErrInvalidState
			}
		}
	}
	for id, grant := range state.Grants {
		_, exists := state.Clients[grant.ClientID]
		if id == "" || !exists || grant.Resource == "" || grant.Scope != "mcp" || !validHash(grant.RefreshHash) || grant.AccessTokens == nil {
			return State{}, ErrInvalidState
		}
		for hash, expiry := range grant.AccessTokens {
			if !validHash(hash) || expiry.IsZero() {
				return State{}, ErrInvalidState
			}
		}
		for _, hash := range grant.UsedRefreshHashes {
			if !validHash(hash) {
				return State{}, ErrInvalidState
			}
		}
	}
	return state, nil
}

// Init 在状态落盘后才返回一次性 owner secret。reset 保留客户端注册与账号选择，
// 撤销全部 grant，并记录已有 grant 的客户端曾获批准。
func (s Store) Init(ctx context.Context, reset bool) (string, error) {
	if s.Path == "" {
		return "", errors.New("MCP state path is required")
	}
	var secret string
	err := filelock.WithPrivateLock(ctx, s.Path, func() error {
		state, err := s.Read(ctx)
		if errors.Is(err, ErrNotInitialized) {
			state = State{Version: 1, Clients: map[string]Client{}, Grants: map[string]Grant{}}
		} else if err != nil {
			return err
		} else if !reset {
			return ErrAlreadyInitialized
		}
		// 这是 256 位机器随机值，持久化不可逆摘要就足够。
		candidate, err := randomAuthorizationValue()
		if err != nil {
			return err
		}
		state.OwnerVerifier = tokenHash(candidate)
		// 清空 grant 前先标记这些客户端获过 owner 批准，避免旧授权记录随 grant 一起丢失。
		for _, grant := range state.Grants {
			client := state.Clients[grant.ClientID]
			client.OwnerApproved = true
			state.Clients[grant.ClientID] = client
		}
		state.Grants = map[string]Grant{}
		if err := s.save(ctx, state); err != nil {
			return err
		}
		secret = candidate
		return nil
	})
	if err != nil {
		return "", err
	}
	return secret, nil
}

func validHash(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func (s Store) registerClient(ctx context.Context, client Client) (string, error) {
	var id string
	err := filelock.WithPrivateLock(ctx, s.Path, func() error {
		state, err := s.Read(ctx)
		if err != nil {
			return err
		}
		if len(state.Clients) >= maxOAuthEntries {
			granted := make(map[string]bool, len(state.Grants))
			for _, grant := range state.Grants {
				granted[grant.ClientID] = true
			}
			// 旧状态可能没有 OwnerApproved；只要还有 grant（包括已撤销的）就保留该客户端。
			for id, candidate := range state.Clients {
				if !candidate.OwnerApproved && !granted[id] {
					delete(state.Clients, id)
					if len(state.Clients) < maxOAuthEntries {
						break
					}
				}
			}
			if len(state.Clients) >= maxOAuthEntries {
				return errOAuthCapacity
			}
		}
		candidate, err := randomAuthorizationValue()
		if err != nil {
			return err
		}
		if _, exists := state.Clients[candidate]; exists {
			return errors.New("MCP client ID collision")
		}
		state.Clients[candidate] = client
		if err := s.save(ctx, state); err != nil {
			return err
		}
		id = candidate
		return nil
	})
	return id, err
}

// approveClient 与注册、reset 共用同一把文件锁；批准写盘成功后才允许签发授权码。
func (s Store) approveClient(ctx context.Context, clientID, generation, redirectURI string) error {
	return filelock.WithPrivateLock(ctx, s.Path, func() error {
		state, err := s.Read(ctx)
		if err != nil {
			return err
		}
		client, exists := state.Clients[clientID]
		if state.OwnerVerifier != generation || !exists || !slices.Contains(client.RedirectURIs, redirectURI) {
			return errInvalidGrant
		}
		if client.OwnerApproved {
			return nil
		}
		client.OwnerApproved = true
		state.Clients[clientID] = client
		return s.save(ctx, state)
	})
}

// save 只供已持有侧车锁的事务调用；落盘成功前不对外发布 secret、client 或 token。
func (s Store) save(ctx context.Context, state State) error {
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	_, err = atomicfile.AtomicWrite(ctx, s.Path, bytes.NewReader(append(body, '\n')))
	return err
}

// SelectPixivUser 只写入账号 ID；调用方须先确认本地账号存在，这里不复制凭据，也不改
// CLI 的默认账号。
func (s Store) SelectPixivUser(ctx context.Context, userID int64) error {
	_, _, err := s.selectPixivUser(ctx, userID, nil)
	return err
}

// InitializePixivUser 只在尚无账号选择时采用 initial 账号，并返回最终选中的账号 ID。
func (s Store) InitializePixivUser(ctx context.Context, userID int64) (int64, error) {
	selected, _, err := s.selectPixivUser(ctx, userID, func(selected int64) (bool, error) { return selected == 0, nil })
	return selected, err
}

// SelectPixivUserIf 在同一把状态文件锁内检查 eligible 并提交选择，避免用过期信息覆盖
// 显式选择。eligible 只能读，拿到账号身份而不会拿到凭据。
func (s Store) SelectPixivUserIf(ctx context.Context, userID int64, eligible func(int64) (bool, error)) (bool, error) {
	if eligible == nil {
		return false, errors.New("MCP selection condition is required")
	}
	_, changed, err := s.selectPixivUser(ctx, userID, eligible)
	return changed, err
}

func (s Store) selectPixivUser(ctx context.Context, userID int64, eligible func(int64) (bool, error)) (int64, bool, error) {
	if s.Path == "" {
		return 0, false, errors.New("MCP state path is required")
	}
	if userID <= 0 {
		return 0, false, errors.New("MCP Pixiv user ID must be positive")
	}
	var selected int64
	var changed bool
	err := filelock.WithPrivateLock(ctx, s.Path, func() error {
		state, err := s.Read(ctx)
		if err != nil {
			return err
		}
		if eligible != nil {
			allowed, err := eligible(state.SelectedPixivUserID)
			if err != nil {
				return err
			}
			if !allowed || state.SelectedPixivUserID == userID {
				selected = state.SelectedPixivUserID
				return nil
			}
		}
		state.SelectedPixivUserID = userID
		if err := s.save(ctx, state); err != nil {
			return err
		}
		selected = userID
		changed = true
		return nil
	})
	return selected, changed, err
}
