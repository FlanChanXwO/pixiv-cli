package vectormodel

import (
	"context"
	"errors"
	"fmt"
)

// ProbeGPU 在选定 GPU 上试算固定图片，与同一模型的 CPU 结果比较后才启用设备。
func (m *Model) ProbeGPU(ctx context.Context) (score float64, err error) {
	if m.local == nil || m.options.Device != "gpu" {
		return 0, errors.New("GPU probe requires a local GPU model")
	}
	data, err := ProbeImage()
	if err != nil {
		return 0, err
	}
	gpu, err := m.Image(ctx, data)
	if err != nil {
		return 0, err
	}
	options := m.options
	options.Device = "cpu"
	cpu, err := Open(options, nil)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, cpu.Close()) }()
	reference, err := cpu.Image(ctx, data)
	if err != nil {
		return 0, err
	}
	for i, v := range reference {
		score += float64(v) * float64(gpu[i])
	}
	if score < .999 {
		return score, fmt.Errorf("GPU probe cosine %.6f is below 0.999", score)
	}
	return score, nil
}
