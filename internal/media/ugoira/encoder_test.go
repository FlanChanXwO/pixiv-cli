package ugoira

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func encoderInput(t *testing.T, format Format) Input {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	return Input{
		ZipPath:    writeFrameArchive(t, []frameZipEntry{{"a.png", framePNG(t, img)}, {"bad.png", []byte("broken")}}),
		Frames:     []Frame{{File: "a.png", Delay: 80}, {File: "a.png", Delay: 11}},
		OutputPath: filepath.Join(t.TempDir(), "out."+string(format)), Format: format,
	}
}

func checkEncoderOutput(t *testing.T, input Input) {
	t.Helper()
	body, err := os.ReadFile(input.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if input.Format == FormatAPNG {
		if frames := decodeAPNGTest(t, body); len(frames) != 2 {
			t.Fatal("APNG frame count differs")
		}
	} else {
		decoded, err := gif.DecodeAll(bytes.NewReader(body))
		if err != nil || len(decoded.Image) != 2 {
			t.Fatalf("GIF replay: %v", err)
		}
	}
	checkNoAnimationTemps(t, input.OutputPath)
}

func checkNoAnimationTemps(t *testing.T, output string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(output), ".ugoira-*"))
	if err != nil || len(paths) != 0 {
		t.Fatalf("temporary files remain: %v (%v)", paths, err)
	}
}

func TestEncoderPublishesAndReplaces(t *testing.T) {
	for _, format := range []Format{FormatGIF, FormatAPNG, ""} {
		t.Run(string(format), func(t *testing.T) {
			input := encoderInput(t, FormatGIF)
			input.Format = format
			if err := NewEncoder().Encode(t.Context(), input); err != nil {
				t.Fatal(err)
			}
			checkEncoderOutput(t, input)
			if err := os.WriteFile(input.OutputPath, []byte("previous output"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := NewEncoder().Encode(t.Context(), input); err != nil {
				t.Fatal(err)
			}
			checkEncoderOutput(t, input)
		})
	}
}

func TestEncoderFailurePreservesDestination(t *testing.T) {
	for _, format := range []Format{FormatGIF, FormatAPNG} {
		for _, cause := range []string{"missing-zip", "missing-frame", "bad-frame", "bad-delay", "empty-frames", "bad-format", "pre-cancel", "running-cancel", "publish-cancel"} {
			t.Run(string(format)+"/"+cause, func(t *testing.T) {
				input := encoderInput(t, format)
				original := []byte("previous output")
				if err := os.WriteFile(input.OutputPath, original, 0o600); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				var callCtx context.Context = ctx
				var want error
				switch cause {
				case "missing-zip":
					input.ZipPath += ".missing"
					want = fs.ErrNotExist
				case "missing-frame":
					input.Frames[1].File = "missing.png"
					want = fs.ErrNotExist
				case "bad-frame":
					input.Frames[1].File = "bad.png"
				case "bad-delay":
					input.Frames[1].Delay = 0
				case "empty-frames":
					input.Frames = nil
				case "bad-format":
					input.Format = "webp"
				case "pre-cancel":
					cancel()
					want = context.Canceled
				case "running-cancel", "publish-cancel":
					checks := 0
					callCtx = &encoderCheckContext{Context: ctx, check: func() {
						paths, _ := filepath.Glob(filepath.Join(filepath.Dir(input.OutputPath), ".ugoira-*"))
						if len(paths) != 1 {
							return
						}
						checks++
						info, err := os.Stat(paths[0])
						if (cause == "running-cancel" && checks == 15) || (cause == "publish-cancel" && err == nil && info.Size() > 0) {
							cancel()
						}
					}}
					want = context.Canceled
				}
				err := NewEncoder().Encode(callCtx, input)
				if err == nil || (want != nil && !errors.Is(err, want)) {
					t.Fatalf("expected failure %v, got %v", want, err)
				}
				body, readErr := os.ReadFile(input.OutputPath)
				if readErr != nil || !bytes.Equal(body, original) {
					t.Fatalf("old output changed: %q (%v)", body, readErr)
				}
				checkNoAnimationTemps(t, input.OutputPath)
				if len(encodeGate) != 0 {
					t.Fatal("encoder retained gate after failure")
				}
			})
		}
	}
}

func TestEncoderDestinationErrors(t *testing.T) {
	for _, format := range []Format{FormatGIF, FormatAPNG} {
		t.Run(string(format), func(t *testing.T) {
			input := encoderInput(t, format)
			if err := os.Mkdir(input.OutputPath, 0o700); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(input.OutputPath, "keep")
			if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := NewEncoder().Encode(t.Context(), input); err == nil {
				t.Fatal("directory replaced")
			}
			if body, err := os.ReadFile(sentinel); err != nil || string(body) != "keep" {
				t.Fatal("directory contents changed")
			}
			checkNoAnimationTemps(t, input.OutputPath)
			input.OutputPath = filepath.Join(filepath.Dir(input.OutputPath), "no-extension")
			if err := NewEncoder().Encode(t.Context(), input); err == nil {
				t.Fatal("extensionless destination accepted")
			}
			input.OutputPath = filepath.Join(input.OutputPath, "missing", "out.gif")
			if err := NewEncoder().Encode(t.Context(), input); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("missing parent cause: %v", err)
			}
		})
	}
}

func TestEncoderSerializesAndCancelsQueuedWork(t *testing.T) {
	for _, format := range []Format{FormatGIF, FormatAPNG} {
		t.Run(string(format), func(t *testing.T) {
			first := encoderInput(t, format)
			second := encoderInput(t, format)
			ready, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			held := false
			firstCtx := &encoderCheckContext{Context: t.Context(), check: func() {
				if !held && len(encodeGate) == 1 {
					held = true
					close(ready)
					<-release
				}
			}}
			firstResult := make(chan error, 1)
			go func() { firstResult <- NewEncoder().Encode(firstCtx, first) }()
			<-ready
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			queued := make(chan struct{})
			secondCtx := &encoderQueuedContext{Context: ctx, reached: queued}
			secondResult := make(chan error, 1)
			go func() { secondResult <- NewEncoder().Encode(secondCtx, second) }()
			<-queued
			checkNoAnimationTemps(t, second.OutputPath)
			cancel()
			if err := <-secondResult; !errors.Is(err, context.Canceled) {
				t.Fatalf("queued cancellation: %v", err)
			}
			if _, err := os.Stat(second.OutputPath); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("queued encoder published output: %v", err)
			}
			close(release)
			if err := <-firstResult; err != nil {
				t.Fatal(err)
			}
			checkEncoderOutput(t, first)
			if err := NewEncoder().Encode(t.Context(), second); err != nil {
				t.Fatalf("gate not reusable: %v", err)
			}
			checkEncoderOutput(t, second)
		})
	}
}

