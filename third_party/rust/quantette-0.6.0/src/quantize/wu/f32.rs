use super::shared::{Histogram3, Stats, Wu, sum_of_squares};
use crate::{
    BoundedIndex, BoundedSlice, ColorComponents, ImageRef, IndexedColorMap, IndexedImageCounts,
    LengthOutOfRange, Palette, PaletteBuf, PaletteCounts, PaletteSize,
};
use alloc::{vec, vec::Vec};
use core::{array, marker::PhantomData};
use palette::cast::{self, AsArrays as _};
use wide::{f32x8, i32x8, u32x8};

/// A histogram binner for colors consisting of 3 `f32` components.
///
/// `B1`, `B2`, and `B3` specify the number of bins to have in each dimension.
/// It is recommended to weight the number of bins based on the importance of each dimension.
/// Also consider using powers of two which may slightly speed up index calculations.
#[derive(Debug, Clone, Copy)]
pub struct BinnerF32x3<const B1: usize, const B2: usize, const B3: usize> {
    /// The offsets/minimums for values in each dimension.
    mins: [f32; 3],
    /// The multipliers used to make each value less than the number of bins.
    scale: [f32; 3],
}
impl<const B1: usize, const B2: usize, const B3: usize> BinnerF32x3<B1, B2, B3> {
    /// Create a new [`BinnerF32x3`] from the given ranges of values for each component.
    ///
    /// Each range should be of the form `(min_value, max_value)`.
    #[must_use]
    pub const fn new(ranges: [(f32, f32); 3]) -> Self {
        const {
            assert!(1 <= B1 && B1 <= u8::MAX as usize);
            assert!(1 <= B2 && B2 <= u8::MAX as usize);
            assert!(1 <= B3 && B3 <= u8::MAX as usize);
        }
        let [(l1, u1), (l2, u2), (l3, u3)] = ranges;
        #[allow(clippy::cast_precision_loss)]
        Self {
            mins: [l1, l2, l3],
            scale: [
                B1 as f32 / (u1 - l1),
                B2 as f32 / (u2 - l2),
                B3 as f32 / (u3 - l3),
            ],
        }
    }
}

impl<const B1: usize, const B2: usize, const B3: usize> BinnerF32x3<B1, B2, B3> {
    /// The range of possible values for `f32` components of an [`Oklab`](palette::Oklab) color,
    /// provided that it was converted from a [`Srgb<u8>`](palette::Srgb) color.
    pub const OKLAB_COMPONENT_RANGES_FROM_SRGB8: [(f32, f32); 3] = [
        (0.0, 1.0),
        (-0.2338874, 0.2762164),
        (-0.31152815, 0.19856972),
    ];
}

impl BinnerF32x3<32, 16, 16> {
    /// Returns the default binner used for [`Oklab`](palette::Oklab) colors that were converted
    /// from [`Srgb<u8>`](palette::Srgb) colors.
    #[must_use]
    #[inline]
    pub const fn oklab_from_srgb8() -> Self {
        BinnerF32x3::new(Self::OKLAB_COMPONENT_RANGES_FROM_SRGB8)
    }
}

impl<const B1: usize, const B2: usize, const B3: usize> BinnerF32x3<B1, B2, B3> {
    /// Returns the histogram bins for each color component.
    #[inline]
    fn bin(&self, components: [f32; 3]) -> [u8; 3] {
        let Self { mins, scale } = self;
        let max_bins = [B1, B2, B3];
        let mut bin = [0; 3];
        #[allow(clippy::cast_possible_truncation, clippy::cast_sign_loss)]
        for c in 0..3 {
            bin[c] = (((components[c] - mins[c]) * scale[c]) as u8).min(max_bins[c] as u8 - 1);
        }
        bin
    }

    /// Returns the flattened histogram bin index for a color.
    #[allow(clippy::cast_possible_truncation)]
    #[inline]
    fn index(&self, components: [f32; 3]) -> u32 {
        let [b1, b2, b3] = self.bin(components).map(u32::from);
        b1 * B2 as u32 * B3 as u32 + b2 * B3 as u32 + b3
    }

    /// Returns the flattened histogram bin indices for 8 colors at a time.
    #[inline]
    fn index8(&self, components: [f32x8; 3]) -> u32x8 {
        let Self { mins, scale } = self;
        #[allow(clippy::cast_possible_truncation)]
        let max_bins = [B1, B2, B3].map(|n| u32x8::splat((n - 1) as u32));

        let [b1, b2, b3] = array::from_fn(|i| {
            let bins = (components[i] - mins[i]) * scale[i];
            let bins = bins.trunc_int().max(i32x8::ZERO);
            let bins: u32x8 = bytemuck::cast(bins);
            bins.min(max_bins[i])
        });

        #[allow(clippy::cast_possible_truncation)]
        {
            b1 * u32x8::splat(B2 as u32 * B3 as u32) + b2 * u32x8::splat(B3 as u32) + b3
        }
    }
}

