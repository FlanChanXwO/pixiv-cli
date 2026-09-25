// Package vector persists page-level assets and embeddings independently of the account database.
package vector

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/config/paths"
	_ "modernc.org/sqlite"
)

const applicationID = 0x50495856 // PIXV, distinct from the account database's PIXC.

// schemaVersion 是首个也是当前唯一的 schema 版本。vector.db 从未随正式 release
// 发布过，因此不存在需要兼容的旧库；首次打开直接建立最终结构。
const schemaVersion = 1

type Key struct {
	Source string
	ID     string
	Page   int
}

type Asset struct {
	Key         Key
	Fingerprint string
	Metadata    json.RawMessage
	// ResourceRef 是 Pixiv 页面的稳定资源身份（sdk.ResourceRef 文本）；本地 Asset
	// 恒为空。它从不包含会过期的签名 URL，因此可以跨进程重启重新解析。
	ResourceRef      string
	TargetModel      string
	TargetGeneration string
}

type Store struct{ db *sql.DB }

// Open creates a private vector database under appDataDir, never the account database.
func Open(appDataDir string) (*Store, error) {
	if strings.TrimSpace(appDataDir) == "" {
		return nil, errors.New("vector: app data directory is required")
	}
	if err := os.MkdirAll(appDataDir, paths.PrivateDirMode); err != nil {
		return nil, fmt.Errorf("vector: create directory: %w", err)
	}
	if err := os.Chmod(appDataDir, paths.PrivateDirMode); err != nil {
		return nil, fmt.Errorf("vector: protect directory: %w", err)
	}
	path := filepath.Join(appDataDir, "vector.db")
	info, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("vector: inspect database: %w", err)
	}
	if err == nil && !info.Mode().IsRegular() {
		return nil, errors.New("vector: database must be a regular file")
	}
	if errors.Is(err, os.ErrNotExist) {
		file, createErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, paths.PrivateFileMode)
		if createErr != nil {
			return nil, fmt.Errorf("vector: create database: %w", createErr)
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("vector: close new database: %w", err)
		}
	}
	query := url.Values{}
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "trusted_schema(off)")
	query.Add("_pragma", "synchronous(FULL)")
	// 跨进程写竞争用 SQLite 自带等待。写事务用 IMMEDIATE：deferred 事务在锁升级时
	// 会触发 SQLite 死锁规避而直接 SQLITE_BUSY，IMMEDIATE 让 busy_timeout 真正生效。
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_txlock", "immediate")
	name := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && !strings.HasPrefix(name, "/") {
		name = "/" + name
	}
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: name, RawQuery: query.Encode()}).String())
	if err != nil {
		return nil, fmt.Errorf("vector: open database: %w", err)
	}
	// ponytail: one connection serializes writes; increase only if measured throughput requires it.
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.init(path); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) init(path string) error {
	var id int
	if err := s.db.QueryRow(`PRAGMA application_id`).Scan(&id); err != nil {
		return fmt.Errorf("vector: read database identity: %w", err)
	}
	if id != 0 && id != applicationID {
		return errors.New("vector: database belongs to another application")
	}
	if id == 0 {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("vector: inspect database: %w", err)
		}
		if info.Size() != 0 {
			return errors.New("vector: existing database has no vector identity")
		}
	}
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("vector: read schema version: %w", err)
	}
	if version != 0 && version != schemaVersion {
		return fmt.Errorf("vector: unsupported schema version %d", version)
	}
	// Mark ownership before schema writes so an interrupted first open can resume safely.
	if _, err := s.db.Exec(fmt.Sprintf(`PRAGMA application_id = %d`, applicationID)); err != nil {
		return fmt.Errorf("vector: set database identity: %w", err)
	}
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS asset (
		source TEXT NOT NULL, source_id TEXT NOT NULL, page_index INTEGER NOT NULL,
		fingerprint TEXT NOT NULL, metadata BLOB NOT NULL, resource_ref TEXT NOT NULL DEFAULT '',
		target_model TEXT NOT NULL DEFAULT '', target_generation TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (source, source_id, page_index)
	)`); err != nil {
		return fmt.Errorf("vector: create asset table: %w", err)
	}
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS embedding (
		source TEXT NOT NULL, source_id TEXT NOT NULL, page_index INTEGER NOT NULL,
		model TEXT NOT NULL, generation TEXT NOT NULL, vector BLOB NOT NULL,
		PRIMARY KEY (source, source_id, page_index, model, generation),
		FOREIGN KEY (source, source_id, page_index) REFERENCES asset(source, source_id, page_index) ON DELETE CASCADE
	)`); err != nil {
		return fmt.Errorf("vector: create embedding table: %w", err)
	}
	if version == 0 {
		if _, err := s.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
			return fmt.Errorf("vector: set schema version: %w", err)
		}
	}
	if err := os.Chmod(path, paths.PrivateFileMode); err != nil {
		return fmt.Errorf("vector: protect database: %w", err)
	}
	return nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Upsert preserves the target generation when content is unchanged; a new fingerprint retargets the asset.
