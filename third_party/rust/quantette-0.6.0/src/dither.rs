//! Dithering implementations.
//!
//! Currently only Floyd–Steinberg dithering is supported. See the docs for [`FloydSteinberg`] for
//! more information.

use crate::{BoundedIndex, ColorComponents, ImageBuf, ImageRef, IndexedColorMap, IndexedImage};
use alloc::{vec, vec::Vec};
use bytemuck::Zeroable;
use core::array;
use ordered_float::OrderedFloat;
use palette::cast;

/// Floyd–Steinberg dithering.
///
/// Create a [`FloydSteinberg`] using [`new`](Self::new) or
/// [`with_error_diffusion`](Self::with_error_diffusion).
///
/// Both [`Image`](crate::Image)s and [`IndexedImage`]s are supported as inputs and outputs via one of the following:
/// - [`dither_image_to_image`](Self::dither_image_to_image)
/// - [`dither_image_to_indexed`](Self::dither_image_to_indexed)
/// - [`dither_indexed_to_image`](Self::dither_indexed_to_image)
/// - [`dither_indexed_to_indexed`](Self::dither_indexed_to_indexed)
///
/// Parallel versions of the above functions are also available when the `threads` feature is enabled.
///
/// It is recommended to use a perceptually uniform color space/color type like [`Oklab`](palette::Oklab) as input.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct FloydSteinberg(OrderedFloat<f32>);

impl FloydSteinberg {
    /// The default error diffusion factor.
    pub const DEFAULT_ERROR_DIFFUSION: f32 = 7.0 / 8.0;

    /// Create a new [`FloydSteinberg`] with the default error diffusion factor.
    #[must_use]
    #[inline]
    pub const fn new() -> Self {
        Self(OrderedFloat(Self::DEFAULT_ERROR_DIFFUSION))
    }

    /// Create a new [`FloydSteinberg`] with the given error diffusion factor.
    ///
    /// For example, a factor of `1.0` diffuses all of the error to the neighboring pixels.
    ///
    /// This will return `None` if `error_diffusion` is not in the range `(0.0, 1.0]`.
    /// I.e., diffusion must be greater than `0.0` and less than or equal to `1.0`.
    #[must_use]
    #[inline]
    pub const fn with_error_diffusion(error_diffusion: f32) -> Option<Self> {
        if 0.0 < error_diffusion && error_diffusion <= 1.0 {
            Some(Self(OrderedFloat(error_diffusion)))
        } else {
            None
        }
    }

    /// Returns the error diffusion factor of a [`FloydSteinberg`].
    #[inline]
    pub const fn error_diffusion(&self) -> f32 {
        self.0.0
    }
}
impl Default for FloydSteinberg {
    #[inline]
    fn default() -> Self {
        Self::new()
    }
}

/// Calculates `y = ax + y`
#[inline]
fn saxpy<const N: usize>(y: &mut [f32; N], a: f32, x: [f32; N]) {
    for i in 0..N {
        y[i] += a * x[i];
    }
}

/// Calculates `y = ax`
#[inline]
fn sax<const N: usize>(y: &mut [f32; N], a: f32, x: [f32; N]) {
    for i in 0..N {
        y[i] = a * x[i];
    }
}

/// Propagates, stores, and applies the dither error to the pixels.
struct ErrorBuf<'a, const N: usize> {
    /// The width of a row of pixels.
    width: usize,
    /// The propagated error for the current row of pixels.
    this_err: &'a mut [[f32; N]],
    /// The propagated error for the next row of pixels.
    next_err: &'a mut [[f32; N]],
}

impl<'a, const N: usize> ErrorBuf<'a, N> {
    /// Create the backing buffer for a new [`ErrorBuf`].
    fn new_buf(width: usize) -> Vec<[f32; N]> {
        vec![[0.0; N]; 2 * (width + 2)]
    }

    /// Create a new [`ErrorBuf`] using the given `buf`.
    fn new(width: usize, buf: &'a mut [[f32; N]]) -> Self {
        debug_assert_eq!(buf.len(), 2 * (width + 2));
        let (this_err, next_err) = buf.split_at_mut(width + 2);
        Self { width, this_err, next_err }
    }

