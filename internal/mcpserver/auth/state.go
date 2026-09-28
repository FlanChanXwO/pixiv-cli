// Package auth 拥有 MCP owner 与 OAuth 的私有状态，不接触产品账号凭据。
package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/internal/storage/file/atomic"
	"github.com/FlanChanXwO/pixiv-cli/internal/storage/file/lock"
)

var (
	ErrNotInitialized     = errors.New("MCP owner is not initialized; run pixiv mcp auth init")
	ErrAlreadyInitialized = errors.New("MCP owner is already initialized; use pixiv mcp auth init --reset")
	ErrInvalidState       = errors.New("MCP state is invalid")
)

// State 是唯一持久合同；owner verifier 同时是内存授权会话/code 的 generation。
type State struct {
	Version             int               `json:"version"`
	OwnerVerifier       string            `json:"owner_verifier"`
	Clients             map[string]Client `json:"clients"`
	Grants              map[string]Grant  `json:"grants"`
	SelectedPixivUserID int64             `json:"selected_pixiv_user_id"`
}

// Client 只记录 public client metadata；map key 是稳定 client ID，不保存 secret。
type Client struct {
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
}

// Grant 持久化 token verifier 而非原值；已用 refresh hashes 留给重放撤销检查。
type Grant struct {
	ClientID          string               `json:"client_id"`
	Resource          string               `json:"resource"`
	Scope             string               `json:"scope"`
	AccessTokens      map[string]time.Time `json:"access_tokens"`
	RefreshHash       string               `json:"refresh_hash"`
	UsedRefreshHashes []string             `json:"used_refresh_hashes"`
	Revoked           bool                 `json:"revoked"`
}

// Store 只持有路径；每次操作从原子文件读取当前状态，不缓存 verifier。
type Store struct{ Path string }

// Read 读取当前快照；缺失状态不是可自动放行的空 owner。
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
	// 拒绝链接/设备文件，避免读写指向不同目标；权限错误不能通过 reset 掩盖。
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return State{}, errors.New("MCP state must be a private regular file (0600)")
	}
	body, err := os.ReadFile(s.Path)
	if err != nil {
		return State{}, err
	}
	// selected=0 明确表示尚未选择；缺字段/null 不能被当作首次初始化。
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return State{}, ErrInvalidState
	}
	for _, key := range []string{"version", "owner_verifier", "clients", "grants", "selected_pixiv_user_id"} {
		if len(fields[key]) == 0 || bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
			return State{}, ErrInvalidState
		}
	}
	var state State
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	// JSON 错误可能带入原始字段值，边界只返回安全的类别。
	if err := decoder.Decode(&state); err != nil {
		return State{}, ErrInvalidState
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return State{}, ErrInvalidState
	}
	if state.Version != 1 {
		return State{}, errors.New("unsupported MCP state version")
	}
	if !validHash(state.OwnerVerifier) || state.Clients == nil || state.Grants == nil || state.SelectedPixivUserID < 0 {
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

// Init 提交成功后才返回一次性 secret。reset 保留注册与账号选择，撤销全部 grants。
func (s Store) Init(ctx context.Context, reset bool) (string, error) {
	if s.Path == "" {
		return "", errors.New("MCP state path is required")
	}
	var secret string
	err := lock.WithPrivateLock(ctx, s.Path, func() error {
		state, err := s.Read(ctx)
		if errors.Is(err, ErrNotInitialized) {
			state = State{Version: 1, Clients: map[string]Client{}, Grants: map[string]Grant{}}
		} else if err != nil {
			return err
		} else if !reset {
			return ErrAlreadyInitialized
		}
		// 256 位机器随机值不是用户密码，持久化只需不可逆 SHA-256 verifier。
		value := make([]byte, 32)
		if _, err := rand.Read(value); err != nil {
			return err
		}
		candidate := base64.RawURLEncoding.EncodeToString(value)
		digest := sha256.Sum256([]byte(candidate))
		state.OwnerVerifier = hex.EncodeToString(digest[:])
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
	err := lock.WithPrivateLock(ctx, s.Path, func() error {
		state, err := s.Read(ctx)
		if err != nil {
			return err
		}
		value := make([]byte, 32)
		if _, err := rand.Read(value); err != nil {
			return err
		}
		candidate := base64.RawURLEncoding.EncodeToString(value)
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

// save 仅供持有侧车锁的事务调用；成功落盘前不发布 secret、client 或 token。
func (s Store) save(ctx context.Context, state State) error {
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	_, err = atomic.AtomicWrite(ctx, s.Path, bytes.NewReader(append(body, '\n')))
	return err
}
