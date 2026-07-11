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

/// A xoshiro256+ random number generator.
///
/// The xoshiro256+ algorithm is not suitable for cryptographic purposes, but
/// is very fast and has good statistical properties, besides a low linear
/// complexity in the lowest bits.
///
/// The algorithm used here is translated from [the `xoshiro256plus.c`
/// reference source code](http://xoshiro.di.unimi.it/xoshiro256plus.c) by
/// David Blackman and Sebastiano Vigna.
#[derive(Debug, Clone, PartialEq, Eq)]
#[cfg_attr(feature = "serde", derive(Serialize, Deserialize))]
pub struct Xoshiro256Plus {
    s: [u64; 4],
}

impl Xoshiro256Plus {
    /// Jump forward, equivalently to 2^128 calls to `next_u64()`.
    ///
    /// This can be used to generate 2^128 non-overlapping subsequences for
    /// parallel computations.
    ///
    /// ```
    /// use rand_xoshiro::rand_core::SeedableRng;
    /// use rand_xoshiro::Xoshiro256Plus;
    ///
    /// let rng1 = Xoshiro256Plus::seed_from_u64(0);
    /// let mut rng2 = rng1.clone();
    /// rng2.jump();
    /// let mut rng3 = rng2.clone();
    /// rng3.jump();
    /// ```
    pub fn jump(&mut self) {
        impl_jump!(
            u64,
            self,
            [
                0x180ec6d33cfd0aba,
                0xd5a61266f0c9392c,
                0xa9582618e03fc9aa,
                0x39abdc4529b1661c
            ]
        );
    }

    /// Jump forward, equivalently to 2^192 calls to `next_u64()`.
    ///
    /// This can be used to generate 2^64 starting points, from each of which
    /// `jump()` will generate 2^64 non-overlapping subsequences for parallel
    /// distributed computations.
    pub fn long_jump(&mut self) {
        impl_jump!(
            u64,
            self,
            [
                0x76e15d3efefdcbbf,
                0xc5004e441c522fb3,
                0x77710069854ee241,
                0x39109bb02acbe635
            ]
        );
    }
}

impl_state_array_of_four!(Xoshiro256Plus, u64);

impl SeedableRng for Xoshiro256Plus {
    type Seed = [u8; 32];

    /// Create a new `Xoshiro256Plus`.  If `seed` is entirely 0, it will be
    /// mapped to a different seed.
    #[inline]
    fn from_seed(seed: [u8; 32]) -> Xoshiro256Plus {
        Xoshiro256Plus {
            s: utils::read_words(crate::common::zero_seed_fallback(&seed)),
        }
    }

    /// Seed a `Xoshiro256Plus` from a `u64` using `SplitMix64`.
    fn seed_from_u64(seed: u64) -> Xoshiro256Plus {
        from_splitmix!(seed)
    }
}

impl TryRng for Xoshiro256Plus {
    type Error = Infallible;

    #[inline]
    fn try_next_u32(&mut self) -> Result<u32, Self::Error> {
        // The lowest bits have some linear dependencies, so we use the
        // upper bits instead.
        Ok((self.next_u64() >> 32) as u32)
    }

    #[inline]
    fn try_next_u64(&mut self) -> Result<u64, Self::Error> {
        let result_plus = self.s[0].wrapping_add(self.s[3]);
        impl_xoshiro_u64!(self);
        Ok(result_plus)
    }

    #[inline]
    fn try_fill_bytes(&mut self, dest: &mut [u8]) -> Result<(), Self::Error> {
        utils::fill_bytes_via_next_word(dest, || self.try_next_u64())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn reference() {
        let mut rng = Xoshiro256Plus::from_seed([
            1, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0,
            0, 0, 0,
        ]);
        // These values were produced with the reference implementation:
        // http://xoshiro.di.unimi.it/xoshiro256plus.c
        let expected = [
            5,
            211106232532999,
            211106635186183,
            9223759065350669058,
            9250833439874351877,
            13862484359527728515,
            2346507365006083650,
            1168864526675804870,
            34095955243042024,
            3466914240207415127,
        ];
        for &e in &expected {
            assert_eq!(rng.next_u64(), e);
        }
    }

    #[test]
    fn zero_seed_maps_to_seed_from_u64_zero() {
        let from_zero = Xoshiro256Plus::from_seed([0u8; 32]);
        let from_sm0 = Xoshiro256Plus::seed_from_u64(0);
        assert_eq!(from_zero, from_sm0);
    }

    #[test]
    fn state_roundtrip() {
        let rng = Xoshiro256Plus::seed_from_u64(42);
        let clone = Xoshiro256Plus::from_seed(rng.state());
        assert_eq!(clone, rng);
    }
}
