//! Functions to deduplicate pixels into a [`Vec`], [`PaletteCounts`], [`IndexedImage`],
//! or [`IndexedImageCounts`].

use crate::{
    BoundedSlice, ColorComponents, ImageRef, IndexedImage, IndexedImageCounts, LengthOutOfRange,
    MAX_PIXELS, PaletteCounts,
};
use alloc::{boxed::Box, vec, vec::Vec};
use bitvec::vec::BitVec;
use bytemuck::Zeroable;
use core::ops::Range;
use palette::cast::{self, AsArrays as _};
use wide::{CmpEq as _, u8x32};

/// A byte-sized Radix.
const RADIX: usize = u8::MAX as usize + 1;

/// Returns the range associated with the `i`-th chunk.
#[inline]
fn chunk_range(chunks: &[u32], i: usize) -> Range<usize> {
    (chunks[i] as usize)..(chunks[i + 1] as usize)
}
/// Count the first bytes of each pixel.
fn chunked_u8_counts<const CHUNKS: usize>(
    pixels: &[[u8; 3]],
    counts: &mut [[u32; RADIX + 1]; CHUNKS],
) {
    let (chunks, remainder) = pixels.as_chunks::<CHUNKS>();
    for chunk in chunks {
        for (counts, &[r, ..]) in counts.iter_mut().zip(chunk) {
            counts[usize::from(r)] += 1;
        }
    }
    for &[r, ..] in remainder {
        counts[0][usize::from(r)] += 1;
    }

    #[allow(clippy::expect_used)]
    let (counts, partial_counts) = counts.split_first_mut().expect("CHUNKS != 0");
    for i in 0..RADIX {
        for partial in &*partial_counts {
            counts[i] += partial[i];
        }
    }
}

#[inline]
fn prefix_sum<const N: usize>(counts: &mut [u32; N]) {
    for i in 1..N {
        counts[i] += counts[i - 1];
    }
}

fn dedup_colors_u8_3_bounded<Color: ColorComponents<u8, 3>>(
    slice: &BoundedSlice<Color>,
) -> Vec<Color> {
    let mut bitmask: BitVec = BitVec::repeat(false, RADIX * RADIX * RADIX);
    for color in slice.as_arrays() {
        let [r, g, b] = color.map(usize::from);
        bitmask.set(r * RADIX * RADIX + g * RADIX + b, true);
    }

    let mut colors = Vec::new();
    for i in bitmask.iter_ones() {
        let r = i / (RADIX * RADIX);
        let g = i % (RADIX * RADIX) / RADIX;
        let b = i % (RADIX * RADIX) % RADIX;
        #[allow(clippy::cast_possible_truncation)]
        let color = cast::from_array([r as u8, g as u8, b as u8]);
        colors.push(color);
    }
    colors
}

/// Deduplicate a slice of colors into a [`Vec`] of unique colors.
///
/// The color type must consist of 3 `u8`s.
///
/// # Errors
///
/// Returns an error if the length of `slice` is greater than [`MAX_PIXELS`].
#[inline]
pub fn dedup_colors_u8_3<Color: ColorComponents<u8, 3>>(
    slice: &[Color],
) -> Result<Vec<Color>, LengthOutOfRange> {
    LengthOutOfRange::check_u32(slice, 0, MAX_PIXELS)?;
    Ok(BoundedSlice::new(slice)
        .map(dedup_colors_u8_3_bounded)
        .unwrap_or_default())
}

/// Create a lookup table for the starting index of each chunk of colors that have the same two
/// first bytes.
fn create_prefix_lookup(palette: &[[u8; 3]]) -> Box<[[u32; RADIX]; RADIX]> {
    let mut lookup = bytemuck::zeroed_box::<[[u32; RADIX]; RADIX]>();
    let mut i = 0;
    for chunk in palette.chunk_by(|&[r1, g1, _], &[r2, g2, _]| r1 == r2 && g1 == g2) {
        let [r, g, _] = chunk[0];
        lookup[usize::from(r)][usize::from(g)] = i;
        #[allow(clippy::cast_possible_truncation)]
        {
            i += chunk.len() as u32;
        }
    }
    lookup
}

