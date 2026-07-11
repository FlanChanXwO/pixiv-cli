//go:build !ugoira_rust || !cgo

package download

import (
	"context"
	"fmt"
)

type unavailableRustUgoiraEncoder struct{}

func defaultUgoiraEncoder() UgoiraEncoder {
	return NewRustUgoiraEncoder()
}

// NewRustUgoiraEncoder 在没有链接预构建 staticlib 的构建中返回可诊断的 encoder。
func NewRustUgoiraEncoder() UgoiraEncoder {
	return unavailableRustUgoiraEncoder{}
}

func (unavailableRustUgoiraEncoder) Encode(_ context.Context, input UgoiraEncodeInput) error {
	if _, err := input.Format.normalize(); err != nil {
		return err
	}
	return fmt.Errorf("rust ugoira encoder unavailable: built without ugoira_rust tag or cgo")
}
