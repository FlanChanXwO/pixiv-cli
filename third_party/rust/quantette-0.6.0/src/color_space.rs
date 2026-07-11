//! Efficient color space conversion.

use alloc::vec::Vec;
use core::array;
use palette::{LinSrgb, Oklab, Srgb, cast};
use wide::{CmpEq as _, CmpGe as _, CmpLe as _, f32x8, u32x8};

// https://bottosson.github.io/posts/oklab/#converting-from-linear-srgb-to-oklab

/// SIMD lane-wise cube root.
#[inline]
fn cbrt(x: f32x8) -> f32x8 {
    // See: [`libm::cbrtf`]
    /* origin: FreeBSD /usr/src/lib/msun/src/s_cbrtf.c */
    /*
     * Conversion to float by Ian Lance Taylor, Cygnus Support, ian@cygnus.com.
     * Debugged and optimized by Bruce D. Evans.
     */
    /*
     * ====================================================
     * Copyright (C) 1993 by Sun Microsystems, Inc. All rights reserved.
     *
     * Developed at SunPro, a Sun Microsystems, Inc. business.
     * Permission to use, copy, modify, and distribute this
     * software is freely granted, provided that this notice
     * is preserved.
     * ====================================================
     */

    // General method and algorithm taken from the above as well as
    // https://web.archive.org/web/20131227144655/http://metamerist.com/cbrt/cbrt.htm
    // and adapted to SIMD.

    const MU: f64 = 0.049593534765;
    const BIAS: f64 = (f32::MAX_EXP - 1) as f64;
    const EXP_SHIFT: f64 = (1 << (f32::MANTISSA_DIGITS - 1)) as f64;
    #[allow(clippy::cast_possible_truncation, clippy::cast_sign_loss)]
    const C: u32 = ((BIAS - BIAS / 3.0 - MU * 2.0 / 3.0) * EXP_SHIFT) as u32;

    // We assume the input was originally converted from Srgb<u8> and so is within certain ranges.
    // This eliminates additional code needed to handle infinity, subnormal numbers, etc.
    debug_assert!((x.simd_eq(0.0) | (x.simd_ge(f32::MIN_POSITIVE) & x.simd_le(1.0))).all());

    let x_bits: u32x8 = bytemuck::cast(x);
    let sign = x_bits & u32x8::splat(0x80000000);
    let mut without_sign = x_bits & u32x8::splat(0x7fffffff);
    // compiler should vectorize this for us (wasm reverts to scalar unfortunately)
    for x in without_sign.as_mut_array() {
        *x /= 3;
    }
    let without_sign = without_sign + u32x8::splat(C);
    let x_bits = sign | without_sign;

    // Running Halley's Method for 2 iterations.
    // We use f32 instead of f64 for speed, sacrificing some precision.
    // Conversion from Srgb<u8> to Oklab still roundtrips.
    let a: f32x8 = bytemuck::cast(x_bits);

    let a3 = a * a * a;
    let a = a * (x + x + a3) / (x + a3 + a3);

    let a3 = a * a * a;
    let a = a * (x + x + a3) / (x + a3 + a3);

    x.simd_eq(0.0).blend(x, a)
}
/// Convert [`LinSrgb`] to [`Oklab`] using SIMD.
#[allow(clippy::excessive_precision, clippy::inline_always)]
#[inline(always)] // around 15-40% slow down if not inlined
fn lin_srgb_to_oklab(lin_srgb: LinSrgb<f32x8>) -> Oklab<f32x8> {
    let (r, g, b) = lin_srgb.into_components();

    let l = 0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b;
    let m = 0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b;
    let s = 0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b;

    let l_ = cbrt(l);
    let m_ = cbrt(m);
    let s_ = cbrt(s);

    let l = 0.2104542553 * l_ + 0.7936177850 * m_ - 0.0040720468 * s_;
    let a = 1.9779984951 * l_ + -2.4285922050 * m_ + 0.4505937099 * s_;
    let b = 0.0259040371 * l_ + 0.7827717662 * m_ - 0.8086757660 * s_;

    Oklab { l, a, b }
}

/// Convert a [`LinSrgb`] with [`f32x8`] components to an array of [`Oklab`] values.
#[allow(clippy::inline_always)]
#[inline(always)]
fn lin_srgb_to_oklab_arr(lin_srgb: LinSrgb<f32x8>) -> [Oklab; 8] {
    let Oklab { l, a, b } = lin_srgb_to_oklab(lin_srgb);
    array::from_fn(|i| Oklab {
        l: l.as_array()[i],
        a: a.as_array()[i],
        b: b.as_array()[i],
    })
}