/// Find the index of `rgb` according to the rg index `lookup` table and list of `blue`.
#[inline]
fn find_index([r, g, b]: [u8; 3], lookup: &[[u32; RADIX]; RADIX], blue: &[u8]) -> u32 {
    let index = lookup[usize::from(r)][usize::from(g)];
    let i = index as usize;
    #[allow(clippy::expect_used)]
    let slice: &[u8; RADIX] = blue[i..(i + RADIX)]
        .try_into()
        .expect("slice of exactly length RADIX");
    let slice: &[[u8; 32]; RADIX / 32] = bytemuck::cast_ref(slice);
    let b = u8x32::splat(b);
    for (chunk_i, &chunk) in slice.iter().enumerate() {
        let mask = u8x32::new(chunk).simd_eq(b).to_bitmask();
        if mask != 0 {
            #[cfg(target_endian = "big")]
            let mask = mask.swap_bytes();
            #[allow(clippy::cast_possible_truncation)]
            return chunk_i as u32 * 32 + mask.trailing_zeros() + index;
        }
    }
    #[allow(clippy::unreachable)]
    {
        unreachable!("target byte should exist")
    }
}

/// Calculate the palette indices for each pixel.
fn indices_from_palette_u8_3<Color: ColorComponents<u8, 3>>(
    palette: &[Color],
    pixels: &[Color],
) -> Vec<u32> {
    let lookup = create_prefix_lookup(palette.as_arrays());
    let lookup = lookup.as_ref();

    let blue = palette
        .as_arrays()
        .iter()
        .map(|&[_, _, b]| b)
        .chain(core::iter::repeat_n(0, RADIX))
        .collect::<Vec<_>>();

    pixels
        .as_arrays()
        .iter()
        .map(|&rgb| find_index(rgb, lookup, &blue))
        .collect()
}

/// Deduplicate an image into an [`IndexedImage`].
///
/// The color type must consist of 3 `u8`s.
#[must_use]
pub fn dedup_image_u8_3<Color: ColorComponents<u8, 3>>(
    image: ImageRef<'_, Color>,
) -> IndexedImage<Color, u32> {
    if image.is_empty() {
        return IndexedImage::default();
    }

    let pixels = image.as_slice();
    let palette = dedup_colors_u8_3_bounded(BoundedSlice::new_unchecked(pixels));
    let indices = indices_from_palette_u8_3(&palette, pixels);
    IndexedImage::new_unchecked(image.width(), image.height(), palette, indices)
}

/// Deduplicate a slice of colors into a [`PaletteCounts`].
///
/// The color type must consist of 3 `u8`s.
pub(crate) fn dedup_colors_u8_3_counts_bounded<Color: ColorComponents<u8, 3>>(
    slice: &BoundedSlice<Color>,
) -> PaletteCounts<Color> {
    let total_count = slice.length();
    let mut colors = Vec::new();
    let mut counts = Vec::new();
    let mut green_blue = vec![[0; 2]; slice.len()];

    let mut lower_counts = bytemuck::zeroed_box::<[[u32; RADIX]; RADIX]>();
    let mut bitmask: BitVec = BitVec::repeat(false, RADIX * RADIX);

    let mut red_prefix = bytemuck::zeroed_box();
    chunked_u8_counts::<4>(slice.as_arrays(), &mut red_prefix);
    let red_prefix = &mut red_prefix[0];
    prefix_sum(red_prefix);

    for &[r, g, b] in slice.as_arrays() {
        let r = usize::from(r);
        let j = red_prefix[r] - 1;
        green_blue[j as usize] = [g, b];
        red_prefix[r] = j;
    }
    red_prefix[RADIX] = total_count;

    for r in 0..RADIX {
        let chunk = chunk_range(red_prefix, r);

        if !chunk.is_empty() {
            let green_blue = &green_blue[chunk.clone()];

            if chunk.len() < RADIX * RADIX / 4 {
                for gb in green_blue {
                    let [g, b] = gb.map(usize::from);
                    lower_counts[g][b] += 1;
                    bitmask.set(g * RADIX + b, true);
                }

                for i in bitmask.iter_ones() {
                    let g = i / RADIX;
                    let b = i % RADIX;
                    #[allow(clippy::cast_possible_truncation)]
                    let color = cast::from_array([r as u8, g as u8, b as u8]);
                    colors.push(color);
                    counts.push(lower_counts[g][b]);
                    lower_counts[g][b] = 0;
                }

                bitmask.fill(false);
            } else {
                for &[g, b] in green_blue {
                    lower_counts[usize::from(g)][usize::from(b)] += 1;
                }

                for (g, count) in lower_counts.iter().enumerate() {
                    for (b, &count) in count.iter().enumerate() {
                        if count > 0 {
                            #[allow(clippy::cast_possible_truncation)]
                            let color = cast::from_array([r as u8, g as u8, b as u8]);
                            colors.push(color);
                            counts.push(count);
                        }
                    }
                }

                *lower_counts = Zeroable::zeroed();
            }
        }
    }

    PaletteCounts::new_unchecked(colors, counts, total_count)
}