impl<const B1: usize, const B2: usize, const B3: usize> Histogram3<Stats<f64, 3>, B1, B2, B3> {
    /// Add the given color to the stats for a histogram bin.
    #[inline]
    fn add_color_f32_3(stats: &mut Stats<f64, 3>, color: [f32; 3]) {
        let Stats { count, components, sum_squared } = stats;
        let color = color.map(f64::from);

        *count += 1;
        for (c, v) in components.iter_mut().zip(color) {
            *c += v;
        }
        *sum_squared += sum_of_squares(color);
    }

    /// Add the given colors to the histogram.
    fn add_colors_f32_3(&mut self, colors: &[[f32; 3]], binner: &BinnerF32x3<B1, B2, B3>) {
        let hist = self.as_flattened_mut();
        let (chunks, remainder) = colors.as_chunks::<8>();
        for &colors in chunks {
            let components = array::from_fn(|i| f32x8::new(colors.map(|c| c[i])));
            let bins = binner.index8(components);

            for (color, bin) in colors.into_iter().zip(bins.to_array()) {
                Self::add_color_f32_3(&mut hist[bin as usize], color);
            }
        }
        for &color in remainder {
            let bin = binner.index(color);
            Self::add_color_f32_3(&mut hist[bin as usize], color);
        }
    }

    /// Add the given color and count to the stats for a histogram bin.
    #[inline]
    fn add_color_count_f32_3(stats: &mut Stats<f64, 3>, color: [f32; 3], n: u32) {
        let Stats { count, components, sum_squared } = stats;
        let color = color.map(f64::from);

        *count += n;
        let w = f64::from(n);
        for (c, v) in components.iter_mut().zip(color) {
            *c += w * v;
        }
        *sum_squared += w * sum_of_squares(color);
    }

    /// Add the given colors and their counts to the histogram.
    fn add_color_counts_f32_3(
        &mut self,
        colors: &[[f32; 3]],
        counts: &[u32],
        binner: &BinnerF32x3<B1, B2, B3>,
    ) {
        let hist = self.as_flattened_mut();
        let (chunks, remainder) = colors.as_chunks::<8>();
        let (counts_chunks, counts_remainder) = counts.as_chunks::<8>();
        for (&colors, &counts) in chunks.iter().zip(counts_chunks) {
            let components: [_; 3] = array::from_fn(|i| f32x8::new(colors.map(|c| c[i])));
            let bins = binner.index8(components);

            for ((color, n), bin) in colors.into_iter().zip(counts).zip(bins.to_array()) {
                Self::add_color_count_f32_3(&mut hist[bin as usize], color, n);
            }
        }
        for (&color, &n) in remainder.iter().zip(counts_remainder) {
            let bin = binner.index(color);
            Self::add_color_count_f32_3(&mut hist[bin as usize], color, n);
        }
    }
}