/// Convert a slice of [`Srgb<u8>`] colors to [`Oklab`] colors.
pub fn srgb8_to_oklab(input: &[Srgb<u8>]) -> Vec<Oklab> {
    let mut output = bytemuck::zeroed_vec(input.len());
    let (out_chunks, out_remainder) = output.as_chunks_mut::<8>();
    let (in_chunks, in_remainder) = input.as_chunks::<8>();

    for (chunk, output) in in_chunks.iter().zip(out_chunks) {
        let mut arr = [[0.0; 8]; 3];
        for (i, srgb) in chunk.iter().enumerate() {
            let lin_srgb = cast::into_array(srgb.into_linear());
            for (arr, c) in arr.iter_mut().zip(lin_srgb) {
                arr[i] = c;
            }
        }
        *output = lin_srgb_to_oklab_arr(cast::from_array(arr.map(f32x8::new)));
    }

    if !in_remainder.is_empty() {
        let mut arr = [[0.0; 8]; 3];
        for (i, srgb) in in_remainder.iter().enumerate() {
            let lin_srgb = cast::into_array(srgb.into_linear());
            for (arr, c) in arr.iter_mut().zip(lin_srgb) {
                arr[i] = c;
            }
        }
        let oklab = lin_srgb_to_oklab_arr(cast::from_array(arr.map(f32x8::new)));
        out_remainder.copy_from_slice(&oklab[..in_remainder.len()]);
    }

    output
}

/// Convert [`Oklab`] to [`LinSrgb`] using SIMD.
#[allow(clippy::excessive_precision, clippy::inline_always)]
#[inline(always)]
fn oklab_to_lin_srgb(oklab: Oklab<f32x8>) -> LinSrgb<f32x8> {
    let Oklab { l, a, b } = oklab;

    let l_ = l + 0.3963377774 * a + 0.2158037573 * b;
    let m_ = l - 0.1055613458 * a - 0.0638541728 * b;
    let s_ = l - 0.0894841775 * a - 1.2914855480 * b;

    let l = l_ * l_ * l_;
    let m = m_ * m_ * m_;
    let s = s_ * s_ * s_;

    let r = 4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s;
    let g = -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s;
    let b = -0.0041960863 * l - 0.7034186147 * m + 1.7076147010 * s;

    LinSrgb::new(r, g, b)
}

/// Convert an [`Oklab`] with [`f32x8`] components to an array of [`LinSrgb`] values.
#[allow(clippy::excessive_precision, clippy::inline_always)]
#[inline(always)] // around 5-7% slowdown if not inlined
fn oklab_arr_to_lin_srgb(oklab: [Oklab; 8]) -> LinSrgb<f32x8> {
    let oklab = cast::into_array_array(oklab);
    let oklab: [[f32; 8]; 3] = core::array::from_fn(|i| oklab.map(|x| x[i]));
    let [l, a, b] = oklab.map(f32x8::new);
    oklab_to_lin_srgb(Oklab { l, a, b })
}

/// Convert a slice of [`Oklab`] colors to [`Srgb<u8>`] colors.
///
/// [`Oklab`] colors outside the [`Srgb`] range are clamped to be inside.
pub fn oklab_to_srgb8(input: &[Oklab]) -> Vec<Srgb<u8>> {
    let mut output = bytemuck::zeroed_vec(input.len());
    let (out_chunks, out_remainder) = output.as_chunks_mut::<8>();
    let (in_chunks, in_remainder) = input.as_chunks::<8>();

    for (&chunk, output) in in_chunks.iter().zip(out_chunks) {
        let lin_srgb = oklab_arr_to_lin_srgb(chunk);
        for (i, output) in output.iter_mut().enumerate() {
            // TODO: use fast_srgb::f32x4_to_srgb8
            *output = LinSrgb::new(
                lin_srgb.red.as_array()[i],
                lin_srgb.green.as_array()[i],
                lin_srgb.blue.as_array()[i],
            )
            .into_encoding();
        }
    }

    if !in_remainder.is_empty() {
        let mut oklab = [Oklab::new(0.0, 0.0, 0.0); 8];
        oklab[..in_remainder.len()].copy_from_slice(in_remainder);
        let lin_srgb = oklab_arr_to_lin_srgb(oklab);
        for (i, output) in out_remainder.iter_mut().enumerate() {
            *output = LinSrgb::new(
                lin_srgb.red.as_array()[i],
                lin_srgb.green.as_array()[i],
                lin_srgb.blue.as_array()[i],
            )
            .into_encoding();
        }
    }

    output
}

#[cfg(feature = "threads")]
mod parallel {
    use super::{lin_srgb_to_oklab_arr, oklab_arr_to_lin_srgb};
    use core::array;
    use palette::{LinSrgb, Oklab, Srgb, cast};
    use rayon::prelude::*;
    use wide::f32x8;