/// Deduplicate a slice of colors into a [`PaletteCounts`].
///
/// The color type must consist of 3 `u8`s.
///
/// # Errors
///
/// Returns an error if the length of `slice` is greater than [`MAX_PIXELS`].
#[inline]
pub fn dedup_colors_u8_3_counts<Color: ColorComponents<u8, 3>>(
    slice: &[Color],
) -> Result<PaletteCounts<Color>, LengthOutOfRange> {
    LengthOutOfRange::check_u32(slice, 0, MAX_PIXELS)?;
    Ok(BoundedSlice::new(slice)
        .map(dedup_colors_u8_3_counts_bounded)
        .unwrap_or_default())
}

/// Deduplicate an image into an [`IndexedImageCounts`].
///
/// The color type must consist of 3 `u8`s.
#[must_use]
pub fn dedup_image_u8_3_counts<Color: ColorComponents<u8, 3>>(
    image: ImageRef<'_, Color>,
) -> IndexedImageCounts<Color, u32> {
    if image.is_empty() {
        return IndexedImageCounts::default();
    }

    let pixels = image.as_slice();
    let palette_counts = dedup_colors_u8_3_counts_bounded(BoundedSlice::new_unchecked(pixels));
    let indices = indices_from_palette_u8_3(palette_counts.palette(), pixels);
    IndexedImageCounts::from_palette_counts_unchecked(
        image.width(),
        image.height(),
        palette_counts,
        indices,
    )
}

#[cfg(feature = "threads")]
mod parallel {
    use super::{RADIX, chunk_range, chunked_u8_counts};
    use crate::{
        BoundedSlice, ColorComponents, ImageRef, IndexedImage, IndexedImageCounts,
        LengthOutOfRange, MAX_PIXELS, PaletteCounts,
        dedup::{create_prefix_lookup, find_index},
    };
    use bitvec::vec::BitVec;
    use palette::cast::{self, AsArrays as _};
    use rayon::prelude::*;

    /// Unsafe utilities for sharing data across multiple threads.
    #[allow(unsafe_code)]
    mod sync_unsafe {
        #[cfg(test)]
        use core::sync::atomic::{AtomicBool, Ordering};
        use core::{cell::UnsafeCell, ops::Range};

        /// Unsafely share a mutable slice across multiple threads.
        pub struct SyncUnsafeSlice<'a, T> {
            /// The inner [`UnsafeCell`] containing the mutable slice.
            cell: UnsafeCell<&'a mut [T]>,
            /// Check each index is written to only once during tests.
            #[cfg(test)]
            written: Vec<AtomicBool>,
        }

        // Safety: this type is inherently unsafe. Methods on this type are marked as unsafe as necessary.
        unsafe impl<T: Send + Sync> Send for SyncUnsafeSlice<'_, T> {}

        // Safety: this type is inherently unsafe. Methods on this type are marked as unsafe as necessary.
        unsafe impl<T: Send + Sync> Sync for SyncUnsafeSlice<'_, T> {}

        impl<'a, T> SyncUnsafeSlice<'a, T> {
            /// Create a new [`SyncUnsafeSlice`] with the given slice.
            pub fn new(slice: &'a mut [T]) -> Self {
                Self {
                    #[cfg(test)]
                    written: slice.iter().map(|_| AtomicBool::new(false)).collect(),
                    cell: UnsafeCell::new(slice),
                }
            }