/// The struct for Wu's color quantization method for colors with `f32` components in 3 dimensions.
///
/// See the [module](crate::wu) documentation for more information.
///
/// Slices, [`ImageRef`], [`PaletteCounts`], and [`IndexedImageCounts`] are supported as inputs via:
/// - [`run_slice`](Self::run_slice)
/// - [`run_image`](Self::run_image)
/// - [`run_palette_counts`](Self::run_palette_counts)
/// - [`run_indexed_image_counts`](Self::run_indexed_image_counts)
///
/// Parallel versions are also available if the `threads` feature is enabled.
///
/// To produce the final output, use one of the following:
/// - [`palette`](Self::palette)
/// - [`palette_and_counts`](Self::palette_and_counts)
/// - [`color_map`](Self::color_map)
/// - [`color_map_and_counts`](Self::color_map_and_counts)
///
/// Note that these functions take a reference to `self`, and so can be called multiple times on
/// the same [`WuF32x3`] with different [`PaletteSize`]s.
///
/// # Examples
///
/// Minimal example:
/// ```
/// use quantette::{PaletteSize, wu::{WuF32x3, BinnerF32x3}};
/// use palette::Oklab;
///
/// let input = vec![Oklab::new(0.0, 0.0, 0.0)];
/// let binner = BinnerF32x3::oklab_from_srgb8();
/// let palette = WuF32x3::run_slice(&input, binner).unwrap().palette(PaletteSize::MAX);
/// assert_eq!(palette.len(), input.len());
/// ```
///
/// Full image quantization example:
/// ```
/// # fn main() -> Result<(), Box<dyn std::error::Error>> {
/// use quantette::{
///     wu::{BinnerF32x3, WuF32x3},
///     ImageBuf, PaletteSize,
/// };
/// use palette::Oklab;
///
/// let image = ImageBuf::new(1, 1, vec![Oklab::new(0.0, 0.0, 0.0)])?;
/// let binner = BinnerF32x3::oklab_from_srgb8();
/// let color_map = WuF32x3::run_image(image.as_ref(), binner).unwrap().color_map(PaletteSize::MAX);
/// let quantized = image.map_to_image(&color_map);
/// assert_eq!(image.dimensions(), quantized.dimensions());
/// # Ok(())
/// # }
/// ```
#[must_use]
pub struct WuF32x3<Color, const B1: usize, const B2: usize, const B3: usize> {
    /// The color type must remain the same for each [`WuF32x3`].
    color: PhantomData<Color>,
    /// The histogram binner.
    binner: BinnerF32x3<B1, B2, B3>,
    /// The histogram data.
    hist: Histogram3<Stats<f64, 3>, B1, B2, B3>,
}

impl<Color: ColorComponents<f32, 3>, const B1: usize, const B2: usize, const B3: usize>
    WuF32x3<Color, B1, B2, B3>
{
    pub(crate) fn run_slice_bounded(
        colors: &BoundedSlice<Color>,
        binner: BinnerF32x3<B1, B2, B3>,
    ) -> Self {
        let mut hist = Histogram3::new();
        hist.add_colors_f32_3(colors.as_arrays(), &binner);
        hist.calc_cumulative_moments();
        Self { color: PhantomData, binner, hist }
    }

    /// Run Wu's quantization method on a slice of colors.
    ///
    /// # Errors
    ///
    /// Returns an error if `colors` is empty or longer than [`MAX_PIXELS`](crate::MAX_PIXELS).
    pub fn run_slice(
        colors: &[Color],
        binner: BinnerF32x3<B1, B2, B3>,
    ) -> Result<Self, LengthOutOfRange> {
        let colors = BoundedSlice::new(colors)?;
        Ok(Self::run_slice_bounded(colors, binner))
    }

    /// Run Wu's quantization method on an [`ImageRef`].
    ///
    /// Returns `None` if the image is empty.
    #[must_use]
    pub fn run_image(image: ImageRef<'_, Color>, binner: BinnerF32x3<B1, B2, B3>) -> Option<Self> {
        let pixels = BoundedSlice::new(image.as_slice()).ok()?;
        Some(Self::run_slice_bounded(pixels, binner))
    }

    fn run_palette_and_counts(
        palette: &[Color],
        counts: &[u32],
        total_count: u32,
        binner: BinnerF32x3<B1, B2, B3>,
    ) -> Option<Self> {
        (total_count != 0).then(|| {
            let mut hist = Histogram3::new();
            hist.add_color_counts_f32_3(palette.as_arrays(), counts, &binner);
            hist.calc_cumulative_moments();
            Self { color: PhantomData, binner, hist }
        })
    }

    /// Run Wu's quantization method on an [`PaletteCounts`].
    ///
    /// Returns `None` if the [`PaletteCounts`] is empty.
    #[must_use]
    pub fn run_palette_counts(
        palette_counts: &PaletteCounts<Color>,
        binner: BinnerF32x3<B1, B2, B3>,
    ) -> Option<Self> {
        Self::run_palette_and_counts(
            palette_counts.palette(),
            palette_counts.counts(),
            palette_counts.total_count(),
            binner,
        )
    }

    /// Run Wu's quantization method on an [`IndexedImageCounts`].
    ///
    /// Returns `None` if the [`IndexedImageCounts`] is empty.
    #[must_use]
    pub fn run_indexed_image_counts<Index: BoundedIndex>(
        image: &IndexedImageCounts<Color, Index>,
        binner: BinnerF32x3<B1, B2, B3>,
    ) -> Option<Self> {
        Self::run_palette_and_counts(image.palette(), image.counts(), image.total_count(), binner)
    }

    /// Compute the color palette.
    #[must_use]
    pub fn palette(&self, k: PaletteSize) -> PaletteBuf<Color> {
        Wu::<_, Color, _, _>::new(&self.hist).palette(k)
    }

    /// Compute the color palette and the number of pixels assigned to each palette color.
    #[must_use]
    pub fn palette_and_counts(&self, k: PaletteSize) -> (PaletteBuf<Color>, PaletteBuf<u32>) {
        Wu::<_, Color, _, _>::new(&self.hist).palette_and_counts(k)
    }

    /// Compute the color map and the number of pixels assigned to each palette color.
    #[must_use]
    pub fn color_map_and_counts(
        &self,
        k: PaletteSize,
    ) -> (WuF32x3ColorMap<Color, B1, B2, B3>, PaletteBuf<u32>) {
        let (palette, counts, cubes) =
            Wu::<_, Color, _, _>::new(&self.hist).palette_counts_and_cubes(k);
        let hist = Histogram3::from_cubes(cubes);
        let color_map = WuF32x3ColorMap { palette, binner: self.binner, hist };
        (color_map, counts)
    }

    /// Compute the color map.
    #[must_use]
    pub fn color_map(&self, k: PaletteSize) -> WuF32x3ColorMap<Color, B1, B2, B3> {
        let (palette, cubes) = Wu::<_, Color, _, _>::new(&self.hist).palette_and_cubes(k);
        let hist = Histogram3::from_cubes(cubes);
        WuF32x3ColorMap { palette, binner: self.binner, hist }
    }
}

