# rand_xoshiro

[![Test Status](https://github.com/rust-random/rngs/actions/workflows/test.yml/badge.svg?event=push)](https://github.com/rust-random/rngs/actions)
[![Latest version](https://img.shields.io/crates/v/rand_xoshiro.svg)](https://crates.io/crates/rand_xoshiro)
[![Book](https://img.shields.io/badge/book-master-yellow.svg)](https://rust-random.github.io/book/)
[![API](https://docs.rs/rand_xoshiro/badge.svg)](https://docs.rs/rand_xoshiro)

Rust implementation of the [xoshiro, xoroshiro and splitmix64](http://xoshiro.di.unimi.it) random number generators.

This crate depends on [rand_core](https://crates.io/crates/rand_core) and is
part of the [Rand project](https://github.com/rust-random/rand).

Links:

-   [API documentation (docs.rs)](https://docs.rs/rand_xoshiro)
-   [Changelog](https://github.com/rust-random/rngs/blob/master/rand_xoshiro/CHANGELOG.md)

## Crate Features

`rand_xoshiro` is no_std compatible by default.

The `serde` feature includes implementations of `Serialize` and `Deserialize` for the included RNGs.

## License

`rand_xoshiro` is distributed under the terms of both the MIT license and the
Apache License (Version 2.0).

See [LICENSE-APACHE](LICENSE-APACHE) and [LICENSE-MIT](LICENSE-MIT), and
[COPYRIGHT](COPYRIGHT) for details.