    /// Propagate error using floyd steinberg dithering, going from left to right.
    #[inline]
    fn propagate_ltr(&mut self, i: usize, err: [f32; N]) {
        saxpy(&mut self.this_err[i + 2], 7.0 / 16.0, err);
        saxpy(&mut self.next_err[i], 3.0 / 16.0, err);
        saxpy(&mut self.next_err[i + 1], 5.0 / 16.0, err);
        sax(&mut self.next_err[i + 2], 1.0 / 16.0, err);
    }

    /// Propagate error using floyd steinberg dithering, going from right to left.
    #[inline]
    fn propagate_rtl(&mut self, i: usize, err: [f32; N]) {
        saxpy(&mut self.this_err[i], 7.0 / 16.0, err);
        saxpy(&mut self.next_err[i + 2], 3.0 / 16.0, err);
        saxpy(&mut self.next_err[i + 1], 5.0 / 16.0, err);
        sax(&mut self.next_err[i], 1.0 / 16.0, err);
    }

    /// Apply the accumulated error to this pixel.
    #[inline]
    fn apply(&self, i: usize, point: &mut [f32; N]) {
        let err = self.this_err[i + 1];
        for i in 0..N {
            point[i] += err[i];
        }
    }

    /// Reset and swap the error buffers for the next row of pixels.
    #[inline]
    fn next_row(&mut self) {
        core::mem::swap(&mut self.this_err, &mut self.next_err);
        self.next_err[1] = [0.0; N];
        self.next_err[self.width] = [0.0; N];
    }
}

/// Dither a single pixel, returning the palette index and error.
#[inline]
fn dither_pixel_indexed_map<Color, const N: usize, ColorMap>(
    i: usize,
    color: Color,
    color_map: ColorMap,
    error: &mut ErrorBuf<'_, N>,
    diffusion: f32,
) -> (u8, [f32; N])
where
    Color: ColorComponents<f32, N>,
    ColorMap: IndexedColorMap<Color>,
{
    let mut color = cast::into_array(color);
    error.apply(i, &mut color);
    let nearest_index = color_map.palette_index(&cast::from_array(color));
    let nearest_color = cast::into_array(color_map.base_palette()[nearest_index]);
    let err = array::from_fn(|i| diffusion * (color[i] - nearest_color[i]));
    (nearest_index, err)
}

/// Dither an image and output palette indices.
#[inline]
fn dither_image_to_indexed<Color, const N: usize, ColorMap>(
    width: usize,
    indices: &mut [u8],
    image: &[Color],
    color_map: ColorMap,
    mut error: ErrorBuf<'_, N>,
    diffusion: f32,
) where
    Color: ColorComponents<f32, N>,
    ColorMap: IndexedColorMap<Color>,
{
    for (row, (indices, colors)) in indices
        .chunks_exact_mut(width)
        .zip(image.chunks_exact(width))
        .enumerate()
    {
        if row % 2 == 0 {
            for (i, (index, &point)) in indices.iter_mut().zip(colors).enumerate() {
                let (idx, err) =
                    dither_pixel_indexed_map(i, point, &color_map, &mut error, diffusion);
                *index = idx;
                error.propagate_ltr(i, err);
            }
        } else {
            for (i, (index, &point)) in indices.iter_mut().zip(colors).enumerate().rev() {
                let (idx, err) =
                    dither_pixel_indexed_map(i, point, &color_map, &mut error, diffusion);
                *index = idx;
                error.propagate_rtl(i, err);
            }
        }

        error.next_row();
    }
}

