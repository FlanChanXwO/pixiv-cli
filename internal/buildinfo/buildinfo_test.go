package buildinfo_test

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/buildinfo"
)

func TestInfoIsDevelopment(t *testing.T) {
	tests := []struct {
		name string
		info buildinfo.Info
		want bool
	}{
		{
			name: "development version with default metadata",
			info: buildinfo.Info{Version: "dev", Commit: "unknown", BuildDate: "unknown"},
			want: true,
		},
		{
			// 同一 Version 配合不同元数据仍为开发构建，证明判断仅由 Version 决定。
			name: "development version with release metadata",
			info: buildinfo.Info{Version: "dev", Commit: "0123456789abcdef", BuildDate: "2026-07-11T00:00:00Z"},
			want: true,
		},
		{
			name: "release version",
			info: buildinfo.Info{Version: "v0.1.0", Commit: "unknown", BuildDate: "unknown"},
			want: false,
		},
		{
			name: "prerelease version",
			info: buildinfo.Info{Version: "v0.1.0-beta.1", Commit: "deadbeef", BuildDate: "2026-07-12T00:00:00Z"},
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.info.IsDevelopment(); got != test.want {
				t.Errorf("Info.IsDevelopment() = %t, want %t", got, test.want)
			}
		})
	}
}
