# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.8.1] - 2026-05-16
### Added
- `state()` method on every RNG type returning the internal state as a value
  matching `SeedableRng::Seed`, allowing reproduction via `from_seed` without
  the `serde` feature ([#110]).

### Changed
- Shrink the code size of the all-zero-seed fallback by replacing the per-RNG
  `SplitMix64::seed_from_u64(0)` call with a single shared constant.

[#110]: https://github.com/rust-random/rngs/pull/110

## [0.8.0] - 2026-02-01
### Changes
- Use Edition 2024 and MSRV 1.85 ([#73])
- Update to `rand_core` v0.10 ([#82])

[#73]: https://github.com/rust-random/rngs/pull/73
[#82]: https://github.com/rust-random/rngs/pull/82

## [0.7.0] - 2025-01-27
- Bump the MSRV to 1.63 ([#58])
- Update to `rand_core` v0.9.0 ([#58])
- Rename feature `serde1` to `serde` ([#58])
- Add `Xoshiro128Plus::long_jump`
- Add examples for initializing the RNGs
- Speed up `from_seed` implementation for 128-bit seeds
- Fix a few typos in the docs

[#58]: https://github.com/rust-random/rngs/pull/58

## [0.6.0] - 2020-12-18
- Bump `rand_core` version to 0.6 ([#17])

[#17]: https://github.com/rust-random/rngs/pull/17

## [0.5.0] - 2020-09-11
- Derive PartialEq+Eq for SplitMix64, Xoroshiro64Star, Xoroshiro64StarStar,
  Xoroshiro128Plus, Xoroshiro128PlusPlus, Xoroshiro128StarStar,
  Xoshiro128Plus, Xoshiro128PlusPlus, Xoshiro128StarStar, Xoshiro256Plus,
  Xoshiro256PlusPlus, Xoshiro256StarStar, Xoshiro512Plus, Xoshiro512PlusPlus,
  and Xoshiro512StarStar ([#6])
- `next_u32`: Prefer upper bits for `Xoshiro256{PlusPlus,StarStar}` and
  `Xoshiro512{Plus,PlusPlus,StarStar}`, breaking value stability

[#6]: https://github.com/rust-random/rngs/pull/6

## [0.4.0] - 2019-09-03
- Add xoshiro128++, 256++ and 512++ variants
- Add xoroshiro128++ variant
- Add `long_jump` method to RNGs missing it
- Update xoshiro128** to version 1.1, breaking value stability

## [0.3.1] - 2019-08-06
- Drop `byteorder`-dependency in favor of `stdlib`-implementation.

## [0.3.0] - 2019-06-12
- Bump minor crate version since rand_core bump is a breaking change
- Switch to Edition 2018

## [0.2.1] - 2019-06-06 - yanked
- Bump `rand_core` version
- Document crate features in README

## [0.2.0] - 2019-05-28
- Fix `seed_from_u64(0)` for `Xoroshiro64StarStar` and `Xoroshiro64Star`. This
  breaks value stability for these generators if initialized with `seed_from_u64`.
- Implement Serde support.

## [0.1.0] - 2019-01-04
Initial release.