            /// Unsafely get the inner mutable reference.
            unsafe fn get(&self) -> &'a mut [T] {
                // Safety: caller ensures no two threads write to the same range or index.
                unsafe { *self.cell.get() }
            }
        }

        impl<T: Copy> SyncUnsafeSlice<'_, T> {
            /// Unsafely write the given slice to the given range.
            ///
            /// # Safety
            /// It is undefined behaviour if two threads write to the same range/indices without synchronization.
            #[inline]
            pub unsafe fn write_slice(&self, range: Range<usize>, slice: &[T]) {
                #[cfg(test)]
                {
                    assert!(
                        !self.written[range.clone()]
                            .iter()
                            .any(|b| b.swap(true, Ordering::SeqCst))
                    );
                }
                // Safety: caller ensures no two threads write to the same range or index.
                (unsafe { self.get() })[range].copy_from_slice(slice);
            }
        }
    }

    use sync_unsafe::SyncUnsafeSlice;

    /// Count the first byte component in the given color slice.
    fn u8_counts(slice: &[[u8; 3]], chunk_size: usize) -> Vec<[u32; RADIX + 1]> {
        let mut counts = slice
            .par_chunks(chunk_size)
            .map(|chunk| {
                let mut counts = bytemuck::zeroed_box();
                chunked_u8_counts::<4>(chunk, &mut counts);
                counts[0]
            })
            .collect::<Vec<_>>();

        let mut carry = 0;
        for i in 0..RADIX {
            counts[0][i] += carry;
            for j in 1..counts.len() {
                counts[j][i] += counts[j - 1][i];
            }
            carry = counts[counts.len() - 1][i];
        }

        debug_assert_eq!(carry as usize, slice.len());

        counts
    }

    /// Writes the green and blue components of the sorted colors.
    fn sorted_green_blue(
        slice: &[[u8; 3]],
        chunk_size: usize,
        red_prefixes: Vec<[u32; RADIX + 1]>,
    ) -> Vec<[u8; 2]> {
        let mut green_blue = vec![[0; 2]; slice.len()];
        let gb = SyncUnsafeSlice::new(&mut green_blue);
        slice
            .par_chunks(chunk_size)
            .zip(red_prefixes)
            .for_each(|(chunk, mut red_prefix)| {
                /// Buffer length
                const BUF_LEN: u8 = 128;

                let mut buffer = bytemuck::zeroed_box::<[[[u8; 2]; BUF_LEN as usize]; RADIX]>();
                let mut lengths = [0u8; RADIX];

                #[allow(unsafe_code)]
                for &[r, g, b] in chunk {
                    let r = usize::from(r);
                    let len = lengths[r];
                    let len = if len >= BUF_LEN {
                        let i = red_prefix[r] - u32::from(BUF_LEN);
                        let j = i as usize;
                        // Safety: prefix sums ensure that each location in green_blue is written to only once
                        // and is therefore safe to write to without any form of synchronization.
                        unsafe {
                            gb.write_slice(j..(j + usize::from(BUF_LEN)), &buffer[r]);
                        }
                        red_prefix[r] = i;
                        0
                    } else {
                        len
                    };
                    buffer[r][usize::from(len)] = [g, b];
                    lengths[r] = len + 1;
                }
                #[allow(unsafe_code)]
                for (r, buf) in buffer.iter().enumerate() {
                    let len = lengths[r];
                    let i = red_prefix[r] - u32::from(len);
                    let len = usize::from(len);
                    let j = i as usize;
                    // Safety: prefix sums ensure that each location in green_blue is written to only once
                    // and is therefore safe to write to without any form of synchronization.
                    unsafe {
                        gb.write_slice(j..(j + len), &buf[..len]);
                    }
                }
            });
        green_blue
    }

    /// Deduplicate a slice of colors in parallel.
    #[inline]
    fn dedup_colors_u8_3_bounded_par<Color: ColorComponents<u8, 3>>(
        slice: &BoundedSlice<Color>,
    ) -> Vec<Color> {
        let chunk_size = slice.len().div_ceil(rayon::current_num_threads());
        let red_prefixes = u8_counts(slice.as_arrays(), chunk_size);
        let red_prefix = {
            let mut prefix = [0; RADIX + 1];
            prefix[1..].copy_from_slice(&red_prefixes[red_prefixes.len() - 1][..RADIX]);
            prefix
        };
        let green_blue = sorted_green_blue(slice.as_arrays(), chunk_size, red_prefixes);

        (0..RADIX)
            .into_par_iter()
            .map(|r| {
                let chunk = chunk_range(&red_prefix, r);

                let mut colors = Vec::new();

                if !chunk.is_empty() {
                    let green_blue = &green_blue[chunk];

                    let mut bitmask: BitVec = BitVec::repeat(false, RADIX * RADIX);
                    for gb in green_blue {
                        let [g, b] = gb.map(usize::from);
                        bitmask.set(g * RADIX + b, true);
                    }

                    for i in bitmask.iter_ones() {
                        let g = i / RADIX;
                        let b = i % RADIX;
                        #[allow(clippy::cast_possible_truncation)]
                        let color = cast::from_array([r as u8, g as u8, b as u8]);
                        colors.push(color);
                    }
                }

                colors
            })
            .collect::<Vec<_>>()
            .concat()
    }

    /// Deduplicate a slice of colors into a [`Vec`] of unique colors in parallel.
    ///
    /// The color type must consist of 3 `u8`s.
    ///
    /// # Errors
    ///
    /// Returns an error if the length of `slice` is greater than [`MAX_PIXELS`].
    #[inline]
    pub fn dedup_colors_u8_3_par<Color: ColorComponents<u8, 3>>(
        slice: &[Color],
    ) -> Result<Vec<Color>, LengthOutOfRange> {
        LengthOutOfRange::check_u32(slice, 0, MAX_PIXELS)?;
        Ok(BoundedSlice::new(slice)
            .map(dedup_colors_u8_3_bounded_par)
            .unwrap_or_default())
    }

    /// Calculate the palette indices for each pixel in parallel.
    fn indices_from_palette_u8_3_par<Color: ColorComponents<u8, 3>>(
        palette: &[Color],
        pixels: &[Color],
    ) -> Vec<u32> {
        let lookup = create_prefix_lookup(palette.as_arrays());
        let lookup = lookup.as_ref();

        let blue = palette
            .as_arrays()
            .iter()
            .map(|&[_, _, b]| b)
            .chain(core::iter::repeat_n(0, RADIX))
            .collect::<Vec<_>>();
        let blue = blue.as_slice();

        pixels
            .as_arrays()
            .into_par_iter()
            .map(|&rgb| find_index(rgb, lookup, blue))
            .collect()
    }

    /// Deduplicate an image into an [`IndexedImage`] in parallel.
    ///
    /// The color type must consist of 3 `u8`s.
    #[must_use]
    pub fn dedup_image_u8_3_par<Color: ColorComponents<u8, 3>>(
        image: ImageRef<'_, Color>,
    ) -> IndexedImage<Color, u32> {
        if image.is_empty() {
            return IndexedImage::default();
        }

        let pixels = image.as_slice();
        let palette = dedup_colors_u8_3_bounded_par(BoundedSlice::new_unchecked(pixels));
        let indices = indices_from_palette_u8_3_par(&palette, pixels);
        IndexedImage::new_unchecked(image.width(), image.height(), palette, indices)
    }

    /// Deduplicate a slice of colors into a [`PaletteCounts`] in parallel.
    ///
    /// The color type must consist of 3 `u8`s.
    pub(crate) fn dedup_colors_u8_3_counts_bounded_par<Color: ColorComponents<u8, 3>>(
        slice: &BoundedSlice<Color>,
    ) -> PaletteCounts<Color> {
        let chunk_size = slice.len().div_ceil(rayon::current_num_threads());
        let red_prefixes = u8_counts(slice.as_arrays(), chunk_size);
        let red_prefix = {
            let mut prefix = [0; RADIX + 1];
            prefix[1..].copy_from_slice(&red_prefixes[red_prefixes.len() - 1][..RADIX]);
            prefix
        };
        let green_blue = sorted_green_blue(slice.as_arrays(), chunk_size, red_prefixes);

        let (colors, counts): (Vec<_>, Vec<_>) = (0..RADIX)
            .into_par_iter()
            .map(|r| {
                let chunk = chunk_range(&red_prefix, r);

                let mut colors = Vec::new();
                let mut counts = Vec::new();

                if !chunk.is_empty() {
                    let green_blue = &green_blue[chunk.clone()];
                    let mut lower_counts = bytemuck::zeroed_box::<[[u32; RADIX]; RADIX]>();

                    if chunk.len() < RADIX * RADIX / 4 {
                        let mut bitmask: BitVec = BitVec::repeat(false, RADIX * RADIX);

                        for gb in green_blue {
                            let [g, b] = gb.map(usize::from);
                            lower_counts[g][b] += 1;
                            bitmask.set(g * RADIX + b, true);
                        }

                        for i in bitmask.iter_ones() {
                            let g = i / RADIX;
                            let b = i % RADIX;
                            #[allow(clippy::cast_possible_truncation)]
                            let color = cast::from_array([r as u8, g as u8, b as u8]);
                            colors.push(color);
                            counts.push(lower_counts[g][b]);
                        }
                    } else {
                        for &[g, b] in green_blue {
                            lower_counts[usize::from(g)][usize::from(b)] += 1;
                        }

                        for (g, count) in lower_counts.iter().enumerate() {
                            for (b, &count) in count.iter().enumerate() {
                                if count > 0 {
                                    #[allow(clippy::cast_possible_truncation)]
                                    let color = cast::from_array([r as u8, g as u8, b as u8]);
                                    colors.push(color);
                                    counts.push(count);
                                }
                            }
                        }
                    }
                }

                (colors, counts)
            })
            .unzip();

        PaletteCounts::new_unchecked(colors.concat(), counts.concat(), slice.length())
    }

    /// Deduplicate a slice of colors into a [`PaletteCounts`] in parallel.
    ///
    /// The color type must consist of 3 `u8`s.
    ///
    /// # Errors
    ///
    /// Returns an error if the length of `slice` is greater than [`MAX_PIXELS`].
    #[inline]
    pub fn dedup_colors_u8_3_counts_par<Color: ColorComponents<u8, 3>>(
        slice: &[Color],
    ) -> Result<PaletteCounts<Color>, LengthOutOfRange> {
        LengthOutOfRange::check_u32(slice, 0, MAX_PIXELS)?;
        Ok(BoundedSlice::new(slice)
            .map(dedup_colors_u8_3_counts_bounded_par)
            .unwrap_or_default())
    }

    /// Deduplicate an image into an [`IndexedImageCounts`] in parallel.
    ///
    /// The color type must consist of 3 `u8`s.
    #[must_use]
    pub fn dedup_image_u8_3_counts_par<Color: ColorComponents<u8, 3>>(
        image: ImageRef<'_, Color>,
    ) -> IndexedImageCounts<Color, u32> {
        if image.is_empty() {
            return IndexedImageCounts::default();
        }

        let pixels = image.as_slice();
        let palette_counts =
            dedup_colors_u8_3_counts_bounded_par(BoundedSlice::new_unchecked(pixels));
        let indices = indices_from_palette_u8_3_par(palette_counts.palette(), pixels);
        IndexedImageCounts::from_palette_counts_unchecked(
            image.width(),
            image.height(),
            palette_counts,
            indices,
        )
    }
}

