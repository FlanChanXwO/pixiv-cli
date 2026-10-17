package ugoira

// Sub 预测参考 png 0.18.1 filter/mod.rs（Copyright (c) 2015 nwin）。
// 按 MIT 使用；许可正文见 third_party/licenses/png-0.18.1/LICENSE-MIT。
func filterAPNGRow(row, filtered []byte) {
	// 固定 Sub 避免逐字节 Paeth 分支，仍输出无损RGBA8。
	filtered[0] = 1
	copy(filtered[1:], row[:min(4, len(row))])
	for i := 4; i < len(row); i++ {
		filtered[i+1] = row[i] - row[i-4]
	}
}