/// Dither an indexed image and output palette indices.
#[inline]
fn dither_indexed_to_indexed<Color, const N: usize, Index, ColorMap>(
    width: usize,
    indices: &mut [u8],
    original_palette: &[Color],
    original_indices: &[Index],
    color_map: ColorMap,
    mut error: ErrorBuf<'_, N>,
    diffusion: f32,
) where
    Color: ColorComponents<f32, N>,
    Index: BoundedIndex,
    ColorMap: IndexedColorMap<Color>,
{
    let palette = original_palette;
    for (row, (indices, colors)) in indices
        .chunks_exact_mut(width)
        .zip(original_indices.chunks_exact(width))
        .enumerate()
    {
        if row % 2 == 0 {
            for (i, (index, &point)) in indices.iter_mut().zip(colors).enumerate() {
                let (idx, err) = dither_pixel_indexed_map(
                    i,
                    palette[point.as_()],
                    &color_map,
                    &mut error,
                    diffusion,
                );
                *index = idx;
                error.propagate_ltr(i, err);
            }
        } else {
            for (i, (index, &point)) in indices.iter_mut().zip(colors).enumerate().rev() {
                let (idx, err) = dither_pixel_indexed_map(
                    i,
                    palette[point.as_()],
                    &color_map,
                    &mut error,
                    diffusion,
                );
                *index = idx;
                error.propagate_rtl(i, err);
            }
        }

        error.next_row();
    }
}

/// Dither a single pixel, returning the palette color and error.
#[inline]
fn dither_pixel_map<Color, const N: usize, ColorMap>(
    i: usize,
    color: Color,
    color_map: ColorMap,
    error: &mut ErrorBuf<'_, N>,
    diffusion: f32,
) -> (ColorMap::Output, [f32; N])
where
    Color: ColorComponents<f32, N>,
    ColorMap: IndexedColorMap<Color>,
{
    let mut color = cast::into_array(color);
    error.apply(i, &mut color);
    let nearest_index = color_map.palette_index(&cast::from_array(color));
    let nearest_color = cast::into_array(color_map.base_palette()[nearest_index]);
    let output = color_map.palette()[nearest_index].clone();
    let err = array::from_fn(|i| diffusion * (color[i] - nearest_color[i]));
    (output, err)
}

/// Dither an image and output palette colors.
#[inline]
fn dither_image_to_image<Color, const N: usize, ColorMap>(
    width: usize,
    output: &mut [ColorMap::Output],
    original: &[Color],
    color_map: ColorMap,
    mut error: ErrorBuf<'_, N>,
    diffusion: f32,
) where
    Color: ColorComponents<f32, N>,
    ColorMap: IndexedColorMap<Color>,
{
    for (row, (output, colors)) in output
        .chunks_exact_mut(width)
        .zip(original.chunks_exact(width))
        .enumerate()
    {
        if row % 2 == 0 {
            for (i, (output, &point)) in output.iter_mut().zip(colors).enumerate() {
                let (out, err) = dither_pixel_map(i, point, &color_map, &mut error, diffusion);
                *output = out;
                error.propagate_ltr(i, err);
            }
        } else {
            for (i, (output, &point)) in output.iter_mut().zip(colors).enumerate().rev() {
                let (out, err) = dither_pixel_map(i, point, &color_map, &mut error, diffusion);
                *output = out;
                error.propagate_rtl(i, err);
            }
        }

        error.next_row();
    }
}

/// Dither an indexed image and output palette colors.
#[inline]
fn dither_indexed_to_image<Color, const N: usize, Index, ColorMap>(
    width: usize,
    output: &mut [ColorMap::Output],
    palette: &[Color],
    indices: &[Index],
    color_map: ColorMap,
    mut error: ErrorBuf<'_, N>,
    diffusion: f32,
) where
    Color: ColorComponents<f32, N>,
    Index: BoundedIndex,
    ColorMap: IndexedColorMap<Color>,
{
    for (row, (output, colors)) in output
        .chunks_exact_mut(width)
        .zip(indices.chunks_exact(width))
        .enumerate()
    {
        if row % 2 == 0 {
            for (i, (output, &point)) in output.iter_mut().zip(colors).enumerate() {
                let (out, err) =
                    dither_pixel_map(i, palette[point.as_()], &color_map, &mut error, diffusion);
                *output = out;
                error.propagate_ltr(i, err);
            }
        } else {
            for (i, (output, &point)) in output.iter_mut().zip(colors).enumerate().rev() {
                let (out, err) =
                    dither_pixel_map(i, palette[point.as_()], &color_map, &mut error, diffusion);
                *output = out;
                error.propagate_rtl(i, err);
            }
        }

        error.next_row();
    }
}