/// An [`IndexedColorMap`] that maps colors to histograms bins and the palette index or color
/// associated with the histogram bin.
///
/// Can only be created from [`WuF32x3::color_map`] or [`WuF32x3::color_map_and_counts`].
#[derive(Clone, Debug)]
pub struct WuF32x3ColorMap<Color, const B1: usize, const B2: usize, const B3: usize> {
    /// The color palette.
    palette: PaletteBuf<Color>,
    /// The histogram binner.
    binner: BinnerF32x3<B1, B2, B3>,
    /// The histogram containing palette indices.
    hist: Histogram3<u8, B1, B2, B3>,
}

impl<Color, const B1: usize, const B2: usize, const B3: usize> WuF32x3ColorMap<Color, B1, B2, B3> {
    /// Consume a [`WuF32x3ColorMap`] and return the underlying [`PaletteBuf`].
    #[must_use]
    #[inline]
    pub fn into_palette(self) -> PaletteBuf<Color> {
        self.palette
    }

    /// Returns the [`Palette`] of colors of a [`WuF32x3ColorMap`].
    #[inline]
    pub fn palette(&self) -> &Palette<Color> {
        &self.palette
    }
}

impl<Color: ColorComponents<f32, 3>, const B1: usize, const B2: usize, const B3: usize>
    WuF32x3ColorMap<Color, B1, B2, B3>
{
    /// Mutate a color slice by mapping each color to its corresponding palette color.
    #[inline]
    pub fn map_slice_in_place(&self, colors: &mut [Color]) {
        for color in colors {
            *color = self.palette_color(color);
        }
    }
}

impl<Color: ColorComponents<f32, 3>, const B1: usize, const B2: usize, const B3: usize>
    IndexedColorMap<Color> for WuF32x3ColorMap<Color, B1, B2, B3>
{
    type Output = Color;

    #[inline]
    fn into_palette(self) -> PaletteBuf<Self::Output> {
        self.into_palette()
    }

    #[inline]
    fn palette(&self) -> &Palette<Self::Output> {
        self.palette()
    }

    #[inline]
    fn base_palette(&self) -> &Palette<Color> {
        self.palette()
    }

    #[inline]
    fn palette_index(&self, color: &Color) -> u8 {
        let Self { hist, binner, .. } = self;
        (*hist)[binner.bin(cast::into_array(*color))]
    }

    fn map_to_indices(&self, input: &[Color]) -> Vec<u8> {
        let Self { binner, hist, .. } = self;
        let flat_hist = hist.as_flattened();
        let input = input.as_arrays();
        let mut output = vec![0; input.len()];
        let (out_chunks, out_remainder) = output.as_chunks_mut::<8>();
        let (in_chunks, in_remainder) = input.as_chunks::<8>();
        for (output, colors) in out_chunks.iter_mut().zip(in_chunks) {
            let components = array::from_fn(|i| f32x8::new(colors.map(|c| c[i])));
            let bins = binner.index8(components);
            for (output, bin) in output.iter_mut().zip(bins.to_array()) {
                *output = flat_hist[bin as usize];
            }
        }
        for (output, &color) in out_remainder.iter_mut().zip(in_remainder) {
            let bin = binner.bin(color);
            *output = hist[bin];
        }

        output
    }

    fn map_to_colors_of_palette<Output: Clone + Send + Sync>(
        &self,
        palette: &Palette<Output>,
        input: &[Color],
    ) -> Vec<Output> {
        let Self { binner, hist, .. } = self;
        let flat_hist = hist.as_flattened();
        let input = input.as_arrays();
        let mut output = Vec::with_capacity(input.len());
        let (chunks, remainder) = input.as_chunks::<8>();
        for colors in chunks {
            let components = array::from_fn(|i| f32x8::new(colors.map(|c| c[i])));
            let bins = binner.index8(components);
            output.extend(
                bins.to_array()
                    .into_iter()
                    .map(|bin| palette[flat_hist[bin as usize]].clone()),
            );
        }
        output.extend(
            remainder
                .iter()
                .map(|&color| palette[hist[binner.bin(color)]].clone()),
        );

        output
    }
}

