//go:build !cgo || (!darwin && !linux && !windows) || ((darwin || linux || windows) && !amd64 && !arm64)

package ugoira

import (
	"context"
	"fmt"
)

type unavailableRustEncoder struct{}

func NewRustEncoder() Encoder { return unavailableRustEncoder{} }

func (unavailableRustEncoder) Encode(_ context.Context, input Input) error {
	if _, err := input.Format.normalize(); err != nil {
		return err
	}
	return fmt.Errorf("rust ugoira encoder unavailable: Go 1.26.3 with CGO_ENABLED=1 and a supported platform C linker are required")
}
