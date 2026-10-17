package ugoira

import "math/bits"

// 采样序列移植自 rand_xoshiro 0.8.1 / rand 0.10.2。
// Copyright 2018-2026 Developers of the Rand project，按 MIT 使用；
// 许可见 third_party/licenses/ 中对应版本的 LICENSE-MIT。

type kmeansRNG struct{ s0, s1 uint64 }

func newKmeansRNG() kmeansRNG {
	var seed uint64
	splitmix := func() uint64 {
		seed += 0x9e3779b97f4a7c15
		z := seed
		z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
		z = (z ^ (z >> 27)) * 0x94d049bb133111eb
		return z ^ (z >> 31)
	}
	r := kmeansRNG{splitmix(), splitmix()}
	// 旧 parallel 路径先 jump 再使用第一个随机流；沿用单线程验收基线的顺序。
	var s0, s1 uint64
	for _, jump := range [...]uint64{0x2bd7a6a6e99c2ddc, 0x0992ccaf6a6fca05} {
		for b := range 64 {
			if jump&(uint64(1)<<b) != 0 {
				s0 ^= r.s0
				s1 ^= r.s1
			}
			r.next()
		}
	}
	return kmeansRNG{s0, s1}
}

func (r *kmeansRNG) next() uint64 {
	out := bits.RotateLeft64(r.s0+r.s1, 17) + r.s0
	r.s1 ^= r.s0
	r.s0 = bits.RotateLeft64(r.s0, 49) ^ r.s1 ^ (r.s1 << 21)
	r.s1 = bits.RotateLeft64(r.s1, 28)
	return out
}

func (r *kmeansRNG) index(n uint32) uint32 {
	threshold := -n % n
	for {
		product := uint64(uint32(r.next())) * uint64(n)
		if uint32(product) >= threshold {
			return uint32(product >> 32)
		}
	}
}
