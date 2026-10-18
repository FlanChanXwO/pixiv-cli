// IDCT adapted from zune-jpeg 0.5.15 src/idct/scalar.rs.
// Copyright (c) 2023 zune-image developers, MIT.
// See third_party/licenses/zune-jpeg-0.5.15/LICENSE-MIT.

package jpeg

const blockSize = 8 * 8

type block [blockSize]int32

func idct(b *block) {
	// 与旧 zune-jpeg 保持纵向/横向两遍的定点精度及舍入，不能替换为标准库 IDCT。
	dcOnly := true
	for _, c := range b[1:] {
		if c != 0 {
			dcOnly = false
			break
		}
	}
	if dcOnly {
		value := min(int32(255), max(int32(0), (b[0]+4+1024)>>3)) - 128
		for i := range b {
			b[i] = value
		}
		return
	}
	for col := range 8 {
		p2, p3 := b[col+16], b[col+48]
		p1 := (p2 + p3) * 2217
		t2, t3 := p1-p3*7567, p1+p2*3135
		p2, p3 = b[col], b[col+32]
		t0, t1 := (p2+p3)<<12, (p2-p3)<<12
		x0, x3, x1, x2 := t0+t3+512, t0-t3+512, t1+t2+512, t1-t2+512
		t0, t1, t2, t3 = b[col+56], b[col+40], b[col+24], b[col+8]
		p3, p4, p1, p2 := t0+t2, t1+t3, t0+t3, t1+t2
		p5 := (p3 + p4) * 4816
		t0, t1, t2, t3 = t0*1223, t1*8410, t2*12586, t3*6149
		p1, p2, p3, p4 = p5-p1*3685, p5-p2*10497, p3*(-8034), p4*(-1597)
		t3, t2, t1, t0 = t3+p1+p4, t2+p2+p3, t1+p2+p4, t0+p1+p3
		b[col], b[col+8], b[col+16], b[col+24] = (x0+t3)>>10, (x1+t2)>>10, (x2+t1)>>10, (x3+t0)>>10
		b[col+32], b[col+40], b[col+48], b[col+56] = (x3-t0)>>10, (x2-t1)>>10, (x1-t2)>>10, (x0-t3)>>10
	}
	for row := 0; row < 64; row += 8 {
		p2, p3 := b[row+2], b[row+6]
		p1 := (p2 + p3) * 2217
		t2, t3 := p1-p3*7567, p1+p2*3135
		p2, p3 = b[row], b[row+4]
		t0, t1 := (p2+p3)<<12, (p2-p3)<<12
		const bias = 512 + 65536 + (128 << 17)
		x0, x3, x1, x2 := t0+t3+bias, t0-t3+bias, t1+t2+bias, t1-t2+bias
		t0, t1, t2, t3 = b[row+7], b[row+5], b[row+3], b[row+1]
		p3, p4, p1, p2 := t0+t2, t1+t3, t0+t3, t1+t2
		p5 := (p3 + p4) * 4816
		t0, t1, t2, t3 = t0*1223, t1*8410, t2*12586, t3*6149
		p1, p2, p3, p4 = p5-p1*3685, p5-p2*10497, p3*(-8034), p4*(-1597)
		t3, t2, t1, t0 = t3+p1+p4, t2+p2+p3, t1+p2+p4, t0+p1+p3
		b[row], b[row+1], b[row+2], b[row+3] = (x0+t3)>>17, (x1+t2)>>17, (x2+t1)>>17, (x3+t0)>>17
		b[row+4], b[row+5], b[row+6], b[row+7] = (x3-t0)>>17, (x2-t1)>>17, (x1-t2)>>17, (x0-t3)>>17
		for i := row; i < row+8; i++ {
			b[i] = min(int32(255), max(int32(0), b[i])) - 128
		}
	}
}
