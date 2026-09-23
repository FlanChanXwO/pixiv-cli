package vector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type SyncStats struct {
	Scanned int
	Changed int
}

// SyncLocal explicitly scans a gallery and records changed images; it does not embed them.
func SyncLocal(ctx context.Context, store *Store, root string) (SyncStats, error) {
	if strings.TrimSpace(root) == "" {
		return SyncStats{}, errors.New("vector: gallery path is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return SyncStats{}, fmt.Errorf("vector: resolve gallery: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return SyncStats{}, fmt.Errorf("vector: resolve gallery: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return SyncStats{}, fmt.Errorf("vector: stat gallery: %w", err)
	}
	if !info.IsDir() {
		return SyncStats{}, errors.New("vector: gallery must be a directory")
	}
	var stats SyncStats
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		// 只读取明确目录下的常见位图，symlink 不跟随到目录外。
		if !entry.Type().IsRegular() || !IsImageExtension(filepath.Ext(path)) {
			return nil
		}
		fingerprint, err := fileFingerprint(path)
		if err != nil {
			return err
		}
		changed, err := store.Upsert(ctx, Asset{Key: Key{Source: "local", ID: path}, Fingerprint: fingerprint, TargetModel: ModelID, TargetGeneration: Generation})
		if err != nil {
			return err
		}
		stats.Scanned++
		if changed {
			stats.Changed++
		}
		return nil
	})
	if err != nil {
		return stats, fmt.Errorf("vector: scan gallery: %w", err)
	}
	return stats, nil
}

// IsImageExtension reports the extensions scanned from local galleries.
func IsImageExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif", ".bmp", ".tif", ".tiff":
		return true
	}
	return false
}

func fileFingerprint(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	before, err := file.Stat()
	if err != nil {
		file.Close()
		return "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	after, statErr := file.Stat()
	closeErr := file.Close()
	if err := errors.Join(copyErr, statErr, closeErr); err != nil {
		return "", err
	}
	if !before.Mode().IsRegular() || !after.Mode().IsRegular() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return "", fmt.Errorf("vector: image changed during scan: %s", path)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ProcessLocalPending processes pending images under the explicitly synced gallery only.
// Failures remain pending for a later sync.
// ponytail: one worker bounds model memory; add parallel workers only after measuring throughput.
func ProcessLocalPending(ctx context.Context, store *Store, root, model, generation string, embed func(context.Context, string) ([]float32, error)) (int, error) {
	if strings.TrimSpace(root) == "" {
		return 0, errors.New("vector: gallery path is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return 0, fmt.Errorf("vector: resolve gallery: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return 0, fmt.Errorf("vector: resolve gallery: %w", err)
	}
	pending, err := store.Pending(ctx, model, generation)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, asset := range pending {
		if asset.Key.Source != "local" {
			continue
		}
		rel, err := filepath.Rel(root, asset.Key.ID)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		if embed == nil {
			return processed, errors.New("vector: embedding runtime unavailable")
		}
		fingerprint, err := fileFingerprint(asset.Key.ID)
		if err != nil {
			return processed, fmt.Errorf("vector: verify source image: %w", err)
		}
		if fingerprint != asset.Fingerprint {
			return processed, ErrStaleAsset
		}
		values, err := embed(ctx, asset.Key.ID)
		if err != nil {
			return processed, fmt.Errorf("vector: embed image: %w", err)
		}
		fingerprint, err = fileFingerprint(asset.Key.ID)
		if err != nil {
			return processed, fmt.Errorf("vector: verify source image: %w", err)
		}
		if fingerprint != asset.Fingerprint {
			return processed, ErrStaleAsset
		}
		if err := store.PutEmbedding(ctx, asset.Key, fingerprint, model, generation, values); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}
