package jpeg_test

import (
	"bytes"
	"encoding/binary"
	"image"
	"testing"

	framejpeg "github.com/FlanChanXwO/pixiv-cli/internal/media/ugoira/jpeg"
)

func TestShortACCoefficients(t *testing.T) {
	// 四位码依次对应这些 AC 符号；像素固定为加速前的解码结果。
	symbols := []byte{0x01, 0x11, 0x02, 0x22, 0x03, 0x04, 0x05, 0x31, 0x00, 0xf0, 0xf1}
	table := make([]byte, 17)
	table[0], table[4] = 0x10, byte(len(symbols))
	table = append(table, symbols...)
	for _, tc := range []struct {
		name    string
		entropy string
		want    []byte
	}{
		{
			name:    "mixed",
			entropy: "0" + "00001" + "00010" + "001011" + "001100" + "0100111" + "01010000" + "011011111" + "1001" + "00000" + "1000",
			want: []byte{
				180, 184, 177, 172, 156, 132, 117, 100,
				86, 92, 87, 88, 79, 61, 52, 40,
				100, 108, 108, 116, 116, 109, 110, 103,
				193, 201, 203, 215, 223, 226, 235, 233,
				145, 151, 151, 163, 173, 180, 194, 195,
				16, 19, 12, 18, 23, 28, 43, 45,
				68, 66, 52, 49, 48, 48, 60, 61,
				249, 244, 225, 216, 210, 206, 216, 216,
			},
		},
		{
			name:    "mirror",
			entropy: "0" + "00000" + "00011" + "001000" + "001111" + "0100000" + "01011111" + "011000000" + "1001" + "00001" + "1000",
			want: []byte{
				76, 72, 79, 84, 100, 124, 139, 156,
				170, 164, 169, 168, 177, 195, 204, 216,
				156, 148, 148, 140, 140, 147, 146, 153,
				63, 55, 53, 41, 33, 30, 21, 23,
				111, 105, 105, 93, 83, 76, 62, 61,
				240, 237, 244, 238, 233, 228, 213, 211,
				188, 190, 204, 207, 208, 208, 196, 195,
				7, 12, 31, 40, 46, 50, 40, 40,
			},
		},
		// 游程已超出本块时，旧路径不再读取幅值，也不能写到块外。
		{"run-past-block", "0" + "1001" + "1001" + "1001" + "1010", bytes.Repeat([]byte{128}, 64)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := shortACFixture(16, [][]byte{table}, tc.entropy)
			for _, shortReads := range []bool{false, true} {
				var decoded image.Image
				var err error
				if shortReads {
					decoded, err = framejpeg.Decode(oneByteReader{bytes.NewReader(data)})
				} else {
					decoded, err = framejpeg.Decode(bytes.NewReader(data))
				}
				if err != nil {
					t.Fatal(err)
				}
				got := decoded.(*image.Gray)
				if !bytes.Equal(got.Pix, tc.want) {
					t.Fatalf("short reads=%t: pixels=%v, want %v", shortReads, got.Pix, tc.want)
				}
			}
		})
	}
}

func TestShortACRedefinedTable(t *testing.T) {
	old := make([]byte, 17)
	old[0], old[4] = 0x10, 8
	old = append(old, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x31)
	// 新表的短前缀是 EOB；旧表同一前缀曾包含非零幅值。
	current := make([]byte, 17)
	current[0], current[1], current[2] = 0x10, 1, 1
	current = append(current, 0x00, 0x01)
	data := shortACFixture(1, [][]byte{old, current}, "0"+"101"+"0"+"11111111")
	for _, shortReads := range []bool{false, true} {
		var decoded image.Image
		var err error
		if shortReads {
			decoded, err = framejpeg.Decode(oneByteReader{bytes.NewReader(data)})
		} else {
			decoded, err = framejpeg.Decode(bytes.NewReader(data))
		}
		if err != nil {
			t.Fatal(err)
		}
		if got := decoded.(*image.Gray); !bytes.Equal(got.Pix, bytes.Repeat([]byte{128}, 64)) {
			t.Fatalf("redefined table: %v", got.Pix)
		}
	}
}

func shortACFixture(quant byte, tables [][]byte, entropy string) []byte {
	data := []byte{0xff, 0xd8}
	segment := func(marker byte, payload []byte) {
		data = append(data, 0xff, marker)
		data = binary.BigEndian.AppendUint16(data, uint16(len(payload)+2))
		data = append(data, payload...)
	}
	segment(0xdb, append([]byte{0}, bytes.Repeat([]byte{quant}, 64)...))
	segment(0xc0, []byte{8, 0, 8, 0, 8, 1, 1, 0x11, 0})
	dc := make([]byte, 18)
	dc[1] = 1
	segment(0xc4, dc)
	for _, table := range tables {
		segment(0xc4, table)
	}
	segment(0xda, []byte{1, 1, 0, 0, 63, 0})
	for i := 0; i < len(entropy); i += 8 {
		var value byte
		for j := range 8 {
			value <<= 1
			if i+j >= len(entropy) || entropy[i+j] == '1' {
				value |= 1
			}
		}
		data = append(data, value)
		if value == 0xff {
			data = append(data, 0)
		}
	}
	return append(data, 0xff, 0xd9)
}
