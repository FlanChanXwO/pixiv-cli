package ugoira

import (
	"context"
	"math"
)

// Minibatch 更新移植自 quantette 0.6.0 的 quantize/kmeans.rs。
// Copyright (c) 2023 Ian Manske，MIT；见 third_party/licenses/quantette-0.6.0/LICENSE-MIT。

func globalPalette(ctx context.Context, statistics *colorStatistics) ([]rgb, error) {
	samples, err := statistics.paletteSamples(ctx)
	if err != nil {
		return nil, err
	}
	if len(samples.colors) == 0 {
		return nil, nil
	}
	size := 256
	if statistics.hasTransparency {
		size = 255
	}
	centroids, err := wuPalette(ctx, samples, size)
	if err != nil {
		return nil, err
	}
	if err := refinePalette(ctx, samples, centroids); err != nil {
		return nil, err
	}
	out := make([]rgb, len(centroids))
	for i, c := range centroids {
		out[i] = oklabToSRGB(c)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// centroids 由本次量化独占；取消后调用者丢弃尚未完成的结果。
func refinePalette(ctx context.Context, samples paletteSamples, centroids []oklab) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	const batchSize = 4096
	counts := make([]uint32, len(centroids))
	var batch [batchSize]oklab
	var assignments [batchSize]byte
	rng := newKmeansRNG()
	length := uint32(len(samples.indices))
	var search colorSearch
	for start := 0; start < min(len(samples.indices), maxColorSamples); start += batchSize {
		n := min(batchSize, min(len(samples.indices), maxColorSamples)-start)
		search.resetTree(centroids)
		// 同一批的归属使用更新前的中心；更新后下一批必须重建搜索树。
		for i := range n {
			if err := ctx.Err(); err != nil {
				return err
			}
			batch[i] = samples.colors[samples.indices[rng.index(length)]]
			assignments[i] = search.nearest(batch[i])
		}
		for i, c := range batch[:n] {
			if err := ctx.Err(); err != nil {
				return err
			}
			index := assignments[i]
			counts[index]++
			rate := float32(1 / math.Sqrt(float64(counts[index])))
			for channel, x := range c {
				center := &centroids[index][channel]
				*center += float32(rate * float32(x-*center))
			}
		}
	}
	return ctx.Err()
}
