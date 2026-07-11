// Copyright 2018-2026 Developers of the Rand project.
//
// Licensed under the Apache License, Version 2.0 <LICENSE-APACHE or
// https://www.apache.org/licenses/LICENSE-2.0> or the MIT license
// <LICENSE-MIT or https://opensource.org/licenses/MIT>, at your
// option. This file may not be copied, modified, or distributed
// except according to those terms.

/// Initialize a RNG from a `u64` seed using `SplitMix64`.
macro_rules! from_splitmix {
    ($seed:expr) => {{
        let mut rng = crate::SplitMix64::seed_from_u64($seed);
        Self::from_rng(&mut rng)
    }};
}

/// Apply the ** scrambler used by some RNGs from the xoshiro family.
macro_rules! starstar_u64 {
    ($x:expr) => {
        $x.wrapping_mul(5).rotate_left(7).wrapping_mul(9)
    };
}

/// Apply the ** scrambler used by some RNGs from the xoshiro family.
macro_rules! starstar_u32 {
    ($x:expr) => {
        $x.wrapping_mul(0x9E3779BB).rotate_left(5).wrapping_mul(5)
    };
}

/// Apply the ++ scrambler used by some RNGs from the xoshiro family.
macro_rules! plusplus_u64 {
    ($x:expr, $y:expr, $rot:expr) => {
        $x.wrapping_add($y).rotate_left($rot).wrapping_add($x)
    };
}

/// Apply the ++ scrambler used by some RNGs from the xoshiro family.
macro_rules! plusplus_u32 {
    ($x:expr, $y:expr) => {
        $x.wrapping_add($y).rotate_left(7).wrapping_add($x)
    };
}

/// Implement a jump function for an RNG from the xoshiro family.
macro_rules! impl_jump {
    (u32, $self:expr, [$j0:expr, $j1:expr]) => {
        const JUMP: [u32; 2] = [$j0, $j1];
        let mut s0 = 0;
        let mut s1 = 0;
        for j in &JUMP {
            for b in 0..32 {
                if (j & 1 << b) != 0 {
                    s0 ^= $self.s0;
                    s1 ^= $self.s1;
                }
                $self.next_u32();
            }
        }
        $self.s0 = s0;
        $self.s1 = s1;
    };
    (u64, $self:expr, [$j0:expr, $j1:expr]) => {
        const JUMP: [u64; 2] = [$j0, $j1];
        let mut s0 = 0;
        let mut s1 = 0;
        for j in &JUMP {
            for b in 0..64 {
                if (j & 1 << b) != 0 {
                    s0 ^= $self.s0;
                    s1 ^= $self.s1;
                }
                $self.next_u64();
            }
        }
        $self.s0 = s0;
        $self.s1 = s1;
    };
    (u32, $self:expr, [$j0:expr, $j1:expr, $j2:expr, $j3:expr]) => {
        const JUMP: [u32; 4] = [$j0, $j1, $j2, $j3];
        let mut s0 = 0;
        let mut s1 = 0;
        let mut s2 = 0;
        let mut s3 = 0;
        for j in &JUMP {
            for b in 0..32 {
                if (j & 1 << b) != 0 {
                    s0 ^= $self.s[0];
                    s1 ^= $self.s[1];
                    s2 ^= $self.s[2];
                    s3 ^= $self.s[3];
                }
                $self.next_u32();
            }
        }
        $self.s[0] = s0;
        $self.s[1] = s1;
        $self.s[2] = s2;
        $self.s[3] = s3;
    };
    (u64, $self:expr, [$j0:expr, $j1:expr, $j2:expr, $j3:expr]) => {
        const JUMP: [u64; 4] = [$j0, $j1, $j2, $j3];
        let mut s0 = 0;
        let mut s1 = 0;
        let mut s2 = 0;
        let mut s3 = 0;
        for j in &JUMP {
            for b in 0..64 {
                if (j & 1 << b) != 0 {
                    s0 ^= $self.s[0];
                    s1 ^= $self.s[1];
                    s2 ^= $self.s[2];
                    s3 ^= $self.s[3];
                }
                $self.next_u64();
            }
        }
        $self.s[0] = s0;
        $self.s[1] = s1;
        $self.s[2] = s2;
        $self.s[3] = s3;
    };
    (u64, $self:expr, [$j0:expr, $j1:expr, $j2:expr, $j3:expr,
                       $j4:expr, $j5:expr, $j6:expr, $j7:expr]) => {
        const JUMP: [u64; 8] = [$j0, $j1, $j2, $j3, $j4, $j5, $j6, $j7];
        let mut s = [0; 8];
        for j in &JUMP {
            for b in 0..64 {
                if (j & 1 << b) != 0 {
                    s[0] ^= $self.s[0];
                    s[1] ^= $self.s[1];
                    s[2] ^= $self.s[2];
                    s[3] ^= $self.s[3];
                    s[4] ^= $self.s[4];
                    s[5] ^= $self.s[5];
                    s[6] ^= $self.s[6];
                    s[7] ^= $self.s[7];
                }
                $self.next_u64();
            }
        }
        $self.s = s;
    };
}