#[cfg(feature = "threads")]
mod parallel {
    use super::{super::shared::Histogram3, BinnerF32x3, WuF32x3, WuF32x3ColorMap};
    use crate::{
        BoundedIndex, BoundedSlice, ColorComponents, ImageRef, IndexedColorMap, IndexedImageCounts,
        LengthOutOfRange, Palette, PaletteBuf, PaletteCounts, PaletteSize,
    };
    use core::{array, marker::PhantomData};
    use palette::cast::AsArrays as _;
    use rayon::prelude::*;
    use wide::f32x8;

    impl<Color: ColorComponents<f32, 3>, const B1: usize, const B2: usize, const B3: usize>
        WuF32x3<Color, B1, B2, B3>
    {
        /// Returns the per thread chunk size based on the length.
        fn chunk_size(num_bins: usize, len: usize) -> usize {
            let chunk_size = len.div_ceil(rayon::current_num_threads()).max(num_bins * 4);
            let num_chunks = len.div_ceil(chunk_size);
            len.div_ceil(num_chunks)
        }

        pub(crate) fn run_slice_bounded_par(
            colors: &[Color],
            binner: BinnerF32x3<B1, B2, B3>,
        ) -> Self {
            let chunk_size = Self::chunk_size(B1 * B2 * B3, colors.len());
            let mut hist = colors
                .as_arrays()
                .par_chunks(chunk_size)
                .map(|colors| {
                    let mut hist = Histogram3::new();
                    hist.add_colors_f32_3(colors, &binner);
                    hist
                })
                .reduce_with(Histogram3::merge_partial)
                .unwrap_or_else(Histogram3::new);

            hist.calc_cumulative_moments();

            Self { color: PhantomData, binner, hist }
        }

        /// Run Wu's quantization method on a slice of colors in parallel.
        ///
        /// # Errors
        ///
        /// Returns an error if `colors` is empty or longer than [`MAX_PIXELS`](crate::MAX_PIXELS).
        pub fn run_slice_par(
            colors: &[Color],
            binner: BinnerF32x3<B1, B2, B3>,
        ) -> Result<Self, LengthOutOfRange> {
            let colors = BoundedSlice::new(colors)?;
            Ok(Self::run_slice_bounded_par(colors, binner))
        }

        /// Run Wu's quantization method on an [`ImageRef`] in parallel.
        ///
        /// Returns `None` if the image is empty.
        #[must_use]
        pub fn run_image_par(
            image: ImageRef<'_, Color>,
            binner: BinnerF32x3<B1, B2, B3>,
        ) -> Option<Self> {
            let pixels = BoundedSlice::new(image.as_slice()).ok()?;
            Some(Self::run_slice_bounded_par(pixels, binner))
        }

        fn run_palette_and_counts_par(
            palette: &[Color],
            counts: &[u32],
            total_count: u32,
            binner: BinnerF32x3<B1, B2, B3>,
        ) -> Option<Self> {
            (total_count != 0).then(|| {
                let chunk_size = Self::chunk_size(B1 * B2 * B3, palette.len());
                let mut hist = palette
                    .as_arrays()
                    .par_chunks(chunk_size)
                    .zip(counts.par_chunks(chunk_size))
                    .map(|(colors, counts)| {
                        let mut hist = Histogram3::new();
                        hist.add_color_counts_f32_3(colors, counts, &binner);
                        hist
                    })
                    .reduce_with(Histogram3::merge_partial)
                    .unwrap_or_else(Histogram3::new);

                hist.calc_cumulative_moments();

                Self { color: PhantomData, binner, hist }
            })
        }

        /// Run Wu's quantization method on an [`PaletteCounts`] in parallel.
        ///
        /// Returns `None` if the [`PaletteCounts`] is empty.
        #[must_use]
        pub fn run_palette_counts_par(
            palette_counts: &PaletteCounts<Color>,
            binner: BinnerF32x3<B1, B2, B3>,
        ) -> Option<Self> {
            Self::run_palette_and_counts_par(
                palette_counts.palette(),
                palette_counts.counts(),
                palette_counts.total_count(),
                binner,
            )
        }

        /// Run Wu's quantization method on an [`IndexedImageCounts`] in parallel.
        ///
        /// Returns `None` if the [`IndexedImageCounts`] is empty.
        #[must_use]
        pub fn run_indexed_image_counts_par<Index: BoundedIndex>(
            image: &IndexedImageCounts<Color, Index>,
            binner: BinnerF32x3<B1, B2, B3>,
        ) -> Option<Self> {
            Self::run_palette_and_counts_par(
                image.palette(),
                image.counts(),
                image.total_count(),
                binner,
            )
        }
    }