    /// Convert a slice of [`Srgb<u8>`] colors to [`Oklab`] colors in parallel.
    pub fn srgb8_to_oklab_par(input: &[Srgb<u8>]) -> Vec<Oklab> {
        let mut output = Vec::<[Oklab; 8]>::with_capacity(input.len().div_ceil(8));
        let (chunks, remainder) = input.as_chunks::<8>();

        chunks
            .par_iter()
            .map(|chunk| {
                let mut arr = [[0.0; 8]; 3];
                for (i, srgb) in chunk.iter().enumerate() {
                    let lin_srgb = cast::into_array(srgb.into_linear());
                    for (arr, c) in arr.iter_mut().zip(lin_srgb) {
                        arr[i] = c;
                    }
                }
                lin_srgb_to_oklab_arr(cast::from_array(arr.map(f32x8::new)))
            })
            .collect_into_vec(&mut output);

        let mut output = output.into_flattened();
        if !remainder.is_empty() {
            let mut arr = [[0.0; 8]; 3];
            for (i, srgb) in remainder.iter().enumerate() {
                let lin_srgb = cast::into_array(srgb.into_linear());
                for (arr, c) in arr.iter_mut().zip(lin_srgb) {
                    arr[i] = c;
                }
            }
            let oklab = lin_srgb_to_oklab_arr(cast::from_array(arr.map(f32x8::new)));
            output.extend_from_slice(&oklab[..remainder.len()]);
        }

        output
    }

    /// Convert a slice of [`Oklab`] colors to [`Srgb<u8>`] colors in parallel.
    ///
    /// [`Oklab`] colors outside the [`Srgb`] range are clamped to be inside.
    pub fn oklab_to_srgb8_par(input: &[Oklab]) -> Vec<Srgb<u8>> {
        let mut output = Vec::<[Srgb<u8>; 8]>::with_capacity(input.len().div_ceil(8));
        let (chunks, remainder) = input.as_chunks::<8>();

        chunks
            .par_iter()
            .with_min_len(2)
            .map(|&chunk| {
                let lin_srgb = oklab_arr_to_lin_srgb(chunk);
                array::from_fn(|i| {
                    LinSrgb::new(
                        lin_srgb.red.as_array()[i],
                        lin_srgb.green.as_array()[i],
                        lin_srgb.blue.as_array()[i],
                    )
                    .into_encoding()
                })
            })
            .collect_into_vec(&mut output);

        let mut output = output.into_flattened();
        if !remainder.is_empty() {
            let mut oklab = [Oklab::new(0.0, 0.0, 0.0); 8];
            oklab[..remainder.len()].copy_from_slice(remainder);
            let lin_srgb = oklab_arr_to_lin_srgb(oklab);
            let srgb: [Srgb<u8>; 8] = array::from_fn(|i| {
                LinSrgb::new(
                    lin_srgb.red.as_array()[i],
                    lin_srgb.green.as_array()[i],
                    lin_srgb.blue.as_array()[i],
                )
                .into_encoding()
            });
            output.extend(&srgb[..remainder.len()]);
        }

        output
    }
}

#[cfg(feature = "threads")]
pub use parallel::*;

#[cfg(test)]
mod tests {
    use super::*;
    use core::array;
    use palette::{FromColor as _, LinSrgb, Oklab, Srgb, cast};
    use wide::f32x8;

    #[test]
    #[ignore = "takes a long time"]
    fn lin_srgb_to_oklab_oracle() {
        for r in 0..=u8::MAX {
            for g in 0..=u8::MAX {
                for b in (0..=u8::MAX).step_by(8) {
                    #[allow(clippy::cast_possible_truncation)]
                    let lin_srgb: [_; 8] =
                        array::from_fn(|i| Srgb::new(r, g, b + i as u8).into_linear());

                    let actual = lin_srgb_to_oklab_arr(LinSrgb::new(
                        f32x8::new(lin_srgb.map(|x| x.red)),
                        f32x8::new(lin_srgb.map(|x| x.green)),
                        f32x8::new(lin_srgb.map(|x| x.blue)),
                    ));
                    let expected = lin_srgb.map(Oklab::from_color);

                    for (actual, expected) in actual.into_iter().zip(expected) {
                        let [l, a, b] = cast::into_array(actual - expected).map(f32::abs);
                        assert!(l < 0.000001, "difference in l = {l} with ({r}, {g}, {b})");
                        assert!(a < 0.000001, "difference in a = {a} with ({r}, {g}, {b})");
                        assert!(b < 0.000001, "difference in b = {b} with ({r}, {g}, {b})");
                    }
                }
            }
        }
    }

    #[test]
    #[ignore = "takes a long time"]
    fn srgb8_to_oklab_roundtrip() {
        #[allow(clippy::cast_possible_truncation)]
        let mut srgb: [_; 256 * 256] =
            array::from_fn(|i| Srgb::new(0, (i / 256) as u8, (i % 256) as u8));

        let srgb = &mut srgb;

        for r in 0..=u8::MAX {
            for srgb in &mut *srgb {
                srgb.red = r;
            }
            let oklab = srgb8_to_oklab(srgb);
            let actual = oklab_to_srgb8(&oklab);

            for (actual, expected) in actual.iter().zip(&*srgb) {
                assert_eq!(actual, expected);
            }
        }
    }
}