impl FloydSteinberg {
    /// Dither an [`ImageRef`] to an [`IndexedImage`].
    #[must_use]
    pub fn dither_image_to_indexed<Color, const N: usize, ColorMap>(
        &self,
        image: ImageRef<'_, Color>,
        color_map: ColorMap,
    ) -> IndexedImage<ColorMap::Output>
    where
        Color: ColorComponents<f32, N>,
        ColorMap: IndexedColorMap<Color>,
    {
        if image.is_empty() {
            return IndexedImage::default();
        }
        let diffusion = self.error_diffusion();
        let (width, height) = image.dimensions();
        let w = width as usize;
        let mut indices = bytemuck::zeroed_vec(image.num_pixels() as usize);
        let mut error = ErrorBuf::new_buf(w);
        let error = ErrorBuf::new(w, &mut error);
        dither_image_to_indexed(
            w,
            &mut indices,
            image.as_slice(),
            &color_map,
            error,
            diffusion,
        );
        IndexedImage::new_unchecked(width, height, color_map.into_palette().into_vec(), indices)
    }

    /// Dither an [`IndexedImage`] to a new [`IndexedImage`].
    #[must_use]
    pub fn dither_indexed_to_indexed<Color, const N: usize, Index, ColorMap>(
        &self,
        image: &IndexedImage<Color, Index>,
        color_map: ColorMap,
    ) -> IndexedImage<ColorMap::Output>
    where
        Color: ColorComponents<f32, N>,
        Index: BoundedIndex,
        ColorMap: IndexedColorMap<Color>,
    {
        if image.is_empty() {
            return IndexedImage::default();
        }
        let diffusion = self.error_diffusion();
        let (width, height) = image.dimensions();
        let w = width as usize;
        let mut indices = bytemuck::zeroed_vec(image.num_pixels() as usize);
        let mut error = ErrorBuf::new_buf(w);
        let error = ErrorBuf::new(w, &mut error);
        dither_indexed_to_indexed(
            w,
            &mut indices,
            image.palette(),
            image.indices(),
            &color_map,
            error,
            diffusion,
        );
        IndexedImage::new_unchecked(width, height, color_map.into_palette().into_vec(), indices)
    }

    /// Dither an [`ImageRef`] to a new [`ImageBuf`].
    #[must_use]
    pub fn dither_image_to_image<Color, const N: usize, ColorMap>(
        &self,
        image: ImageRef<'_, Color>,
        color_map: ColorMap,
    ) -> ImageBuf<ColorMap::Output>
    where
        Color: ColorComponents<f32, N>,
        ColorMap: IndexedColorMap<Color>,
        ColorMap::Output: Zeroable,
    {
        if image.is_empty() {
            return ImageBuf::default();
        }
        let diffusion = self.error_diffusion();
        let (width, height) = image.dimensions();
        let w = width as usize;
        let mut pixels = bytemuck::zeroed_vec(image.num_pixels() as usize);
        let mut error = ErrorBuf::new_buf(w);
        let error = ErrorBuf::new(w, &mut error);
        dither_image_to_image(
            w,
            &mut pixels,
            image.as_slice(),
            color_map,
            error,
            diffusion,
        );
        ImageBuf::new_unchecked(width, height, pixels)
    }

    /// Dither an [`IndexedImage`] to a new [`ImageBuf`].
    #[must_use]
    pub fn dither_indexed_to_image<Color, const N: usize, Index, ColorMap>(
        &self,
        image: &IndexedImage<Color, Index>,
        color_map: ColorMap,
    ) -> ImageBuf<ColorMap::Output>
    where
        Color: ColorComponents<f32, N>,
        Index: BoundedIndex,
        ColorMap: IndexedColorMap<Color>,
        ColorMap::Output: Zeroable,
    {
        if image.is_empty() {
            return ImageBuf::default();
        }
        let diffusion = self.error_diffusion();
        let (width, height) = image.dimensions();
        let w = width as usize;
        let mut pixels = bytemuck::zeroed_vec(image.num_pixels() as usize);
        let mut error = ErrorBuf::new_buf(w);
        let error = ErrorBuf::new(w, &mut error);
        dither_indexed_to_image(
            w,
            &mut pixels,
            image.palette(),
            image.indices(),
            color_map,
            error,
            diffusion,
        );
        ImageBuf::new_unchecked(width, height, pixels)
    }
}