    impl<Color: ColorComponents<f32, 3>, const B1: usize, const B2: usize, const B3: usize>
        WuF32x3<Color, B1, B2, B3>
    {
        #[must_use]
        #[inline]
        /// Compute the parallel color map and the number of pixels assigned to each
        /// palette color.
        pub fn parallel_color_map_and_counts(
            &self,
            k: PaletteSize,
        ) -> (WuF32x3ParallelColorMap<Color, B1, B2, B3>, PaletteBuf<u32>) {
            let (color_map, counts) = self.color_map_and_counts(k);
            (WuF32x3ParallelColorMap(color_map), counts)
        }

        #[must_use]
        #[inline]
        /// Compute the parallel color map.
        pub fn parallel_color_map(
            &self,
            k: PaletteSize,
        ) -> WuF32x3ParallelColorMap<Color, B1, B2, B3> {
            WuF32x3ParallelColorMap(self.color_map(k))
        }
    }

    /// An [`IndexedColorMap`] that, in parallel, maps colors to histograms bins and the
    /// palette index or color associated with the histogram bin.
    ///
    /// Can only be created from [`WuF32x3::parallel_color_map`] or [`WuF32x3::parallel_color_map_and_counts`].
    #[derive(Clone, Debug)]
    pub struct WuF32x3ParallelColorMap<Color, const B1: usize, const B2: usize, const B3: usize>(
        WuF32x3ColorMap<Color, B1, B2, B3>,
    );

    impl<Color, const B1: usize, const B2: usize, const B3: usize>
        WuF32x3ParallelColorMap<Color, B1, B2, B3>
    {
        /// Consume a [`WuF32x3ParallelColorMap`] and return the underlying [`PaletteBuf`].
        #[must_use]
        #[inline]
        pub fn into_palette(self) -> PaletteBuf<Color> {
            self.0.into_palette()
        }

        /// Returns the [`Palette`] of colors of a [`WuF32x3ParallelColorMap`].
        #[inline]
        pub fn palette(&self) -> &Palette<Color> {
            self.0.palette()
        }

        /// Convert a [`WuF32x3ParallelColorMap`] to a [`WuF32x3ColorMap`].
        #[must_use]
        #[inline]
        pub fn into_serial(self) -> WuF32x3ColorMap<Color, B1, B2, B3> {
            self.0
        }
    }

    impl<Color, const B1: usize, const B2: usize, const B3: usize> WuF32x3ColorMap<Color, B1, B2, B3> {
        /// Convert a [`WuF32x3ColorMap`] to a [`WuF32x3ParallelColorMap`].
        #[must_use]
        #[inline]
        pub fn into_parallel(self) -> WuF32x3ParallelColorMap<Color, B1, B2, B3> {
            WuF32x3ParallelColorMap(self)
        }
    }

    impl<Color, const B1: usize, const B2: usize, const B3: usize>
        From<WuF32x3ParallelColorMap<Color, B1, B2, B3>> for WuF32x3ColorMap<Color, B1, B2, B3>
    {
        #[inline]
        fn from(color_map: WuF32x3ParallelColorMap<Color, B1, B2, B3>) -> Self {
            color_map.into_serial()
        }
    }

    impl<Color, const B1: usize, const B2: usize, const B3: usize>
        From<WuF32x3ColorMap<Color, B1, B2, B3>> for WuF32x3ParallelColorMap<Color, B1, B2, B3>
    {
        #[inline]
        fn from(color_map: WuF32x3ColorMap<Color, B1, B2, B3>) -> Self {
            color_map.into_parallel()
        }
    }

