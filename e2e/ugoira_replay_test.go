package e2e

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira"
)

func ugoiraPNGChunk(w *bytes.Buffer, kind string, data []byte) {
	_ = binary.Write(w, binary.BigEndian, uint32(len(data)))
	w.WriteString(kind)
	w.Write(data)
	sum := crc32.NewIEEE()
	_, _ = sum.Write([]byte(kind))
	_, _ = sum.Write(data)
	_ = binary.Write(w, binary.BigEndian, sum.Sum32())
}

// 独立重建各帧PNG再按Source/Over及None处置重放；不复用生产writer的滤波/压缩代码。
func replayUgoiraAPNG(body []byte, visit func(int, *image.NRGBA, float64) error) error {
	const signature = "\x89PNG\r\n\x1a\n"
	if !bytes.HasPrefix(body, []byte(signature)) {
		return fmt.Errorf("invalid PNG signature")
	}
	body = body[8:]
	var ihdr, control []byte
	var compressed bytes.Buffer
	var canvas *image.NRGBA
	var frames, sequence uint32
	index := 0
	animated := false
	emit := func() error {
		if control == nil || compressed.Len() == 0 {
			return fmt.Errorf("missing APNG frame data")
		}
		w, h := int(binary.BigEndian.Uint32(control[4:8])), int(binary.BigEndian.Uint32(control[8:12]))
		x, y := int(binary.BigEndian.Uint32(control[12:16])), int(binary.BigEndian.Uint32(control[16:20]))
		r := image.Rect(x, y, x+w, y+h)
		if w <= 0 || h <= 0 || !r.In(canvas.Rect) || (index == 0 && r != canvas.Rect) {
			return fmt.Errorf("invalid APNG frame rectangle")
		}
		if control[24] != 0 || control[25] > 1 {
			return fmt.Errorf("APNG disposal/blend changed")
		}
		header := bytes.Clone(ihdr)
		binary.BigEndian.PutUint32(header[0:4], uint32(w))
		binary.BigEndian.PutUint32(header[4:8], uint32(h))
		var frame bytes.Buffer
		frame.WriteString(signature)
		ugoiraPNGChunk(&frame, "IHDR", header)
		ugoiraPNGChunk(&frame, "IDAT", compressed.Bytes())
		ugoiraPNGChunk(&frame, "IEND", nil)
		img, err := png.Decode(bytes.NewReader(frame.Bytes()))
		if err != nil {
			return err
		}
		op := draw.Src
		if control[25] == 1 {
			op = draw.Over
		}
		draw.Draw(canvas, r, img, image.Point{}, op)
		numerator, denominator := binary.BigEndian.Uint16(control[20:22]), binary.BigEndian.Uint16(control[22:24])
		if denominator == 0 {
			denominator = 100
		}
		if err := visit(index, canvas, 1000*float64(numerator)/float64(denominator)); err != nil {
			return err
		}
		index++
		compressed.Reset()
		return nil
	}
	for len(body) > 0 {
		if len(body) < 12 {
			return fmt.Errorf("truncated PNG chunk")
		}
		n := uint64(binary.BigEndian.Uint32(body[:4]))
		if n > uint64(len(body)-12) {
			return fmt.Errorf("truncated PNG chunk data")
		}
		end := int(n) + 8
		kind := string(body[4:8])
		data := body[8:end]
		if crc32.ChecksumIEEE(body[4:end]) != binary.BigEndian.Uint32(body[end:end+4]) {
			return fmt.Errorf("PNG CRC mismatch")
		}
		body = body[end+4:]
		switch kind {
		case "IHDR":
			if ihdr != nil || animated || len(data) != 13 || data[8] != 8 || data[9] != 6 || data[10] != 0 || data[11] != 0 || data[12] != 0 {
				return fmt.Errorf("expected one RGBA8 noninterlaced IHDR")
			}
			ihdr = bytes.Clone(data)
			w, h := int(binary.BigEndian.Uint32(data[:4])), int(binary.BigEndian.Uint32(data[4:8]))
			if w <= 0 || h <= 0 {
				return fmt.Errorf("empty APNG canvas")
			}
			canvas = image.NewNRGBA(image.Rect(0, 0, w, h))
		case "acTL":
			if ihdr == nil || animated || len(data) != 8 || binary.BigEndian.Uint32(data[4:]) != 0 {
				return fmt.Errorf("invalid APNG animation/loop control")
			}
			frames = binary.BigEndian.Uint32(data[:4])
			if frames == 0 {
				return fmt.Errorf("empty APNG animation")
			}
			animated = true
		case "fcTL":
			if !animated || len(data) != 26 || binary.BigEndian.Uint32(data[:4]) != sequence {
				return fmt.Errorf("invalid APNG frame control sequence")
			}
			sequence++
			if control != nil {
				if err := emit(); err != nil {
					return err
				}
			}
			control = bytes.Clone(data)
		case "IDAT":
			if control == nil || index != 0 {
				return fmt.Errorf("unexpected APNG IDAT")
			}
			compressed.Write(data)
		case "fdAT":
			if control == nil || index == 0 || len(data) < 4 || binary.BigEndian.Uint32(data[:4]) != sequence {
				return fmt.Errorf("invalid APNG frame data sequence")
			}
			sequence++
			compressed.Write(data[4:])
		case "IEND":
			if len(data) != 0 || len(body) != 0 {
				return fmt.Errorf("invalid PNG end")
			}
			if err := emit(); err != nil {
				return err
			}
			if uint32(index) != frames {
				return fmt.Errorf("APNG frame count differs")
			}
			return nil
		default:
			if len(kind) != 4 || kind[0]&32 == 0 {
				return fmt.Errorf("unexpected critical PNG chunk")
			}
		}
	}
	return fmt.Errorf("missing PNG end")
}