#[cfg(feature = "threads")]
mod parallel {
    #[allow(clippy::wildcard_imports)]
    use super::*;
    use crate::{BoundedIndex, ColorComponents, ImageBuf, ImageRef, IndexedColorMap, IndexedImage};
    use bytemuck::Zeroable;
    use rayon::prelude::*;

    impl FloydSteinberg {
        /// Dither an [`ImageRef`] to an [`IndexedImage`] in parallel.
        #[must_use]
        pub fn dither_image_to_indexed_par<Color, const N: usize, ColorMap>(
            &self,
            image: ImageRef<'_, Color>,
            color_map: ColorMap,
        ) -> IndexedImage<ColorMap::Output>
        where
            Color: ColorComponents<f32, N>,
            ColorMap: IndexedColorMap<Color> + Sync,
        {
            if image.is_empty() {
                return IndexedImage::default();
            }
            let diffusion = self.error_diffusion();
            let (width, height) = image.dimensions();
            let w = width as usize;
            let chunk_size = chunk_size(w, height as usize);
            let original = image.as_slice();
            let mut indices = bytemuck::zeroed_vec(image.num_pixels() as usize);

            indices
                .par_chunks_mut(chunk_size)
                .enumerate()
                .for_each(|(chunk_i, indices)| {
                    let chunk_start = chunk_i * chunk_size;

                    let mut error = ErrorBuf::new_buf(w);
                    let mut error = ErrorBuf::new(w, &mut error);

                    if chunk_i > 0 {
                        let colors = &original[(chunk_start - w)..chunk_start];

                        for (i, &og) in colors.iter().enumerate().rev() {
                            let (_, err) =
                                dither_pixel_indexed_map(i, og, &color_map, &mut error, diffusion);
                            error.propagate_rtl(i, err);
                        }

                        error.next_row();
                    }

                    let original_colors = &original
                        [chunk_start..usize::min(chunk_start + chunk_size, original.len())];

                    dither_image_to_indexed(
                        w,
                        indices,
                        original_colors,
                        &color_map,
                        error,
                        diffusion,
                    );
                });

            IndexedImage::new_unchecked(width, height, color_map.into_palette().into_vec(), indices)
        }

        /// Dither an [`IndexedImage`] to a new [`IndexedImage`] in parallel.
        #[must_use]
        pub fn dither_indexed_to_indexed_par<Color, const N: usize, Index, ColorMap>(
            &self,
            image: &IndexedImage<Color, Index>,
            color_map: ColorMap,
        ) -> IndexedImage<ColorMap::Output>
        where
            Color: ColorComponents<f32, N>,
            Index: BoundedIndex,
            ColorMap: IndexedColorMap<Color> + Sync,
        {
            if image.is_empty() {
                return IndexedImage::default();
            }
            let diffusion = self.error_diffusion();
            let (width, height) = image.dimensions();
            let w = width as usize;
            let mut indices = bytemuck::zeroed_vec(image.num_pixels() as usize);
            let chunk_size = chunk_size(w, height as usize);
            let original = image.indices();
            let original_palette = image.palette();

            indices
                .par_chunks_mut(chunk_size)
                .enumerate()
                .for_each(|(chunk_i, indices)| {
                    let chunk_start = chunk_i * chunk_size;

                    let mut error = ErrorBuf::new_buf(w);
                    let mut error = ErrorBuf::new(w, &mut error);

                    if chunk_i > 0 {
                        let colors = &original[(chunk_start - w)..chunk_start];

                        for (i, &og) in colors.iter().enumerate().rev() {
                            let (_, err) = dither_pixel_indexed_map(
                                i,
                                original_palette[og.as_()],
                                &color_map,
                                &mut error,
                                diffusion,
                            );
                            error.propagate_rtl(i, err);
                        }

                        error.next_row();
                    }

                    let original_indices = &original
                        [chunk_start..usize::min(chunk_start + chunk_size, original.len())];

                    dither_indexed_to_indexed(
                        w,
                        indices,
                        original_palette,
                        original_indices,
                        &color_map,
                        error,
                        diffusion,
                    );
                });

            IndexedImage::new_unchecked(width, height, color_map.into_palette().into_vec(), indices)
        }