    impl<Color: ColorComponents<f32, 3>, const B1: usize, const B2: usize, const B3: usize>
        WuF32x3ParallelColorMap<Color, B1, B2, B3>
    {
        /// Mutate a color slice by mapping each color to its corresponding palette color in parallel.
        #[inline]
        pub fn map_slice_in_place(&self, colors: &mut [Color]) {
            colors
                .par_iter_mut()
                .for_each(|color| *color = self.palette_color(color))
        }
    }

    impl<Color: ColorComponents<f32, 3>, const B1: usize, const B2: usize, const B3: usize>
        IndexedColorMap<Color> for WuF32x3ParallelColorMap<Color, B1, B2, B3>
    {
        type Output = Color;

        #[inline]
        fn into_palette(self) -> PaletteBuf<Self::Output> {
            self.0.into_palette()
        }

        #[inline]
        fn palette(&self) -> &Palette<Self::Output> {
            self.0.palette()
        }

        #[inline]
        fn base_palette(&self) -> &Palette<Color> {
            self.0.palette()
        }

        #[inline]
        fn palette_index(&self, color: &Color) -> u8 {
            self.0.palette_index(color)
        }

        #[inline]
        fn palette_color(&self, color: &Color) -> Self::Output {
            self.0.palette_color(color)
        }

        #[inline]
        fn map_to_indices(&self, input: &[Color]) -> Vec<u8> {
            let WuF32x3ColorMap { binner, hist, .. } = &self.0;
            let flat_hist = hist.as_flattened();
            let input = input.as_arrays();

            let mut output = Vec::<[u8; 8]>::with_capacity(input.len().div_ceil(8));
            let (chunks, remainder) = input.as_chunks::<8>();

            chunks
                .par_iter()
                .with_min_len(2)
                .map(|chunk| {
                    let components = array::from_fn(|i| f32x8::new(chunk.map(|c| c[i])));
                    let bins = binner.index8(components);
                    bins.as_array().map(|bin| flat_hist[bin as usize])
                })
                .collect_into_vec(&mut output);

            let mut output = output.into_flattened();
            output.extend(remainder.iter().map(|&color| hist[binner.bin(color)]));

            output
        }

        #[inline]
        fn map_to_colors_of_palette<Output: Clone + Send + Sync>(
            &self,
            palette: &Palette<Output>,
            input: &[Color],
        ) -> Vec<Output> {
            let WuF32x3ColorMap { binner, hist, .. } = &self.0;
            let flat_hist = hist.as_flattened();
            let input = input.as_arrays();

            let mut output = Vec::<[Output; 8]>::with_capacity(input.len().div_ceil(8));
            let (chunks, remainder) = input.as_chunks::<8>();

            chunks
                .par_iter()
                .map(|chunk| {
                    let components = array::from_fn(|i| f32x8::new(chunk.map(|c| c[i])));
                    let bins = binner.index8(components);
                    bins.as_array()
                        .map(|bin| palette[flat_hist[bin as usize]].clone())
                })
                .collect_into_vec(&mut output);

            let mut output = output.into_flattened();
            output.extend(
                remainder
                    .iter()
                    .map(|&color| palette[hist[binner.bin(color)]].clone()),
            );

            output
        }
    }
}

