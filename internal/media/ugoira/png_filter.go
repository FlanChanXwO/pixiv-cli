package ugoira

// Paeth 预测沿用 png 0.18.1 filter/mod.rs（Copyright (c) 2015 nwin）。
// 按 MIT 使用；许可正文见 third_party/licenses/png-0.18.1/LICENSE-MIT。
func filterAPNGRow(row, previous, filtered []byte) {
	// 固定 Paeth 避免逐行评分多个候选；保持无损，体积仍受同一验收门槛约束。
	filtered[0] = 4
	for i, value := range row {
		var left, corner byte
		if i >= 4 {
			left, corner = row[i-4], previous[i-4]
		}
		filtered[i+1] = value - pngPaeth(left, previous[i], corner)
	}
}

func pngPaeth(left, up, upperLeft byte) byte {
	lo, hi := min(left, up), max(left, up)
	// 与三绝对距离的比较等价，端点相等时仍优先 left/up 而非 corner。
	threshold := 3*int(upperLeft) - int(left) - int(up)
	if threshold >= int(hi) {
		return lo
	}
	if threshold <= int(lo) {
		return hi
	}
	return upperLeft
}
