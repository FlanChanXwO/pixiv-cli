package buildinfo_test

import (
	"reflect"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/shared/buildinfo"
)

func TestInfoIsDevelopment(t *testing.T) {
	tests := []struct {
		name string
		info buildinfo.Info
		want bool
	}{
		{
			name: "development version",
			info: buildinfo.Info{Version: "dev"},
			want: true,
		},
		{
			name: "release version",
			info: buildinfo.Info{Version: "v0.1.0"},
			want: false,
		},
		{
			name: "prerelease version",
			info: buildinfo.Info{Version: "v0.1.0-beta.1"},
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

func TestInfoContainsOnlyVersion(t *testing.T) {
	infoType := reflect.TypeOf(buildinfo.Info{})
	if got := infoType.NumField(); got != 1 {
		t.Fatalf("Info field count = %d, want 1", got)
	}
	if got := infoType.Field(0).Name; got != "Version" {
		t.Errorf("Info field name = %q, want Version", got)
	}
}