        /// Dither an [`ImageRef`] to a new [`ImageBuf`] in parallel.
        #[must_use]
        pub fn dither_image_to_image_par<Color, const N: usize, ColorMap>(
            &self,
            image: ImageRef<'_, Color>,
            color_map: ColorMap,
        ) -> ImageBuf<ColorMap::Output>
        where
            Color: ColorComponents<f32, N>,
            ColorMap: IndexedColorMap<Color> + Sync,
            ColorMap::Output: Zeroable,
        {
            if image.is_empty() {
                return ImageBuf::default();
            }
            let diffusion = self.error_diffusion();
            let (width, height) = image.dimensions();
            let w = width as usize;
            let chunk_size = chunk_size(w, height as usize);
            let original = image.as_slice();
            let mut pixels = bytemuck::zeroed_vec(original.len());

            pixels
                .par_chunks_mut(chunk_size)
                .enumerate()
                .for_each(|(chunk_i, output)| {
                    let chunk_start = chunk_i * chunk_size;

                    let mut error = ErrorBuf::new_buf(w);
                    let mut error = ErrorBuf::new(w, &mut error);

                    if chunk_i > 0 {
                        let colors = &original[(chunk_start - w)..chunk_start];

                        for (i, &og) in colors.iter().enumerate().rev() {
                            let (_, err) =
                                dither_pixel_map(i, og, &color_map, &mut error, diffusion);
                            error.propagate_rtl(i, err);
                        }

                        error.next_row();
                    }

                    let original_colors = &original
                        [chunk_start..usize::min(chunk_start + chunk_size, original.len())];

                    dither_image_to_image(w, output, original_colors, &color_map, error, diffusion);
                });

            ImageBuf::new_unchecked(width, height, pixels)
        }

        /// Dither an [`IndexedImage`] to a new [`ImageBuf`] in parallel.
        #[must_use]
        pub fn dither_indexed_to_image_par<Color, const N: usize, Index, ColorMap>(
            &self,
            image: &IndexedImage<Color, Index>,
            color_map: ColorMap,
        ) -> ImageBuf<ColorMap::Output>
        where
            Color: ColorComponents<f32, N>,
            Index: BoundedIndex,
            ColorMap: IndexedColorMap<Color> + Sync,
            ColorMap::Output: Zeroable,
        {
            if image.is_empty() {
                return ImageBuf::default();
            }
            let diffusion = self.error_diffusion();
            let (width, height) = image.dimensions();
            let w = width as usize;
            let chunk_size = chunk_size(w, height as usize);
            let mut pixels = bytemuck::zeroed_vec(image.num_pixels() as usize);
            let indices = image.indices();
            let palette = image.palette();

            pixels
                .par_chunks_mut(chunk_size)
                .enumerate()
                .for_each(|(chunk_i, output)| {
                    let chunk_start = chunk_i * chunk_size;

                    let mut error = ErrorBuf::new_buf(w);
                    let mut error = ErrorBuf::new(w, &mut error);

                    if chunk_i > 0 {
                        let colors = &indices[(chunk_start - w)..chunk_start];

                        for (i, &og) in colors.iter().enumerate().rev() {
                            let (_, err) = dither_pixel_map(
                                i,
                                palette[og.as_()],
                                &color_map,
                                &mut error,
                                diffusion,
                            );
                            error.propagate_rtl(i, err);
                        }

                        error.next_row();
                    }

                    let original_indices =
                        &indices[chunk_start..usize::min(chunk_start + chunk_size, indices.len())];

                    dither_indexed_to_image(
                        w,
                        output,
                        palette,
                        original_indices,
                        &color_map,
                        error,
                        diffusion,
                    );
                });

            ImageBuf::new_unchecked(width, height, pixels)
        }
    }

