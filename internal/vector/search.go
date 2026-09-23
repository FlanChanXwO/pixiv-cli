package vector

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

type Match struct {
	Asset Asset
	Score float64
}

// CollapsePixivPages keeps one best match per Pixiv artwork for display, while every
// local asset stays as its own result. Input must already be score-sorted, as returned
// by Store.Search; the index itself stays page-level.
func CollapsePixivPages(matches []Match) []Match {
	collapsed := make([]Match, 0, len(matches))
	seen := make(map[string]bool)
	for _, match := range matches {
		if match.Asset.Key.Source == "pixiv" {
			if seen[match.Asset.Key.ID] {
				continue
			}
			seen[match.Asset.Key.ID] = true
		}
		collapsed = append(collapsed, match)
	}
	return collapsed
}

// Search compares only embeddings from one model generation using exact cosine.
// ponytail: exact scan materializes results; add an index only if a real latency benchmark warrants it.
func (s *Store) Search(ctx context.Context, model, generation string, query []float32) ([]Match, error) {
	if strings.TrimSpace(model) == "" || strings.TrimSpace(generation) == "" || len(query) == 0 {
		return nil, errors.New("vector: model, generation and query are required")
	}
	var querySquared float64
	for _, value := range query {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return nil, errors.New("vector: invalid query vector")
		}
		querySquared += float64(value) * float64(value)
	}
	if querySquared == 0 {
		return nil, errors.New("vector: zero query vector")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT a.source,a.source_id,a.page_index,a.fingerprint,a.metadata,a.target_model,a.target_generation,e.vector
		FROM embedding e JOIN asset a ON a.source=e.source AND a.source_id=e.source_id AND a.page_index=e.page_index
		WHERE e.model=? AND e.generation=?`, model, generation)
	if err != nil {
		return nil, fmt.Errorf("vector: query embeddings: %w", err)
	}
	defer rows.Close()
	var matches []Match
	for rows.Next() {
		var match Match
		var data []byte
		if err := rows.Scan(&match.Asset.Key.Source, &match.Asset.Key.ID, &match.Asset.Key.Page,
			&match.Asset.Fingerprint, &match.Asset.Metadata, &match.Asset.TargetModel, &match.Asset.TargetGeneration, &data); err != nil {
			return nil, fmt.Errorf("vector: read embedding: %w", err)
		}
		if len(data) != len(query)*4 {
			return nil, errors.New("vector: embedding dimension mismatch")
		}
		var dot, squared float64
		for i, q := range query {
			value := float64(math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:])))
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, errors.New("vector: invalid stored embedding")
			}
			dot += float64(q) * value
			squared += value * value
		}
		if squared == 0 {
			return nil, errors.New("vector: zero stored embedding")
		}
		match.Score = dot / math.Sqrt(querySquared*squared)
		matches = append(matches, match)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vector: scan embeddings: %w", err)
	}
	sort.Slice(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Asset.Key.Source != b.Asset.Key.Source {
			return a.Asset.Key.Source < b.Asset.Key.Source
		}
		if a.Asset.Key.ID != b.Asset.Key.ID {
			return a.Asset.Key.ID < b.Asset.Key.ID
		}
		return a.Asset.Key.Page < b.Asset.Key.Page
	})
	return matches, nil
}