#[cfg(feature = "threads")]
pub use parallel::*;

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{Image, tests::*};
    use palette::Srgb;

    fn assert_valid_palette_counts(palette_counts: &PaletteCounts<Srgb<u8>>, colors: &[Srgb<u8>]) {
        assert_eq!(palette_counts.total_count() as usize, colors.len());

        let palette = palette_counts.palette();
        for i in 1..palette.len() {
            assert!(palette[i - 1].into_components() < palette[i].into_components());
        }
    }

    fn assert_valid_indexed_counts(image: &IndexedImageCounts<Srgb<u8>, u32>, colors: &[Srgb<u8>]) {
        assert_eq!(image.total_count() as usize, colors.len());

        let palette = image.palette();
        let mut counts = vec![0; palette.len()];
        for (&i, &color) in image.indices().iter().zip(colors) {
            assert_eq!(palette[i as usize], color);
            counts[i as usize] += 1;
        }
        assert_eq!(&counts, image.counts());

        for i in 1..palette.len() {
            assert!(palette[i - 1].into_components() < palette[i].into_components());
        }
    }

    #[allow(clippy::cast_possible_truncation)]
    fn image_1d<T: AsRef<[U]>, U>(container: T) -> Image<U, T> {
        Image::new_unchecked(container.as_ref().len() as u32, 1, container)
    }

    #[allow(clippy::too_many_lines)]
    fn add_duplicate_color_with_data(colors: Vec<Srgb<u8>>) {
        #[allow(clippy::cast_possible_truncation)]
        fn index_of(colors: &[Srgb<u8>], color: Srgb<u8>) -> usize {
            colors.iter().position(|&c| c == color).unwrap()
        }

        let with_duplicate = {
            let mut pixels = colors;
            let len = pixels.len();
            pixels.as_mut_slice()[len - 1] = pixels[0];
            image_1d(pixels)
        };

        let duplicate = with_duplicate.as_slice()[0];
        let without_duplicate =
            image_1d(&with_duplicate.as_slice()[..with_duplicate.as_slice().len() - 1]);

        let expected = dedup_colors_u8_3(without_duplicate.as_slice()).unwrap();
        let actual = dedup_colors_u8_3(with_duplicate.as_slice()).unwrap();
        assert_eq!(actual, expected);

        let expected = {
            let image = dedup_image_u8_3(without_duplicate.as_ref()).into_indexed_image_counts();
            let (width, height) = image.dimensions();
            let (image, mut counts) = image.into_parts();
            let (palette, mut indices) = image.into_parts();
            let i = index_of(&palette, duplicate);
            counts[i] += 1;
            #[allow(clippy::cast_possible_truncation)]
            indices.push(i as u32);
            IndexedImageCounts::new_unchecked(
                width.checked_add(1).unwrap(),
                height,
                palette,
                counts,
                indices,
            )
        };
        let actual = dedup_image_u8_3(with_duplicate.as_ref()).into_indexed_image_counts();
        assert_valid_indexed_counts(&actual, with_duplicate.as_slice());
        assert_eq!(actual, expected);

        let expected = {
            let palette_counts = dedup_colors_u8_3_counts(without_duplicate.as_slice()).unwrap();
            let total_count = palette_counts.total_count();
            let (palette, mut counts) = palette_counts.into_parts();
            let i = index_of(&palette, duplicate);
            counts[i] += 1;
            PaletteCounts::new_unchecked(palette, counts, total_count.checked_add(1).unwrap())
        };
        let actual = dedup_colors_u8_3_counts(with_duplicate.as_slice()).unwrap();
        assert_valid_palette_counts(&actual, with_duplicate.as_slice());
        assert_eq!(actual, expected);

        let expected = {
            let image = dedup_image_u8_3_counts(without_duplicate.as_ref());
            let (width, height) = image.dimensions();
            let (image, mut counts) = image.into_parts();
            let (palette, mut indices) = image.into_parts();
            let i = index_of(&palette, duplicate);
            counts[i] += 1;
            #[allow(clippy::cast_possible_truncation)]
            indices.push(i as u32);
            IndexedImageCounts::new_unchecked(
                width.checked_add(1).unwrap(),
                height,
                palette,
                counts,
                indices,
            )
        };
        let actual = dedup_image_u8_3_counts(with_duplicate.as_ref());
        assert_valid_indexed_counts(&actual, with_duplicate.as_slice());
        assert_eq!(actual, expected);

        #[cfg(feature = "threads")]
        {
            let expected = dedup_colors_u8_3_par(without_duplicate.as_slice()).unwrap();
            let actual = dedup_colors_u8_3_par(with_duplicate.as_slice()).unwrap();
            assert_eq!(actual, expected);

            let expected = {
                let image =
                    dedup_image_u8_3_par(without_duplicate.as_ref()).into_indexed_image_counts();
                let (width, height) = image.dimensions();
                let (image, mut counts) = image.into_parts();
                let (palette, mut indices) = image.into_parts();
                let i = index_of(&palette, duplicate);
                counts[i] += 1;
                #[allow(clippy::cast_possible_truncation)]
                indices.push(i as u32);
                IndexedImageCounts::new_unchecked(
                    width.checked_add(1).unwrap(),
                    height,
                    palette,
                    counts,
                    indices,
                )
            };
            let actual = dedup_image_u8_3_par(with_duplicate.as_ref()).into_indexed_image_counts();
            assert_valid_indexed_counts(&actual, with_duplicate.as_slice());
            assert_eq!(actual, expected);

            let expected = {
                let palette_counts =
                    dedup_colors_u8_3_counts_par(without_duplicate.as_slice()).unwrap();
                let total_count = palette_counts.total_count();
                let (palette, mut counts) = palette_counts.into_parts();
                let i = index_of(&palette, duplicate);
                counts[i] += 1;
                PaletteCounts::new_unchecked(palette, counts, total_count.checked_add(1).unwrap())
            };
            let actual = dedup_colors_u8_3_counts_par(with_duplicate.as_slice()).unwrap();
            assert_valid_palette_counts(&actual, with_duplicate.as_slice());
            assert_eq!(actual, expected);

            let expected = {
                let image = dedup_image_u8_3_counts_par(without_duplicate.as_ref());
                let (width, height) = image.dimensions();
                let (image, mut counts) = image.into_parts();
                let (palette, mut indices) = image.into_parts();
                let i = index_of(&palette, duplicate);
                counts[i] += 1;
                #[allow(clippy::cast_possible_truncation)]
                indices.push(i as u32);
                IndexedImageCounts::new_unchecked(
                    width.checked_add(1).unwrap(),
                    height,
                    palette,
                    counts,
                    indices,
                )
            };
            let actual = dedup_image_u8_3_counts_par(with_duplicate.as_ref());
            assert_valid_indexed_counts(&actual, with_duplicate.as_slice());
            assert_eq!(actual, expected);
        }
    }

    #[test]
    fn add_duplicate_color() {
        let colors = test_data_1024();
        add_duplicate_color_with_data(colors.clone());

        // for testing non-bitvec branches
        let colors = [colors.as_slice(); 256].concat();
        add_duplicate_color_with_data(colors);
    }

    fn reordered_input_with_data(colors: Vec<Srgb<u8>>) {
        let image = image_1d(colors);
        let reordered = {
            let mut reordered = image.clone();
            reordered.as_mut_slice().rotate_right(21);
            reordered
        };

        let expected = dedup_colors_u8_3(image.as_slice()).unwrap();
        let actual = dedup_colors_u8_3(reordered.as_slice()).unwrap();
        assert_eq!(actual, expected);

        let expected = dedup_image_u8_3(image.as_ref()).into_indexed_image_counts();
        let actual = dedup_image_u8_3(reordered.as_ref()).into_indexed_image_counts();
        assert_valid_indexed_counts(&actual, reordered.as_slice());
        assert_eq!(actual.palette(), expected.palette());
        assert_eq!(actual.counts(), expected.counts());
        assert_eq!(actual.indices().len(), expected.indices().len());

        let expected = dedup_colors_u8_3_counts(image.as_slice()).unwrap();
        let actual = dedup_colors_u8_3_counts(reordered.as_slice()).unwrap();
        assert_valid_palette_counts(&actual, reordered.as_slice());
        assert_eq!(actual, expected);

        let expected = dedup_image_u8_3_counts(image.as_ref());
        let actual = dedup_image_u8_3_counts(reordered.as_ref());
        assert_valid_indexed_counts(&actual, reordered.as_slice());
        assert_eq!(actual.palette(), expected.palette());
        assert_eq!(actual.counts(), expected.counts());
        assert_eq!(actual.indices().len(), expected.indices().len());

        #[cfg(feature = "threads")]
        {
            let expected = dedup_colors_u8_3_par(image.as_slice()).unwrap();
            let actual = dedup_colors_u8_3_par(reordered.as_slice()).unwrap();
            assert_eq!(actual, expected);

            let expected = dedup_image_u8_3_par(image.as_ref()).into_indexed_image_counts();
            let actual = dedup_image_u8_3_par(reordered.as_ref()).into_indexed_image_counts();
            assert_valid_indexed_counts(&actual, reordered.as_slice());
            assert_eq!(actual.palette(), expected.palette());
            assert_eq!(actual.counts(), expected.counts());
            assert_eq!(actual.indices().len(), expected.indices().len());

            let expected = dedup_colors_u8_3_counts_par(image.as_slice()).unwrap();
            let actual = dedup_colors_u8_3_counts_par(reordered.as_slice()).unwrap();
            assert_valid_palette_counts(&actual, reordered.as_slice());
            assert_eq!(actual, expected);

            let expected = dedup_image_u8_3_counts_par(image.as_ref());
            let actual = dedup_image_u8_3_counts_par(reordered.as_ref());
            assert_valid_indexed_counts(&actual, reordered.as_slice());
            assert_eq!(actual.palette(), expected.palette());
            assert_eq!(actual.counts(), expected.counts());
            assert_eq!(actual.indices().len(), expected.indices().len());
        }
    }

    #[test]
    fn reordered_input() {
        let colors = test_data_1024();
        reordered_input_with_data(colors.clone());

        // for testing non-bitvec branches
        let colors = [colors.as_slice(); 256].concat();
        reordered_input_with_data(colors);
    }

    #[cfg(feature = "threads")]
    fn single_and_multi_threaded_match_with_data(colors: Vec<Srgb<u8>>) {
        let image = image_1d(colors);

        let single = dedup_colors_u8_3(image.as_slice()).unwrap();
        let par = dedup_colors_u8_3_par(image.as_slice()).unwrap();
        assert_eq!(single, par);

        let single = dedup_image_u8_3(image.as_ref()).into_indexed_image_counts();
        let par = dedup_image_u8_3_par(image.as_ref()).into_indexed_image_counts();
        assert_valid_indexed_counts(&single, image.as_slice());
        assert_eq!(single, par);

        let single = dedup_colors_u8_3_counts(image.as_slice()).unwrap();
        let par = dedup_colors_u8_3_counts_par(image.as_slice()).unwrap();
        assert_valid_palette_counts(&single, image.as_slice());
        assert_eq!(single, par);

        let single = dedup_image_u8_3_counts(image.as_ref());
        let par = dedup_image_u8_3_counts_par(image.as_ref());
        assert_valid_indexed_counts(&single, image.as_slice());
        assert_eq!(single, par);
    }

    #[test]
    #[cfg(feature = "threads")]
    fn single_and_multi_threaded_match() {
        let colors = test_data_1024();
        single_and_multi_threaded_match_with_data(colors.clone());

        // for testing non-bitvec branches
        let colors = [colors.as_slice(); 256].concat();
        single_and_multi_threaded_match_with_data(colors);
    }
}