func TestUgoiraAPNGReplayRejectsCorruption(t *testing.T) {
	var pngImage bytes.Buffer
	if err := png.Encode(&pngImage, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	// 普通PNG不能冒充动图；损坏CRC/截断也必须失败。
	for _, body := range [][]byte{nil, []byte("not PNG"), pngImage.Bytes(), pngImage.Bytes()[:20], append(bytes.Clone(pngImage.Bytes()), 0)} {
		if err := replayUgoiraAPNG(body, func(int, *image.NRGBA, float64) error { return nil }); err == nil {
			t.Fatal("invalid animation accepted")
		}
	}
}

func TestUgoiraReplayFixture(t *testing.T) {
	for _, name := range []string{"gradient-motion", "alpha-repeat", "one-pixel", "odd-edge"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "sample")
			captureSyntheticUgoira(t, dir, ugoiraSyntheticScenes()[name], []int{1, 9, 10, 655350})
			for _, format := range []ugoira.Format{ugoira.FormatGIF, ugoira.FormatAPNG} {
				t.Run(string(format), func(t *testing.T) {
					output := filepath.Join(t.TempDir(), "out."+string(format))
					if _, err := encodeUgoiraSample(t.Context(), ugoira.NewEncoder(), dir, output, format, 0); err != nil {
						t.Fatal(err)
					}
					t.Setenv("UGOIRA_PARITY_SAMPLE", dir)
					t.Setenv("UGOIRA_PARITY_OUTPUT", output)
					if !t.Run("inspect", TestUgoiraParityInspect) {
						t.Fatal("fixture replay failed")
					}
					if format == ugoira.FormatAPNG {
						body, err := os.ReadFile(output)
						if err != nil {
							t.Fatal(err)
						}
						body[len(body)-1] ^= 1
						if err := replayUgoiraAPNG(body, func(int, *image.NRGBA, float64) error { return nil }); err == nil {
							t.Fatal("corrupt APNG accepted")
						}
					}
				})
			}
		})
	}
}
