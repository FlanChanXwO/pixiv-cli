package jpeg_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 分段点遍历整份输入，使填充字节、restart/EOI 及末个普通字节落在缓冲边界两侧。
func TestDecodeBufferedMarkerBoundaries(t *testing.T) {
	for _, kind := range []string{"444", "progressive", "gray", "restart"} {
		t.Run(kind, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("..", "testdata", "jpeg-"+kind+".jpg"))
			if err != nil {
				t.Fatal(err)
			}
			if kind != "444" && !bytes.Contains(body, []byte{0xff, 0}) {
				t.Fatal("fixture lacks stuffed entropy bytes")
			}
			if kind == "restart" && !bytes.Contains(body, []byte{0xff, 0xd0}) {
				t.Fatal("fixture lacks restart marker")
			}
			// 单字节 reader 不能进入已缓冲普通字节的补位快路径，作为同像素参考。
			want, err := decodeForTest(oneByteReader{bytes.NewReader(body)})
			if err != nil {
				t.Fatal(err)
			}
			for split := 1; split < len(body); split++ {
				got, err := decodeForTest(io.MultiReader(bytes.NewReader(body[:split]), bytes.NewReader(body[split:])))
				if err != nil {
					t.Fatalf("split %d: %v", split, err)
				}
				if got.Bounds() != want.Bounds() {
					t.Fatalf("split %d: bounds %v, want %v", split, got.Bounds(), want.Bounds())
				}
				for y := 0; y < 9; y++ {
					for x := 0; x < 17; x++ {
						if got.At(x, y) != want.At(x, y) {
							t.Fatalf("split %d: pixel (%d,%d) differs", split, x, y)
						}
					}
				}
			}
		})
	}
}

// 相同 DC 图像分别用立即 EOB 和 ZRL 后 EOB 编码，保护快/慢路径的舍入与截断。
func TestDecodeDCBlockClipping(t *testing.T) {
	table := make([]byte, 17)
	table[0], table[1], table[2] = 0x10, 1, 1
	table = append(table, 0x00, 0xf0)
	for _, quant := range []byte{1, 16, 255} {
		for size := range 17 {
			for _, negative := range []bool{false, true} {
				coefficient := (1 << size) - 1
				bit := "1"
				if negative {
					coefficient = -coefficient
					bit = "0"
				}
				want := uint32(min(255, max(0, (coefficient*int(quant)+4+1024)>>3))) * 257
				for _, zrl := range []string{"", "10"} {
					data := shortACFixture(quant, [][]byte{table}, "0"+strings.Repeat(bit, size)+zrl+"0")
					marker := bytes.Index(data, []byte{0xff, 0xc4})
					if marker < 0 || data[marker+2] != 0 || data[marker+3] != 20 {
						t.Fatal("DC fixture table changed")
					}
					data[marker+4+17] = byte(size)
					for _, short := range []bool{false, true} {
						var reader io.Reader = bytes.NewReader(data)
						if short {
							reader = oneByteReader{reader}
						}
						got, err := decodeForTest(reader)
						if err != nil {
							t.Fatalf("quant=%d size=%d negative=%t zrl=%q short=%t: %v", quant, size, negative, zrl, short, err)
						}
						for y := range 8 {
							for x := range 8 {
								r, g, b, a := got.At(x, y).RGBA()
								if r != want || g != want || b != want || a != 65535 {
									t.Fatalf("quant=%d size=%d negative=%t zrl=%q short=%t pixel=(%d,%d): %d/%d/%d/%d, want gray %d", quant, size, negative, zrl, short, x, y, r, g, b, a, want)
								}
							}
						}
					}
				}
			}
		}
	}
}
