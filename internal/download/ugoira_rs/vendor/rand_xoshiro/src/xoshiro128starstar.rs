// Copyright 2018 Developers of the Rand project.
//
// Licensed under the Apache License, Version 2.0 <LICENSE-APACHE or
// https://www.apache.org/licenses/LICENSE-2.0> or the MIT license
// <LICENSE-MIT or https://opensource.org/licenses/MIT>, at your
// option. This file may not be copied, modified, or distributed
// except according to those terms.

use core::convert::Infallible;
use rand_core::{Rng, SeedableRng, TryRng, utils};
#[cfg(feature = "serde")]
use serde::{Deserialize, Serialize};

/// A xoshiro128** random number generator.
///
/// The xoshiro128** algorithm is not suitable for cryptographic purposes, but
/// is very fast and has excellent statistical properties.
///
/// The algorithm used here is translated from [the `xoshiro128starstar.c`
/// reference source code](http://xoshiro.di.unimi.it/xoshiro128starstar.c) by
/// David Blackman and Sebastiano Vigna.
#[derive(Debug, Clone, PartialEq, Eq)]
#[cfg_attr(feature = "serde", derive(Serialize, Deserialize))]
pub struct Xoshiro128StarStar {
    s: [u32; 4],
}

impl Xoshiro128StarStar {
    /// Jump forward, equivalently to 2^64 calls to `next_u32()`.
    ///
    /// This can be used to generate 2^64 non-overlapping subsequences for
    /// parallel computations.
    ///
    /// ```
    /// use rand_xoshiro::rand_core::SeedableRng;
    /// use rand_xoshiro::Xoroshiro128StarStar;
    ///
    /// let rng1 = Xoroshiro128StarStar::seed_from_u64(0);
    /// let mut rng2 = rng1.clone();
    /// rng2.jump();
    /// let mut rng3 = rng2.clone();
    /// rng3.jump();
    /// ```
    pub fn jump(&mut self) {
        impl_jump!(u32, self, [0x8764000b, 0xf542d2d3, 0x6fa035c3, 0x77f2db5b]);
    }

    /// Jump forward, equivalently to 2^96 calls to `next_u32()`.
    ///
    /// This can be used to generate 2^32 starting points, from each of which
    /// `jump()` will generate 2^32 non-overlapping subsequences for parallel
    /// distributed computations.
    pub fn long_jump(&mut self) {
        impl_jump!(u32, self, [0xb523952e, 0x0b6f099f, 0xccf5a0ef, 0x1c580662]);
    }
}

impl_state_array_of_four!(Xoshiro128StarStar, u32);

impl SeedableRng for Xoshiro128StarStar {
    type Seed = [u8; 16];

    /// Create a new `Xoshiro128StarStar`.  If `seed` is entirely 0, it will be
    /// mapped to a different seed.
    #[inline]
    fn from_seed(seed: [u8; 16]) -> Xoshiro128StarStar {
        Xoshiro128StarStar {
            s: utils::read_words(crate::common::zero_seed_fallback(&seed)),
        }
    }

    /// Seed a `Xoshiro128StarStar` from a `u64` using `SplitMix64`.
    fn seed_from_u64(seed: u64) -> Xoshiro128StarStar {
        from_splitmix!(seed)
    }
}

impl TryRng for Xoshiro128StarStar {
    type Error = Infallible;

    #[inline]
    fn try_next_u32(&mut self) -> Result<u32, Self::Error> {
        let result_starstar = starstar_u64!(self.s[1]);
        impl_xoshiro_u32!(self);
        Ok(result_starstar)
    }

    #[inline]
    fn try_next_u64(&mut self) -> Result<u64, Self::Error> {
        utils::next_u64_via_u32(self)
    }

    #[inline]
    fn try_fill_bytes(&mut self, dest: &mut [u8]) -> Result<(), Self::Error> {
        utils::fill_bytes_via_next_word(dest, || self.try_next_u32())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn reference() {
        let mut rng =
            Xoshiro128StarStar::from_seed([1, 0, 0, 0, 2, 0, 0, 0, 3, 0, 0, 0, 4, 0, 0, 0]);
        // These values were produced with the reference implementation (v1.1):
        // http://xoshiro.di.unimi.it/xoshiro128starstar.c
        let expected = [
            11520, 0, 5927040, 70819200, 2031721883, 1637235492, 1287239034, 3734860849,
            3729100597, 4258142804,
        ];
        for &e in &expected {
            assert_eq!(rng.next_u32(), e);
        }
    }

    #[test]
    fn test_jump() {
        let mut rng =
            Xoshiro128StarStar::from_seed([1, 0, 0, 0, 2, 0, 0, 0, 3, 0, 0, 0, 4, 0, 0, 0]);
        rng.jump();
        // These values were produced by instrumenting the reference implementation:
        // http://xoshiro.di.unimi.it/xoshiro128plus.c
        assert_eq!(rng.s[0], 2843103750);
        assert_eq!(rng.s[1], 2038079848);
        assert_eq!(rng.s[2], 1533207345);
        assert_eq!(rng.s[3], 44816753);
    }

    #[test]
    fn test_long_jump() {
        let mut rng =
            Xoshiro128StarStar::from_seed([1, 0, 0, 0, 2, 0, 0, 0, 3, 0, 0, 0, 4, 0, 0, 0]);
        rng.long_jump();
        // These values were produced by instrumenting the reference implementation:
        // http://xoshiro.di.unimi.it/xoshiro128plus.c
        assert_eq!(rng.s[0], 1611968294);
        assert_eq!(rng.s[1], 2125834322);
        assert_eq!(rng.s[2], 966769569);
        assert_eq!(rng.s[3], 3193880526);
    }

    #[test]
    fn zero_seed_maps_to_seed_from_u64_zero() {
        let from_zero = Xoshiro128StarStar::from_seed([0u8; 16]);
        let from_sm0 = Xoshiro128StarStar::seed_from_u64(0);
        assert_eq!(from_zero, from_sm0);
    }

    #[test]
    fn state_roundtrip() {
        let rng = Xoshiro128StarStar::seed_from_u64(42);
        let clone = Xoshiro128StarStar::from_seed(rng.state());
        assert_eq!(clone, rng);
    }
}
