//go:build !cgo || (!darwin && !linux && !windows) || ((darwin || linux || windows) && !amd64 && !arm64)

package ugoira

import (
	"context"
	"fmt"
)

// 这是刻意的编译期门禁：源码构建和 go install 不能绕过 cgo/staticlib 产物，
// 从而生成运行时才降级的 pixiv binary。
const _ = uint8("Go 1.26.3 + CGO_ENABLED=1/cgo + Rust staticlib + target C linker required")

type unavailableRustEncoder struct{}

func NewRustEncoder() Encoder { return unavailableRustEncoder{} }

func (unavailableRustEncoder) Encode(_ context.Context, input Input) error {
	if _, err := input.Format.normalize(); err != nil {
		return err
	}
	return fmt.Errorf("rust ugoira encoder unavailable: Go 1.26.3 with CGO_ENABLED=1 and a supported platform C linker are required")
}