    /// Returns the chunk size for each thread based on the image dimensions.
    fn chunk_size(width: usize, height: usize) -> usize {
        let num_chunks = usize::min(rayon::current_num_threads(), height.div_ceil(256));
        let rows_per_chunk = height.div_ceil(num_chunks);
        width * rows_per_chunk
    }
}

#[cfg(feature = "threads")]
#[allow(unused_imports)]
pub use parallel::*;

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{
        PaletteBuf, color_map::NearestNeighborColorMap, color_space::srgb8_to_oklab, tests::*,
    };
    use palette::Oklab;

    #[test]
    fn empty_inputs() {
        let ditherer = FloydSteinberg::new();
        let image = ImageRef::default();
        let indexed = IndexedImage::<Oklab>::default();
        let color_map = NearestNeighborColorMap::new(
            PaletteBuf::new(vec![Oklab { l: 0.0, a: 0.0, b: 0.0 }]).unwrap(),
        );

        assert!(ditherer.dither_image_to_image(image, &color_map).is_empty());
        assert!(
            ditherer
                .dither_image_to_indexed(image, &color_map)
                .is_empty()
        );
        assert!(
            ditherer
                .dither_indexed_to_image(&indexed, &color_map)
                .is_empty()
        );
        assert!(
            ditherer
                .dither_indexed_to_indexed(&indexed, &color_map)
                .is_empty()
        );

        #[cfg(feature = "threads")]
        {
            assert!(
                ditherer
                    .dither_image_to_image_par(image, &color_map)
                    .is_empty()
            );
            assert!(
                ditherer
                    .dither_image_to_indexed_par(image, &color_map)
                    .is_empty()
            );
            assert!(
                ditherer
                    .dither_indexed_to_image_par(&indexed, &color_map)
                    .is_empty()
            );
            assert!(
                ditherer
                    .dither_indexed_to_indexed_par(&indexed, &color_map)
                    .is_empty()
            );
        }
    }

    #[test]
    fn exact_match_image_unaffected() {
        let ditherer = FloydSteinberg::new();

        let palette = PaletteBuf::from_mapping(&test_data_256(), srgb8_to_oklab);
        let color_map = NearestNeighborColorMap::new(palette.clone());
        let indices = {
            #[allow(clippy::cast_possible_truncation)]
            let indices = (0..palette.len()).map(|i| i as u8).collect::<Vec<_>>();
            let mut indices = [indices.as_slice(); 4].concat();
            indices.rotate_right(7);
            indices
        };
        let indexed = IndexedImage::new(32, 32, palette.into_vec(), indices).unwrap();
        let image = indexed.to_image();

        assert_eq!(
            &ditherer.dither_image_to_image(image.as_ref(), &color_map),
            &image
        );
        assert_eq!(
            &ditherer.dither_image_to_indexed(image.as_ref(), &color_map),
            &indexed
        );
        assert_eq!(
            &ditherer.dither_indexed_to_image(&indexed, &color_map),
            &image
        );
        assert_eq!(
            &ditherer.dither_indexed_to_indexed(&indexed, &color_map),
            &indexed
        );

        #[cfg(feature = "threads")]
        {
            assert_eq!(
                &ditherer.dither_image_to_image_par(image.as_ref(), &color_map),
                &image
            );
            assert_eq!(
                &ditherer.dither_image_to_indexed_par(image.as_ref(), &color_map),
                &indexed
            );
            assert_eq!(
                &ditherer.dither_indexed_to_image_par(&indexed, &color_map),
                &image
            );
            assert_eq!(
                &ditherer.dither_indexed_to_indexed_par(&indexed, &color_map),
                &indexed
            );
        }
    }
}
