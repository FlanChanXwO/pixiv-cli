# quantette

[![Crate](https://badgen.net/crates/v/quantette)](https://crates.io/crates/quantette)
[![Docs](https://docs.rs/quantette/badge.svg)](https://docs.rs/quantette)

`quantette` is a library for fast and high quality image quantization and palette generation. It uses the [Oklab](https://bottosson.github.io/posts/oklab) color space for more accurate quantization (but still supports sRGB and other color spaces). `quantette`'s k-means color quantizer gives high quality results while the included Wu color quantizer gives fast but still quite good results.

In some critical locations, `quantette` makes use of SIMD (via the [`wide`](https://crates.io/crates/wide) crate). Consider enabling the `avx` or `avx2` [target features](https://doc.rust-lang.org/reference/conditional-compilation.html#target_feature) for a noticeable speed up if your target architecture supports these features. If the `threads` cargo feature is enabled, multi-threaded versions of most functions become available for even greater speed up.

See [Examples](docs/Examples.md) for example output images from `quantette` and see [Benchmarks and Accuracy](docs/Benchmarks%20and%20Accuracy.md) for comparisons with other libraries.

# License

`quantette` is licensed under either
- the [Apache License, Version 2.0](https://www.apache.org/licenses/LICENSE-2.0) (see [LICENSE-APACHE](LICENSE-APACHE))
- the [MIT](http://opensource.org/licenses/MIT) license (see [LICENSE-MIT](LICENSE-MIT))

at your option.
