package version_test

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/version"
)

func TestInfoIsDevelopment(t *testing.T) {
	tests := []struct {
		name string
		info version.Info
		want bool
	}{
		{
			name: "development version",
			info: version.Info{Version: "dev"},
			want: true,
		},
		{
			name: "release version",
			info: version.Info{Version: "v0.1.0"},
			want: false,
		},
		{
			name: "prerelease version",
			info: version.Info{Version: "v0.1.0-beta.1"},
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