/// Implement the xoroshiro iteration.
macro_rules! impl_xoroshiro_u32 {
    ($self:expr) => {
        $self.s1 ^= $self.s0;
        $self.s0 = $self.s0.rotate_left(26) ^ $self.s1 ^ ($self.s1 << 9);
        $self.s1 = $self.s1.rotate_left(13);
    };
}

/// Implement the xoroshiro iteration.
macro_rules! impl_xoroshiro_u64 {
    ($self:expr) => {
        $self.s1 ^= $self.s0;
        $self.s0 = $self.s0.rotate_left(24) ^ $self.s1 ^ ($self.s1 << 16);
        $self.s1 = $self.s1.rotate_left(37);
    };
}

/// Implement the xoroshiro iteration for the ++ scrambler.
macro_rules! impl_xoroshiro_u64_plusplus {
    ($self:expr) => {
        $self.s1 ^= $self.s0;
        $self.s0 = $self.s0.rotate_left(49) ^ $self.s1 ^ ($self.s1 << 21);
        $self.s1 = $self.s1.rotate_left(28);
    };
}

/// Implement the xoshiro iteration for `u32` output.
macro_rules! impl_xoshiro_u32 {
    ($self:expr) => {
        let t = $self.s[1] << 9;

        $self.s[2] ^= $self.s[0];
        $self.s[3] ^= $self.s[1];
        $self.s[1] ^= $self.s[2];
        $self.s[0] ^= $self.s[3];

        $self.s[2] ^= t;

        $self.s[3] = $self.s[3].rotate_left(11);
    };
}

/// Implement the xoshiro iteration for `u64` output.
macro_rules! impl_xoshiro_u64 {
    ($self:expr) => {
        let t = $self.s[1] << 17;

        $self.s[2] ^= $self.s[0];
        $self.s[3] ^= $self.s[1];
        $self.s[1] ^= $self.s[2];
        $self.s[0] ^= $self.s[3];

        $self.s[2] ^= t;

        $self.s[3] = $self.s[3].rotate_left(45);
    };
}

/// Implement the large-state xoshiro iteration.
macro_rules! impl_xoshiro_large {
    ($self:expr) => {
        let t = $self.s[1] << 11;

        $self.s[2] ^= $self.s[0];
        $self.s[5] ^= $self.s[1];
        $self.s[1] ^= $self.s[2];
        $self.s[7] ^= $self.s[3];
        $self.s[3] ^= $self.s[4];
        $self.s[4] ^= $self.s[5];
        $self.s[0] ^= $self.s[6];
        $self.s[6] ^= $self.s[7];

        $self.s[6] ^= t;

        $self.s[7] = $self.s[7].rotate_left(21);
    };
}

/// Implement `state` for an RNG with two word-sized fields `s0` and `s1`.
macro_rules! impl_state_pair {
    ($type:ident, $word:ty) => {
        impl $type {
            /// Return the internal state of the generator as little-endian
            /// bytes that can be passed to [`SeedableRng::from_seed`] to
            /// reconstruct an identical generator.
            ///
            /// The all-zero state is unreachable from any non-zero seed, so
            /// the round-trip is exact for any generator obtained via the
            /// usual constructors. (An all-zero input to `from_seed` is
            /// remapped to `seed_from_u64(0)`.)
            ///
            /// [`SeedableRng::from_seed`]: rand_core::SeedableRng::from_seed
            pub fn state(&self) -> [u8; 2 * core::mem::size_of::<$word>()] {
                const N: usize = core::mem::size_of::<$word>();
                const {
                    assert!(core::mem::size_of::<Self>() == 2 * N);
                }
                let mut out = [0u8; 2 * N];
                out[..N].copy_from_slice(&self.s0.to_le_bytes());
                out[N..].copy_from_slice(&self.s1.to_le_bytes());
                out
            }
        }
    };
}