func (s *Store) Upsert(ctx context.Context, asset Asset) (bool, error) {
	if err := validateKey(asset.Key); err != nil {
		return false, err
	}
	if strings.TrimSpace(asset.TargetModel) == "" || strings.TrimSpace(asset.TargetGeneration) == "" {
		return false, errors.New("vector: asset model and generation are required")
	}
	if len(asset.Metadata) == 0 {
		asset.Metadata = json.RawMessage(`{}`)
	}
	metadata := bytes.TrimSpace(asset.Metadata)
	if !json.Valid(metadata) || metadata[0] != '{' {
		return false, errors.New("vector: asset metadata must be a JSON object")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("vector: begin asset update: %w", err)
	}
	defer tx.Rollback()
	var oldFingerprint string
	var oldResourceRef string
	err = tx.QueryRowContext(ctx, `SELECT fingerprint, resource_ref FROM asset WHERE source=? AND source_id=? AND page_index=?`,
		asset.Key.Source, asset.Key.ID, asset.Key.Page).Scan(&oldFingerprint, &oldResourceRef)
	changed := errors.Is(err, sql.ErrNoRows)
	if err != nil && !changed {
		return false, fmt.Errorf("vector: read asset: %w", err)
	}
	if changed {
		_, err = tx.ExecContext(ctx, `INSERT INTO asset (source,source_id,page_index,fingerprint,metadata,resource_ref,target_model,target_generation)
			VALUES (?,?,?,?,?,?,?,?)`, asset.Key.Source, asset.Key.ID, asset.Key.Page, asset.Fingerprint, []byte(asset.Metadata), asset.ResourceRef, asset.TargetModel, asset.TargetGeneration)
	} else {
		changed = oldFingerprint != asset.Fingerprint
		if changed {
			// 内容变化时同时刷新资源身份：新观察路径可能带来不同的 variant，但同一页的身份升级是无害的。
			_, err = tx.ExecContext(ctx, `UPDATE asset SET fingerprint=?, metadata=?, resource_ref=?, target_model=?, target_generation=?
				WHERE source=? AND source_id=? AND page_index=?`, asset.Fingerprint, []byte(asset.Metadata), asset.ResourceRef, asset.TargetModel, asset.TargetGeneration,
				asset.Key.Source, asset.Key.ID, asset.Key.Page)
		} else {
			// 指纹不变时只刷新可变展示字段与资源身份（detail 路径可能补充 listing 未给出的页面身份）。
			_, err = tx.ExecContext(ctx, `UPDATE asset SET metadata=?, resource_ref=? WHERE source=? AND source_id=? AND page_index=?`,
				[]byte(asset.Metadata), pickResourceRef(oldResourceRef, asset.ResourceRef), asset.Key.Source, asset.Key.ID, asset.Key.Page)
		}
	}
	if err != nil {
		return false, fmt.Errorf("vector: write asset: %w", err)
	}
	if changed {
		if _, err := tx.ExecContext(ctx, `DELETE FROM embedding WHERE source=? AND source_id=? AND page_index=?`,
			asset.Key.Source, asset.Key.ID, asset.Key.Page); err != nil {
			return false, fmt.Errorf("vector: invalidate embedding: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("vector: commit asset: %w", err)
	}
	return changed, nil
}

func (s *Store) Get(ctx context.Context, key Key) (Asset, error) {
	if err := validateKey(key); err != nil {
		return Asset{}, err
	}
	var asset Asset
	asset.Key = key
	err := s.db.QueryRowContext(ctx, `SELECT fingerprint,metadata,resource_ref,target_model,target_generation FROM asset WHERE source=? AND source_id=? AND page_index=?`, key.Source, key.ID, key.Page).Scan(&asset.Fingerprint, &asset.Metadata, &asset.ResourceRef, &asset.TargetModel, &asset.TargetGeneration)
	if err != nil {
		return Asset{}, fmt.Errorf("vector: get asset: %w", err)
	}
	return asset, nil
}

// pickResourceRef 保留已有身份：指纹不变时，新观察可能来自只携带 cover 的 listing，
// 其 page 0 身份（large variant）不应覆盖 detail 已记录的页面身份（original variant）。
func pickResourceRef(oldRef, newRef string) string {
	if oldRef != "" {
		return oldRef
	}
	return newRef
}

func validateKey(key Key) error {
	if key.Source != "local" && key.Source != "pixiv" || strings.TrimSpace(key.ID) == "" || key.Page < 0 {
		return errors.New("vector: invalid asset identity")
	}
	return nil
}

// ErrStaleAsset means the image changed or disappeared while its embedding was computed.
var ErrStaleAsset = errors.New("vector: asset changed before embedding was stored")

// PutEmbedding writes only if the asset still has the fingerprint read by the worker.
func (s *Store) PutEmbedding(ctx context.Context, key Key, fingerprint, model, generation string, values []float32) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if strings.TrimSpace(model) == "" || strings.TrimSpace(generation) == "" || len(values) == 0 {
		return errors.New("vector: model, generation and vector are required")
	}
	data := make([]byte, len(values)*4)
	for i, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return errors.New("vector: embedding contains a non-finite number")
		}
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(value))
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO embedding (source,source_id,page_index,model,generation,vector)
		SELECT source,source_id,page_index,?,?,? FROM asset
		WHERE source=? AND source_id=? AND page_index=? AND fingerprint=?
		ON CONFLICT (source,source_id,page_index,model,generation)
		DO UPDATE SET vector=excluded.vector`, model, generation, data,
		key.Source, key.ID, key.Page, fingerprint)
	if err != nil {
		return fmt.Errorf("vector: put embedding: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("vector: count embedding writes: %w", err)
	}
	if rows == 0 {
		return ErrStaleAsset
	}
	return nil
}

func (s *Store) Embedding(ctx context.Context, key Key, model, generation string) ([]float32, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	if strings.TrimSpace(model) == "" || strings.TrimSpace(generation) == "" {
		return nil, errors.New("vector: model and generation are required")
	}
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT vector FROM embedding
		WHERE source=? AND source_id=? AND page_index=? AND model=? AND generation=?`,
		key.Source, key.ID, key.Page, model, generation).Scan(&data)
	if err != nil {
		return nil, fmt.Errorf("vector: get embedding: %w", err)
	}
	if len(data) == 0 || len(data)%4 != 0 {
		return nil, errors.New("vector: invalid stored embedding")
	}
	values := make([]float32, len(data)/4)
	for i := range values {
		values[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return values, nil
}

// Pending returns assets assigned to the requested generation that still need an embedding.
// Successful work has no separate job history; absence is durable across restarts.
// ponytail: materializes pending assets; stream rows if index size strains memory.
func (s *Store) Pending(ctx context.Context, model, generation string) ([]Asset, error) {
	if strings.TrimSpace(model) == "" || strings.TrimSpace(generation) == "" {
		return nil, errors.New("vector: model and generation are required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT a.source,a.source_id,a.page_index,a.fingerprint,a.metadata,a.resource_ref,a.target_model,a.target_generation
		FROM asset a WHERE a.target_model=? AND a.target_generation=? AND NOT EXISTS (
			SELECT 1 FROM embedding e WHERE e.source=a.source AND e.source_id=a.source_id
			AND e.page_index=a.page_index AND e.model=? AND e.generation=?
		) ORDER BY a.source,a.source_id,a.page_index`, model, generation, model, generation)
	if err != nil {
		return nil, fmt.Errorf("vector: query pending assets: %w", err)
	}
	defer rows.Close()
	var pending []Asset
	for rows.Next() {
		var asset Asset
		if err := rows.Scan(&asset.Key.Source, &asset.Key.ID, &asset.Key.Page, &asset.Fingerprint, &asset.Metadata, &asset.ResourceRef, &asset.TargetModel, &asset.TargetGeneration); err != nil {
			return nil, fmt.Errorf("vector: read pending asset: %w", err)
		}
		pending = append(pending, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vector: list pending assets: %w", err)
	}
	return pending, nil
}

// Status reports durable rows; an embedding invalidated by a changed asset is not counted.
func (s *Store) Status(ctx context.Context) (assets, embeddings int, err error) {
	if err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM asset`).Scan(&assets); err != nil {
		return 0, 0, fmt.Errorf("vector: count assets: %w", err)
	}
	if err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM embedding`).Scan(&embeddings); err != nil {
		return 0, 0, fmt.Errorf("vector: count embeddings: %w", err)
	}
	return assets, embeddings, nil
}

// HasPixivAssets reports whether the index holds any Pixiv asset, so `vector rebuild`
// only enters the authenticated Pixiv port when there is Pixiv work to re-embed.
func (s *Store) HasPixivAssets(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM asset WHERE source='pixiv'`).Scan(&count); err != nil {
		return false, fmt.Errorf("vector: count pixiv assets: %w", err)
	}
	return count > 0, nil
}

// localRebuildAssets retargets only local assets. Existing vectors remain readable until replaced.
func (s *Store) localRebuildAssets(ctx context.Context, model, generation string) ([]Asset, error) {
	if strings.TrimSpace(model) == "" || strings.TrimSpace(generation) == "" {
		return nil, errors.New("vector: model and generation are required")
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE asset SET target_model=?, target_generation=? WHERE source='local'`, model, generation); err != nil {
		return nil, fmt.Errorf("vector: retarget local assets: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT source_id, page_index, fingerprint FROM asset WHERE source='local' ORDER BY source_id, page_index`)
	if err != nil {
		return nil, fmt.Errorf("vector: list local assets: %w", err)
	}
	defer rows.Close()
	var assets []Asset
	for rows.Next() {
		asset := Asset{Key: Key{Source: "local"}}
		if err := rows.Scan(&asset.Key.ID, &asset.Key.Page, &asset.Fingerprint); err != nil {
			return nil, fmt.Errorf("vector: read local asset: %w", err)
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vector: list local assets: %w", err)
	}
	return assets, nil
}

// pixivRebuildAssets retargets Pixiv assets and returns them with their persisted resource
// identities so a rebuild can re-fetch each page. Existing vectors remain readable until
// replaced.
func (s *Store) pixivRebuildAssets(ctx context.Context, model, generation string) ([]Asset, error) {
	if strings.TrimSpace(model) == "" || strings.TrimSpace(generation) == "" {
		return nil, errors.New("vector: model and generation are required")
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE asset SET target_model=?, target_generation=? WHERE source='pixiv'`, model, generation); err != nil {
		return nil, fmt.Errorf("vector: retarget pixiv assets: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT source_id,page_index,fingerprint,resource_ref FROM asset WHERE source='pixiv' ORDER BY source_id,page_index`)
	if err != nil {
		return nil, fmt.Errorf("vector: list pixiv assets: %w", err)
	}
	defer rows.Close()
	var assets []Asset
	for rows.Next() {
		asset := Asset{Key: Key{Source: "pixiv"}}
		if err := rows.Scan(&asset.Key.ID, &asset.Key.Page, &asset.Fingerprint, &asset.ResourceRef); err != nil {
			return nil, fmt.Errorf("vector: read pixiv asset: %w", err)
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vector: list pixiv assets: %w", err)
	}
	return assets, nil
}
