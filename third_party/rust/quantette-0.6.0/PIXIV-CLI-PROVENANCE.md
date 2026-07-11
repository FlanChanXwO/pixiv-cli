# Vendored provenance

- Package: `quantette 0.6.0`
- crates.io source cached by Cargo; source commit recorded by `.cargo_vcs_info.json`
- Upstream tag: `v0.6.0`
- Upstream commit: `066855ad3efb4143c3b901c812f80b9b17f8f303`
- Repository: https://github.com/IanManske/quantette

The crates.io package declares `MIT OR Apache-2.0` but its `include = ["src"]`
rule omits both license files. `LICENSE-MIT` and `LICENSE-APACHE` were restored
verbatim from the upstream tag so release license generation can fail closed.

The vendored build manifest omits upstream-only benchmark/dev dependencies and
relaxes deprecated/unused warnings caused by newer transitive `wide` releases.
The library source under `src/` is unchanged from the published crate.

SHA-256:

- `LICENSE-MIT`: `8f2b430687de9562bfac3b9c2e58d5989d2f0b95772928e94c23a942878c919a`
- `LICENSE-APACHE`: `aac73b3148f6d1d7111dbca32099f68d26c644c6813ae1e4f05f6579aa2663fe`
