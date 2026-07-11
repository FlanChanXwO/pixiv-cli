#![cfg(feature = "serde")]

use rand_core::{Rng, SeedableRng};
use rand_xoshiro::{
    SplitMix64, Xoroshiro64Star, Xoroshiro64StarStar, Xoroshiro128Plus, Xoroshiro128StarStar,
    Xoshiro128Plus, Xoshiro128StarStar, Xoshiro256Plus, Xoshiro256StarStar, Xoshiro512Plus,
    Xoshiro512StarStar,
};

macro_rules! serde_rng {
    ($rng:ident) => {
        let mut rng = $rng::seed_from_u64(0);

        let buf = postcard::to_allocvec(&rng).expect("Could not serialize");

        let mut deserialized: $rng = postcard::from_bytes(&buf).expect("Could not deserialize");

        for _ in 0..16 {
            assert_eq!(rng.next_u64(), deserialized.next_u64());
        }
    };
}

#[test]
fn test_splitmix64() {
    serde_rng!(SplitMix64);
}

#[test]
fn test_xoroshiro64starstar() {
    serde_rng!(Xoroshiro64StarStar);
}

#[test]
fn test_xoroshiro64star() {
    serde_rng!(Xoroshiro64Star);
}

#[test]
fn test_xoroshiro128plus() {
    serde_rng!(Xoroshiro128Plus);
}

#[test]
fn test_xoroshiro128starstar() {
    serde_rng!(Xoroshiro128StarStar);
}

#[test]
fn test_xoshiro128starstar() {
    serde_rng!(Xoshiro128StarStar);
}

#[test]
fn test_xoshiro128plus() {
    serde_rng!(Xoshiro128Plus);
}

#[test]
fn test_xoshiro256starstar() {
    serde_rng!(Xoshiro256StarStar);
}

#[test]
fn test_xoshiro256plus() {
    serde_rng!(Xoshiro256Plus);
}

#[test]
fn test_xoshiro512starstar() {
    serde_rng!(Xoshiro512StarStar);
}

#[test]
fn test_xoshiro512plus() {
    serde_rng!(Xoshiro512Plus);
}