/// Implement `state` for an RNG with an `s: [WORD; 4]` field.
macro_rules! impl_state_array_of_four {
    ($type:ident, $word:ty) => {
        impl $type {
            /// Return the internal state of the generator as little-endian
            /// bytes that can be passed to [`SeedableRng::from_seed`] to
            /// reconstruct an identical generator.
            ///
            /// The all-zero state is unreachable from any non-zero seed, so
            /// the round-trip is exact for any generator obtained via the
            /// usual constructors. (An all-zero input to `from_seed` is
            /// remapped to `seed_from_u64(0)`.)
            ///
            /// [`SeedableRng::from_seed`]: rand_core::SeedableRng::from_seed
            pub fn state(&self) -> [u8; 4 * core::mem::size_of::<$word>()] {
                const N: usize = core::mem::size_of::<$word>();
                const {
                    assert!(core::mem::size_of::<Self>() == 4 * N);
                }
                let mut out = [0u8; 4 * N];
                out[..N].copy_from_slice(&self.s[0].to_le_bytes());
                out[N..2 * N].copy_from_slice(&self.s[1].to_le_bytes());
                out[2 * N..3 * N].copy_from_slice(&self.s[2].to_le_bytes());
                out[3 * N..].copy_from_slice(&self.s[3].to_le_bytes());
                out
            }
        }
    };
}

/// Implement `state` for a 512-bit RNG (`s: [u64; 8]`), returning a [`Seed512`].
macro_rules! impl_state_seed512 {
    ($type:ident) => {
        impl $type {
            /// Return the internal state of the generator as a [`Seed512`]
            /// that can be passed to [`SeedableRng::from_seed`] to reconstruct
            /// an identical generator.
            ///
            /// The all-zero state is unreachable from any non-zero seed, so
            /// the round-trip is exact for any generator obtained via the
            /// usual constructors. (An all-zero input to `from_seed` is
            /// remapped to `seed_from_u64(0)`.)
            ///
            /// [`SeedableRng::from_seed`]: rand_core::SeedableRng::from_seed
            pub fn state(&self) -> crate::Seed512 {
                let mut out = [0u8; 64];
                for (i, word) in self.s.iter().enumerate() {
                    out[i * 8..(i + 1) * 8].copy_from_slice(&word.to_le_bytes());
                }
                crate::Seed512(out)
            }
        }
    };
}

/// Fallback seed used when `from_seed` is called with an all-zero input.
///
/// Equal to the first 64 bytes of `SplitMix64::seed_from_u64(0)`'s output.
/// Smaller generators take a prefix.
static ZERO_SEED_FALLBACK: [u8; 64] = [
    0xaf, 0xcd, 0x1d, 0x7b, 0x39, 0xa8, 0x20, 0xe2, 0xf4, 0x65, 0xb9, 0xa1, 0x6a, 0x9e, 0x78, 0x6e,
    0x4f, 0x45, 0x09, 0x80, 0x18, 0x5d, 0xc4, 0x06, 0xec, 0x81, 0x4c, 0x72, 0xa8, 0xb8, 0x8b, 0xf8,
    0x9b, 0x74, 0xa8, 0x51, 0x6a, 0x89, 0x39, 0x1b, 0xea, 0xa2, 0x7e, 0x74, 0x0c, 0x9f, 0xcb, 0x53,
    0xe1, 0x32, 0x45, 0x1f, 0xbe, 0x9a, 0x82, 0x2c, 0x3c, 0xab, 0x16, 0xc9, 0x3a, 0x13, 0x84, 0xc5,
];

/// Yield seed bytes for state initialization, swapping in the fallback when
/// the input is all-zero.
#[inline]
pub(crate) fn zero_seed_fallback<const N: usize>(seed: &[u8; N]) -> &[u8; N] {
    const { assert!(N <= ZERO_SEED_FALLBACK.len()) };
    if seed.iter().any(|&b| b != 0) {
        seed
    } else {
        ZERO_SEED_FALLBACK.first_chunk::<N>().unwrap()
    }
}

/// 512-bit seed for a generator.
///
/// This wrapper is necessary, because some traits required for a seed are not
/// implemented on large arrays.
#[derive(Clone)]
pub struct Seed512(pub [u8; 64]);

impl Seed512 {
    /// Return an iterator over the seed.
    pub fn iter(&self) -> core::slice::Iter<'_, u8> {
        self.0.iter()
    }
}

impl core::fmt::Debug for Seed512 {
    fn fmt(&self, f: &mut core::fmt::Formatter) -> core::fmt::Result {
        self.0[..].fmt(f)
    }
}

impl Default for Seed512 {
    fn default() -> Seed512 {
        Seed512([0; 64])
    }
}

impl AsRef<[u8]> for Seed512 {
    fn as_ref(&self) -> &[u8] {
        &self.0
    }
}

impl AsMut<[u8]> for Seed512 {
    fn as_mut(&mut self) -> &mut [u8] {
        &mut self.0
    }
}

#[cfg(test)]
mod tests {
    use super::ZERO_SEED_FALLBACK;
    use crate::SplitMix64;
    use rand_core::{Rng, SeedableRng};

    #[test]
    fn zero_seed_fallback_matches_splitmix() {
        let mut sm = SplitMix64::seed_from_u64(0);
        let mut expected = [0u8; 64];
        sm.fill_bytes(&mut expected);
        assert_eq!(ZERO_SEED_FALLBACK, expected);
    }
}