#[cfg(feature = "threads")]
pub use parallel::*;

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{color_space::srgb8_to_oklab, tests::*};
    use ordered_float::OrderedFloat;
    use palette::Oklab;

    #[test]
    fn not_enough_colors() {
        let len_32 = 64u32;
        let len = len_32 as usize;
        let mut colors = srgb8_to_oklab(&test_data_1024());
        colors.truncate(len);
        let binner = BinnerF32x3::oklab_from_srgb8();
        let palette_size = PaletteSize::MAX;

        let wu = WuF32x3::run_slice(&colors, binner).unwrap();
        let (palette, counts) = wu.palette_and_counts(palette_size);
        assert_eq!(len, palette.len());
        assert_eq!(len_32, counts.iter().copied().sum::<u32>());
        let (color_map, counts) = wu.color_map_and_counts(palette_size);
        assert_eq!(len, color_map.palette().len());
        assert_eq!(len_32, counts.iter().copied().sum::<u32>());

        #[cfg(feature = "threads")]
        {
            let wu = WuF32x3::run_slice_par(&colors, binner).unwrap();
            let (palette, counts) = wu.palette_and_counts(palette_size);
            assert_eq!(len, palette.len());
            assert_eq!(len_32, counts.iter().copied().sum::<u32>());
            let (color_map, counts) = wu.parallel_color_map_and_counts(palette_size);
            assert_eq!(len, color_map.palette().len());
            assert_eq!(len_32, counts.iter().copied().sum::<u32>());
        }
    }

    #[test]
    fn exact_match_image_unaffected() {
        const COUNT: u32 = 4;

        fn sort_oklab(slice: &mut [Oklab]) {
            slice.sort_by_key(|&oklab| cast::into_array(oklab).map(OrderedFloat));
        }

        let expected_palette = {
            let ab = [-0.2, -0.1, 0.0, 0.1, 0.2];
            let mut palette = [0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0]
                .into_iter()
                .zip(ab)
                .zip(ab)
                .map(|((l, a), b)| Oklab::new(l, a, b))
                .collect::<Vec<_>>();
            sort_oklab(&mut palette);
            PaletteBuf::new_unchecked(palette)
        };

        let indices = {
            #[allow(clippy::cast_possible_truncation)]
            let indices = (0..expected_palette.len())
                .map(|i| i as u8)
                .collect::<Box<_>>();
            let mut indices = [indices.as_ref(); COUNT as usize].concat();
            indices.rotate_right(7);
            indices
        };

        let colors = indices
            .iter()
            .map(|&i| expected_palette[i as usize])
            .collect::<Vec<_>>();

        let palette_size = PaletteSize::MAX;
        let binner = BinnerF32x3::oklab_from_srgb8();

        let wu = WuF32x3::run_slice(&colors, binner).unwrap();
        let actual = wu.palette_and_counts(palette_size);
        assert_eq!(actual.1.iter().copied().sum::<u32>() as usize, colors.len());
        assert_eq!(actual.1.as_slice(), vec![COUNT; expected_palette.len()]);
        let actual_palette = {
            let mut palette = actual.0;
            sort_oklab(&mut palette);
            palette
        };
        assert_eq!(actual_palette, expected_palette);

        let actual = wu.color_map_and_counts(palette_size);
        assert_eq!(actual.1.iter().copied().sum::<u32>() as usize, colors.len());
        assert_eq!(actual.1.as_slice(), vec![COUNT; expected_palette.len()]);
        let actual_palette = {
            let mut palette = actual.0.into_palette();
            sort_oklab(&mut palette);
            palette
        };
        assert_eq!(actual_palette, expected_palette);

        #[cfg(feature = "threads")]
        {
            let wu = WuF32x3::run_slice_par(&colors, binner).unwrap();
            let actual = wu.palette_and_counts(palette_size);
            assert_eq!(actual.1.as_slice(), vec![COUNT; expected_palette.len()]);
            assert_eq!(actual.1.iter().copied().sum::<u32>() as usize, colors.len());
            let actual_palette = {
                let mut palette = actual.0;
                sort_oklab(&mut palette);
                palette
            };
            assert_eq!(actual_palette, expected_palette);

            let actual = wu.parallel_color_map_and_counts(palette_size);
            assert_eq!(actual.1.iter().copied().sum::<u32>() as usize, colors.len());
            assert_eq!(actual.1.as_slice(), vec![COUNT; expected_palette.len()]);
            let actual_palette = {
                let mut palette = actual.0.into_palette();
                sort_oklab(&mut palette);
                palette
            };
            assert_eq!(actual_palette, expected_palette);
        }
    }

    #[cfg(feature = "threads")]
    #[test]
    fn single_and_multi_threaded_match() {
        let colors = srgb8_to_oklab(&test_data_1024());
        let binner = BinnerF32x3::oklab_from_srgb8();
        let palette_size = PaletteSize::MAX;

        let wu_single = WuF32x3::run_slice(&colors, binner).unwrap();
        let wu_par = WuF32x3::run_slice_par(&colors, binner).unwrap();

        for (a, b) in wu_single
            .hist
            .as_flattened()
            .iter()
            .zip(wu_par.hist.as_flattened())
        {
            assert_eq!(a.count, b.count);
            #[allow(clippy::float_cmp)]
            {
                assert_eq!(a.components, b.components);
                assert_eq!(a.sum_squared, b.sum_squared);
            }
        }

        let single = wu_single.palette_and_counts(palette_size);
        let par = wu_par.palette_and_counts(palette_size);
        assert_eq!(single, par);
        assert_eq!(single.1.iter().copied().sum::<u32>() as usize, colors.len());

        let single = wu_single.color_map_and_counts(palette_size);
        let par = wu_par.parallel_color_map_and_counts(palette_size);
        assert_eq!(single.0.palette(), par.0.palette());
        assert_eq!(single.1.iter().copied().sum::<u32>() as usize, colors.len());
    }
}