type encoderCheckContext struct {
	context.Context
	check func()
}

func (c *encoderCheckContext) Err() error {
	c.check()
	return c.Context.Err()
}

type encoderQueuedContext struct {
	context.Context
	reached chan struct{}
}

func (c *encoderQueuedContext) Done() <-chan struct{} {
	close(c.reached)
	return c.Context.Done()
}

func TestEncoderOutputBudget(t *testing.T) {
	for _, format := range []Format{FormatGIF, FormatAPNG} {
		t.Run(string(format), func(t *testing.T) {
			input := encoderInput(t, format)
			if err := NewEncoder().Encode(t.Context(), input); err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(input.OutputPath)
			if err != nil {
				t.Fatal(err)
			}
			for _, limit := range []uint64{0, 16, uint64(len(original) - 1), uint64(len(original)), uint64(len(original) + 1)} {
				t.Run(fmt.Sprint(limit), func(t *testing.T) {
					input.MaxOutputBytes = &limit
					err := NewEncoder().Encode(t.Context(), input)
					if limit < uint64(len(original)) && !errors.Is(err, ErrOutputCapacity) {
						t.Fatalf("capacity failure lost: %v", err)
					}
					if limit >= uint64(len(original)) && err != nil {
						t.Fatal(err)
					}
					body, err := os.ReadFile(input.OutputPath)
					if err != nil || !bytes.Equal(body, original) {
						t.Fatal("existing destination changed")
					}
					checkNoAnimationTemps(t, input.OutputPath)
				})
			}
		})
	}
}

func TestEncoderSharedBudget(t *testing.T) {
	failure := errors.New("shared capacity refused")
	for _, format := range []Format{FormatGIF, FormatAPNG} {
		for _, mode := range []string{"success", "failure", "panic", "cancel"} {
			t.Run(string(format)+"/"+mode, func(t *testing.T) {
				input := encoderInput(t, format)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				var reserved uint64
				calls := 0
				input.ReserveOutput = func(n uint64) error {
					calls++
					paths, err := filepath.Glob(filepath.Join(filepath.Dir(input.OutputPath), ".ugoira-*"))
					if err != nil {
						return err
					}
					for _, p := range paths {
						info, err := os.Stat(p)
						if err != nil {
							return err
						}
						if uint64(info.Size()) > reserved {
							t.Fatal("write preceded reservation")
						}
					}
					switch mode {
					case "failure":
						return failure
					case "panic":
						panic("fixture")
					case "cancel":
						cancel()
						return ctx.Err()
					}
					reserved += n
					return nil
				}
				err := NewEncoder().Encode(ctx, input)
				if calls == 0 {
					t.Fatal("missing reservation")
				}
				switch mode {
				case "success":
					if err != nil {
						t.Fatal(err)
					}
					info, err := os.Stat(input.OutputPath)
					if err != nil || uint64(info.Size()) != reserved {
						t.Fatalf("reservation mismatch: %v", err)
					}
				case "failure":
					if !errors.Is(err, failure) {
						t.Fatalf("callback cause lost: %v", err)
					}
				case "panic":
					if err == nil || !strings.Contains(err.Error(), "callback panicked") {
						t.Fatalf("panic not contained: %v", err)
					}
				case "cancel":
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("cancellation lost: %v", err)
					}
				}
				checkNoAnimationTemps(t, input.OutputPath)
				if mode != "success" {
					if _, err := os.Stat(input.OutputPath); !errors.Is(err, fs.ErrNotExist) {
						t.Fatalf("failed output published: %v", err)
					}
				}
				// 每次调用独立拥有预算，后续普通调用不继承回调或限制。
				input.ReserveOutput = nil
				if err := NewEncoder().Encode(t.Context(), input); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestEncoderObservesWaitAndEncode(t *testing.T) {
	input := encoderInput(t, FormatGIF)
	var stages []string
	input.Observe = func(stage string, d time.Duration) {
		if d < 0 {
			t.Fatal("negative duration")
		}
		stages = append(stages, stage)
	}
	if err := NewEncoder().Encode(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(stages) != "[animation_encode_wait animation_encode]" {
		t.Fatalf("stages: %v", stages)
	}
}
