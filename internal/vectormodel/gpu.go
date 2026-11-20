package vectormodel

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/internal/i18n"
)

// ProbeGPU 在选定 GPU 上试算固定图片，与同一模型的 CPU 结果比较后才启用设备。
func (m *Model) ProbeGPU(ctx context.Context) (score float64, err error) {
	if m.local == nil || m.options.Device != "gpu" {
		return 0, i18n.Errorf("error.gpu_probe_requires_a_local_gpu_model")
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
		return score, i18n.Errorf("error.gpu_probe_cosine_6f_is_below_0_999", score)
	}
	return score, nil
}
