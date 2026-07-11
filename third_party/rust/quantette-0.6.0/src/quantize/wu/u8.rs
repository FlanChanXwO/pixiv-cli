use super::shared::{Histogram3, Stats, Wu, sum_of_squares};
use crate::{
    BoundedIndex, BoundedSlice, ColorComponents, ImageRef, IndexedColorMap, IndexedImageCounts,
    LengthOutOfRange, Palette, PaletteBuf, PaletteCounts, PaletteSize,
};
use core::marker::PhantomData;
use palette::cast::{self, AsArrays as _};

/// A histogram binner for colors consisting of 3 `u8` components.
///
/// `B1`, `B2`, and `B3` specify the number of bins to have in each dimension
/// and they must all be a power of 2 less than or equal to `128`.
/// It is recommended to weight the number of bins based on the importance of each dimension.
#[derive(Debug, Clone, Copy, Default)]
pub struct BinnerU8x3<const B1: usize, const B2: usize, const B3: usize>(());

impl<const B1: usize, const B2: usize, const B3: usize> BinnerU8x3<B1, B2, B3> {
    /// Create a new [`BinnerU8x3`].
    ///
    /// Fails at compile time if `B1`, `B2`, or `B3` are not a power of 2 or are greater than `128`.
    #[must_use]
    #[inline]
    pub const fn new() -> Self {
        const {
            assert!(B1.is_power_of_two());
            assert!(B2.is_power_of_two());
            assert!(B3.is_power_of_two());

            assert!(B1.ilog2() < u8::BITS);
            assert!(B2.ilog2() < u8::BITS);
            assert!(B3.ilog2() < u8::BITS);
        }

        Self(())
    }
}
impl BinnerU8x3<16, 32, 16> {
    /// Returns the default binner used for [`Srgb<u8>`](palette::Srgb) colors.
    #[must_use]
    #[inline]
    pub const fn rgb() -> Self {
        Self::new()
    }
}

impl<const B1: usize, const B2: usize, const B3: usize> BinnerU8x3<B1, B2, B3> {
    #[allow(clippy::unused_self)] // Force calls to `new`.
    #[allow(clippy::trivially_copy_pass_by_ref)] // See comment below.
    #[inline]
    /// Returns the histogram bins for each color component.
    fn bin(&self, components: &[u8; 3]) -> [u8; 3] {
        // Noticeably different assembly is generated depending on whether `components`
        // is passed by value or by reference. Passing by reference results in "cleaner" assembly
        // that is 10-25% faster.
        let [c1, c2, c3] = components;
        [
            c1 >> (u8::BITS - B1.ilog2()),
            c2 >> (u8::BITS - B2.ilog2()),
            c3 >> (u8::BITS - B3.ilog2()),
        ]
    }
}

impl<const B1: usize, const B2: usize, const B3: usize> Histogram3<Stats<u32, 3, u64>, B1, B2, B3> {
    /// Add the given colors to the histogram.
    fn add_colors_u8_3_u32(&mut self, colors: &[[u8; 3]], binner: BinnerU8x3<B1, B2, B3>) {
        for &color in colors {
            let bin = binner.bin(&color);
            let Stats { count, components, sum_squared } = &mut self[bin];
            let color = color.map(u32::from);

            *count += 1;
            for (c, v) in components.iter_mut().zip(color) {
                *c += v;
            }
            *sum_squared += sum_of_squares(color);
        }
    }

    /// Add the given colors and their counts to the histogram.
    fn add_color_counts_u8_3_u32(
        &mut self,
        colors: &[[u8; 3]],
        counts: &[u32],
        binner: BinnerU8x3<B1, B2, B3>,
    ) {
        for (&color, &n) in colors.iter().zip(counts) {
            let bin = binner.bin(&color);
            let Stats { count, components, sum_squared } = &mut self[bin];
            let color = color.map(u32::from);

            *count += u64::from(n);

            for (c, v) in components.iter_mut().zip(color) {
                *c += n * v;
            }

            let w = f64::from(n);
            *sum_squared += w * sum_of_squares(color);
        }
    }
}

impl<const B1: usize, const B2: usize, const B3: usize> Histogram3<Stats<u64, 3, u32>, B1, B2, B3> {
    /// Add the given colors to the histogram.
    fn add_colors_u8_3_u64(&mut self, colors: &[[u8; 3]], binner: BinnerU8x3<B1, B2, B3>) {
        for &color in colors {
            let bin = binner.bin(&color);
            let Stats { count, components, sum_squared } = &mut self[bin];
            let color = color.map(u64::from);

            *count += 1;
            for (c, v) in components.iter_mut().zip(color) {
                *c += v;
            }
            *sum_squared += sum_of_squares(color);
        }
    }

    /// Add the given colors and their counts to the histogram.
    fn add_color_counts_u8_3_u64(
        &mut self,
        colors: &[[u8; 3]],
        counts: &[u32],
        binner: BinnerU8x3<B1, B2, B3>,
    ) {
        for (&color, &n) in colors.iter().zip(counts) {
            let bin = binner.bin(&color);
            let Stats { count, components, sum_squared } = &mut self[bin];
            let color = color.map(u64::from);

            *count += n;

            let w = u64::from(n);
            for (c, v) in components.iter_mut().zip(color) {
                *c += w * v;
            }

            let w = f64::from(n);
            *sum_squared += w * sum_of_squares(color);
        }
    }
}

/// The minimum number of colors that could trigger an overflow when using a `u32` as the
/// sum/accumulator.
const SUM_POSSIBLE_OVERFLOW: u32 = u32::MAX / u8::MAX as u32;

/// An enum over histograms with either `u32` or `u64` as the sum type.
enum Hist<const B1: usize, const B2: usize, const B3: usize> {
    /// Histogram with `u32` sums.
    U32(Histogram3<Stats<u32, 3, u64>, B1, B2, B3>),
    /// Histogram with `u64` sums.
    U64(Histogram3<Stats<u64, 3, u32>, B1, B2, B3>),
}

/// The struct for Wu's color quantization method for colors with `u8` components in 3 dimensions.
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
/// the same [`WuU8x3`] with different [`PaletteSize`]s.
///
/// # Examples
///
/// Minimal example:
/// ```
/// use quantette::{PaletteSize, wu::{WuU8x3, BinnerU8x3}};
/// use palette::Srgb;
///
/// let input = vec![Srgb::new(0, 0, 0)];
/// let binner = BinnerU8x3::rgb();
/// let palette = WuU8x3::run_slice(&input, binner).unwrap().palette(PaletteSize::MAX);
/// assert_eq!(palette.len(), input.len());
/// ```
///
/// Full image quantization example:
/// ```
/// # fn main() -> Result<(), Box<dyn std::error::Error>> {
/// use quantette::{
///     wu::{BinnerU8x3, WuU8x3},
///     ImageBuf, PaletteSize,
/// };
/// use palette::Srgb;
///
/// let image = ImageBuf::new(1, 1, vec![Srgb::new(0, 0, 0)])?;
/// let binner = BinnerU8x3::rgb();
/// let color_map = WuU8x3::run_image(image.as_ref(), binner).unwrap().color_map(PaletteSize::MAX);
/// let quantized = image.map_to_image(&color_map);
/// assert_eq!(image.dimensions(), quantized.dimensions());
/// # Ok(())
/// # }
/// ```
#[must_use]
pub struct WuU8x3<Color, const B1: usize, const B2: usize, const B3: usize> {
    /// The color type must remain the same for each [`WuU8x3`].
    color: PhantomData<Color>,
    /// The histogram data.
    hist: Hist<B1, B2, B3>,
}

impl<Color: ColorComponents<u8, 3>, const B1: usize, const B2: usize, const B3: usize>
    WuU8x3<Color, B1, B2, B3>
{
    pub(crate) fn run_slice_bounded(
        colors: &BoundedSlice<Color>,
        binner: BinnerU8x3<B1, B2, B3>,
    ) -> Self {
        let hist = if colors.len() < SUM_POSSIBLE_OVERFLOW as usize {
            let mut hist = Histogram3::new();
            hist.add_colors_u8_3_u32(colors.as_arrays(), binner);
            hist.calc_cumulative_moments();
            Hist::U32(hist)
        } else {
            let mut hist = Histogram3::new();
            hist.add_colors_u8_3_u64(colors.as_arrays(), binner);
            hist.calc_cumulative_moments();
            Hist::U64(hist)
        };
        Self { color: PhantomData, hist }
    }

    /// Run Wu's quantization method on a slice of colors.
    ///
    /// # Errors
    ///
    /// Returns an error if `colors` is empty or longer than [`MAX_PIXELS`](crate::MAX_PIXELS).
    pub fn run_slice(
        colors: &[Color],
        binner: BinnerU8x3<B1, B2, B3>,
    ) -> Result<Self, LengthOutOfRange> {
        let colors = BoundedSlice::new(colors)?;
        Ok(Self::run_slice_bounded(colors, binner))
    }

    /// Run Wu's quantization method on an [`ImageRef`].
    ///
    /// Returns `None` if the image is empty.
    #[must_use]
    pub fn run_image(image: ImageRef<'_, Color>, binner: BinnerU8x3<B1, B2, B3>) -> Option<Self> {
        let pixels = BoundedSlice::new(image.as_slice()).ok()?;
        Some(Self::run_slice_bounded(pixels, binner))
    }

    fn run_palette_and_counts(
        palette: &[Color],
        counts: &[u32],
        total_count: u32,
        binner: BinnerU8x3<B1, B2, B3>,
    ) -> Option<Self> {
        (total_count != 0).then(|| {
            let hist = if total_count < SUM_POSSIBLE_OVERFLOW {
                let mut hist = Histogram3::new();
                hist.add_color_counts_u8_3_u32(palette.as_arrays(), counts, binner);
                hist.calc_cumulative_moments();
                Hist::U32(hist)
            } else {
                let mut hist = Histogram3::new();
                hist.add_color_counts_u8_3_u64(palette.as_arrays(), counts, binner);
                hist.calc_cumulative_moments();
                Hist::U64(hist)
            };
            Self { color: PhantomData, hist }
        })
    }

    /// Run Wu's quantization method on an [`PaletteCounts`].
    ///
    /// Returns `None` if the [`PaletteCounts`] is empty.
    #[must_use]
    pub fn run_palette_counts(
        palette_counts: &PaletteCounts<Color>,
        binner: BinnerU8x3<B1, B2, B3>,
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
        binner: BinnerU8x3<B1, B2, B3>,
    ) -> Option<Self> {
        Self::run_palette_and_counts(image.palette(), image.counts(), image.total_count(), binner)
    }

    /// Compute the color palette.
    #[must_use]
    pub fn palette(&self, k: PaletteSize) -> PaletteBuf<Color> {
        match &self.hist {
            Hist::U32(hist) => Wu::<_, Color, _, _>::new(hist).palette(k),
            Hist::U64(hist) => Wu::<_, Color, _, _>::new(hist).palette(k),
        }
    }

    /// Compute the color palette and the number of pixels assigned to each palette color.
    #[must_use]
    pub fn palette_and_counts(&self, k: PaletteSize) -> (PaletteBuf<Color>, PaletteBuf<u32>) {
        match &self.hist {
            Hist::U32(hist) => Wu::<_, Color, _, _>::new(hist).palette_and_counts(k),
            Hist::U64(hist) => Wu::<_, Color, _, _>::new(hist).palette_and_counts(k),
        }
    }

    /// Compute the color map and the number of pixels assigned to each palette color.
    #[must_use]
    pub fn color_map_and_counts(
        &self,
        k: PaletteSize,
    ) -> (WuU8x3ColorMap<Color, B1, B2, B3>, PaletteBuf<u32>) {
        let (palette, counts, cubes) = match &self.hist {
            Hist::U32(hist) => Wu::<_, Color, _, _>::new(hist).palette_counts_and_cubes(k),
            Hist::U64(hist) => Wu::<_, Color, _, _>::new(hist).palette_counts_and_cubes(k),
        };
        let hist = Histogram3::from_cubes(cubes);
        let color_map = WuU8x3ColorMap { palette, hist };
        (color_map, counts)
    }

    /// Compute the color map.
    #[must_use]
    pub fn color_map(&self, k: PaletteSize) -> WuU8x3ColorMap<Color, B1, B2, B3> {
        let (palette, cubes) = match &self.hist {
            Hist::U32(hist) => Wu::<_, Color, _, _>::new(hist).palette_and_cubes(k),
            Hist::U64(hist) => Wu::<_, Color, _, _>::new(hist).palette_and_cubes(k),
        };
        let hist = Histogram3::from_cubes(cubes);
        WuU8x3ColorMap { palette, hist }
    }
}

/// An [`IndexedColorMap`] that maps colors to histograms bins and the palette index or color
/// associated with the histogram bin.
///
/// Can only be created from [`WuU8x3::color_map`] or [`WuU8x3::color_map_and_counts`].
#[derive(Clone, Debug)]
pub struct WuU8x3ColorMap<Color, const B1: usize, const B2: usize, const B3: usize> {
    /// The color palette.
    palette: PaletteBuf<Color>,
    /// The histogram containing palette indices.
    hist: Histogram3<u8, B1, B2, B3>,
}

impl<Color, const B1: usize, const B2: usize, const B3: usize> WuU8x3ColorMap<Color, B1, B2, B3> {
    /// Consume a [`WuU8x3ColorMap`] and return the underlying [`PaletteBuf`].
    #[must_use]
    #[inline]
    pub fn into_palette(self) -> PaletteBuf<Color> {
        self.palette
    }

    /// Returns the [`Palette`] of colors of a [`WuU8x3ColorMap`].
    #[inline]
    pub fn palette(&self) -> &Palette<Color> {
        &self.palette
    }
}

impl<Color: ColorComponents<u8, 3>, const B1: usize, const B2: usize, const B3: usize>
    WuU8x3ColorMap<Color, B1, B2, B3>
{
    /// Mutate a color slice by mapping each color to its corresponding palette color.
    #[inline]
    pub fn map_slice_in_place(&self, colors: &mut [Color]) {
        for color in colors {
            *color = self.palette_color(color);
        }
    }
}

impl<Color: ColorComponents<u8, 3>, const B1: usize, const B2: usize, const B3: usize>
    IndexedColorMap<Color> for WuU8x3ColorMap<Color, B1, B2, B3>
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
        let Self { hist, .. } = self;
        let bin = BinnerU8x3::<B1, B2, B3>::new().bin(cast::into_array_ref(color));
        hist[bin]
    }
}

#[cfg(feature = "threads")]
mod parallel {
    use super::{
        super::shared::Histogram3, BinnerU8x3, Hist, SUM_POSSIBLE_OVERFLOW, WuU8x3, WuU8x3ColorMap,
    };
    use crate::{
        BoundedIndex, BoundedSlice, ColorComponents, ImageRef, IndexedColorMap, IndexedImageCounts,
        LengthOutOfRange, Palette, PaletteBuf, PaletteCounts, PaletteSize,
    };
    use core::marker::PhantomData;
    use palette::cast::AsArrays as _;
    use rayon::prelude::*;

    impl<Color: ColorComponents<u8, 3>, const B1: usize, const B2: usize, const B3: usize>
        WuU8x3<Color, B1, B2, B3>
    {
        /// Return the per thread chunk size based on the length.
        fn chunk_size(len: usize) -> usize {
            let chunk_size = len
                .div_ceil(rayon::current_num_threads())
                .max(B1 * B2 * B3 * 4);
            let num_chunks = len.div_ceil(chunk_size);
            len.div_ceil(num_chunks)
        }

        pub(crate) fn run_slice_par_bounded(
            colors: &BoundedSlice<Color>,
            binner: BinnerU8x3<B1, B2, B3>,
        ) -> Self {
            let hist = if colors.len() < SUM_POSSIBLE_OVERFLOW as usize {
                let chunk_size = Self::chunk_size(colors.len());
                let mut hist = colors
                    .as_arrays()
                    .par_chunks(chunk_size)
                    .map(|colors| {
                        let mut hist = Histogram3::new();
                        hist.add_colors_u8_3_u32(colors, binner);
                        hist
                    })
                    .reduce_with(Histogram3::merge_partial)
                    .unwrap_or_else(Histogram3::new);

                hist.calc_cumulative_moments();
                Hist::U32(hist)
            } else {
                let chunk_size = Self::chunk_size(colors.len());
                let mut hist = colors
                    .as_arrays()
                    .par_chunks(chunk_size)
                    .map(|colors| {
                        let mut hist = Histogram3::new();
                        hist.add_colors_u8_3_u64(colors, binner);
                        hist
                    })
                    .reduce_with(Histogram3::merge_partial)
                    .unwrap_or_else(Histogram3::new);

                hist.calc_cumulative_moments();
                Hist::U64(hist)
            };

            Self { color: PhantomData, hist }
        }

        /// Run Wu's quantization method on a slice of colors in parallel.
        ///
        /// # Errors
        ///
        /// Returns an error if `colors` is empty or longer than [`MAX_PIXELS`](crate::MAX_PIXELS).
        pub fn run_slice_par(
            colors: &[Color],
            binner: BinnerU8x3<B1, B2, B3>,
        ) -> Result<Self, LengthOutOfRange> {
            let colors = BoundedSlice::new(colors)?;
            Ok(Self::run_slice_par_bounded(colors, binner))
        }

        /// Run Wu's quantization method on an [`ImageRef`] in parallel.
        ///
        /// Returns `None` if the image is empty.
        #[must_use]
        pub fn run_image_par(
            image: ImageRef<'_, Color>,
            binner: BinnerU8x3<B1, B2, B3>,
        ) -> Option<Self> {
            let pixels = BoundedSlice::new(image.as_slice()).ok()?;
            Some(Self::run_slice_par_bounded(pixels, binner))
        }

        fn run_palette_and_counts_par(
            palette: &[Color],
            counts: &[u32],
            total_count: u32,
            binner: BinnerU8x3<B1, B2, B3>,
        ) -> Option<Self> {
            (total_count != 0).then(|| {
                let hist = if total_count < SUM_POSSIBLE_OVERFLOW {
                    let chunk_size = Self::chunk_size(palette.len());
                    let mut hist = palette
                        .as_arrays()
                        .par_chunks(chunk_size)
                        .zip(counts.par_chunks(chunk_size))
                        .map(|(colors, counts)| {
                            let mut hist = Histogram3::new();
                            hist.add_color_counts_u8_3_u32(colors, counts, binner);
                            hist
                        })
                        .reduce_with(Histogram3::merge_partial)
                        .unwrap_or_else(Histogram3::new);

                    hist.calc_cumulative_moments();
                    Hist::U32(hist)
                } else {
                    let chunk_size = Self::chunk_size(palette.len());
                    let mut hist = palette
                        .as_arrays()
                        .par_chunks(chunk_size)
                        .zip(counts.par_chunks(chunk_size))
                        .map(|(colors, counts)| {
                            let mut hist = Histogram3::new();
                            hist.add_color_counts_u8_3_u64(colors, counts, binner);
                            hist
                        })
                        .reduce_with(Histogram3::merge_partial)
                        .unwrap_or_else(Histogram3::new);

                    hist.calc_cumulative_moments();
                    Hist::U64(hist)
                };

                Self { color: PhantomData, hist }
            })
        }

        /// Run Wu's quantization method on an [`PaletteCounts`] in parallel.
        ///
        /// Returns `None` if the [`PaletteCounts`] is empty.
        #[must_use]
        pub fn run_palette_counts_par(
            palette_counts: &PaletteCounts<Color>,
            binner: BinnerU8x3<B1, B2, B3>,
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
            binner: BinnerU8x3<B1, B2, B3>,
        ) -> Option<Self> {
            Self::run_palette_and_counts_par(
                image.palette(),
                image.counts(),
                image.total_count(),
                binner,
            )
        }
    }

    impl<Color: ColorComponents<u8, 3>, const B1: usize, const B2: usize, const B3: usize>
        WuU8x3<Color, B1, B2, B3>
    {
        #[must_use]
        #[inline]
        /// Compute the parallel color map and the number of pixels assigned to each
        /// palette color.
        pub fn parallel_color_map_and_counts(
            &self,
            k: PaletteSize,
        ) -> (WuU8x3ParallelColorMap<Color, B1, B2, B3>, PaletteBuf<u32>) {
            let (color_map, counts) = self.color_map_and_counts(k);
            (WuU8x3ParallelColorMap(color_map), counts)
        }

        #[must_use]
        #[inline]
        /// Compute the parallel color map.
        pub fn parallel_color_map(
            &self,
            k: PaletteSize,
        ) -> WuU8x3ParallelColorMap<Color, B1, B2, B3> {
            WuU8x3ParallelColorMap(self.color_map(k))
        }
    }

    /// An [`IndexedColorMap`] that, in parallel, maps colors to histograms bins and the
    /// palette index or color associated with the histogram bin.
    ///
    /// Can only be created from [`WuU8x3::parallel_color_map`] or [`WuU8x3::parallel_color_map_and_counts`].
    #[derive(Clone, Debug)]
    pub struct WuU8x3ParallelColorMap<Color, const B1: usize, const B2: usize, const B3: usize>(
        WuU8x3ColorMap<Color, B1, B2, B3>,
    );

    impl<Color, const B1: usize, const B2: usize, const B3: usize>
        WuU8x3ParallelColorMap<Color, B1, B2, B3>
    {
        /// Consume a [`WuU8x3ParallelColorMap`] and return the underlying [`PaletteBuf`].
        #[must_use]
        #[inline]
        pub fn into_palette(self) -> PaletteBuf<Color> {
            self.0.into_palette()
        }

        /// Returns the [`Palette`] of colors of a [`WuU8x3ParallelColorMap`].
        #[inline]
        pub fn palette(&self) -> &Palette<Color> {
            self.0.palette()
        }

        /// Convert a [`WuU8x3ParallelColorMap`] to a [`WuU8x3ColorMap`].
        #[must_use]
        #[inline]
        pub fn into_serial(self) -> WuU8x3ColorMap<Color, B1, B2, B3> {
            self.0
        }
    }

    impl<Color, const B1: usize, const B2: usize, const B3: usize> WuU8x3ColorMap<Color, B1, B2, B3> {
        /// Convert a [`WuU8x3ColorMap`] to a [`WuU8x3ParallelColorMap`].
        #[must_use]
        #[inline]
        pub fn into_parallel(self) -> WuU8x3ParallelColorMap<Color, B1, B2, B3> {
            WuU8x3ParallelColorMap(self)
        }
    }

    impl<Color, const B1: usize, const B2: usize, const B3: usize>
        From<WuU8x3ParallelColorMap<Color, B1, B2, B3>> for WuU8x3ColorMap<Color, B1, B2, B3>
    {
        #[inline]
        fn from(color_map: WuU8x3ParallelColorMap<Color, B1, B2, B3>) -> Self {
            color_map.into_serial()
        }
    }

    impl<Color, const B1: usize, const B2: usize, const B3: usize>
        From<WuU8x3ColorMap<Color, B1, B2, B3>> for WuU8x3ParallelColorMap<Color, B1, B2, B3>
    {
        #[inline]
        fn from(color_map: WuU8x3ColorMap<Color, B1, B2, B3>) -> Self {
            color_map.into_parallel()
        }
    }

    impl<Color: ColorComponents<u8, 3>, const B1: usize, const B2: usize, const B3: usize>
        WuU8x3ParallelColorMap<Color, B1, B2, B3>
    {
        /// Mutate a color slice by mapping each color to its corresponding palette color in parallel.
        #[inline]
        pub fn map_slice_in_place(&self, colors: &mut [Color]) {
            colors
                .par_iter_mut()
                .for_each(|color| *color = self.palette_color(color))
        }
    }

    impl<Color: ColorComponents<u8, 3>, const B1: usize, const B2: usize, const B3: usize>
        IndexedColorMap<Color> for WuU8x3ParallelColorMap<Color, B1, B2, B3>
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
            input
                .par_iter()
                .map(|color| self.palette_index(color))
                .collect()
        }

        #[inline]
        fn map_to_colors_of_palette<Output: Clone + Send + Sync>(
            &self,
            palette: &Palette<Output>,
            input: &[Color],
        ) -> Vec<Output> {
            input
                .par_iter()
                .map(|color| palette[self.palette_index(color)].clone())
                .collect()
        }
    }
}

#[cfg(feature = "threads")]
pub use parallel::*;

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tests::*;

    #[test]
    fn not_enough_colors() {
        let len_32 = 64u32;
        let len = len_32 as usize;
        let mut colors = test_data_1024();
        colors.truncate(len);
        let binner = BinnerU8x3::<32, 32, 32>::new();
        let palette_size = PaletteSize::MAX;

        let wu = WuU8x3::run_slice(&colors, binner).unwrap();
        let (palette, counts) = wu.palette_and_counts(palette_size);
        assert_eq!(len, palette.len());
        assert_eq!(len_32, counts.iter().copied().sum::<u32>());
        let (color_map, counts) = wu.color_map_and_counts(palette_size);
        assert_eq!(len, color_map.palette().len());
        assert_eq!(len_32, counts.iter().copied().sum::<u32>());

        #[cfg(feature = "threads")]
        {
            let wu = WuU8x3::run_slice_par(&colors, binner).unwrap();
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

        let expected_palette = {
            let mut palette = test_data_256();
            palette.sort_by_key(|srgb| srgb.into_components());
            palette
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
            .map(|&i| expected_palette[i])
            .collect::<Vec<_>>();

        let palette_size = PaletteSize::MAX;
        let binner = BinnerU8x3::<32, 32, 32>::new();

        let wu = WuU8x3::run_slice(&colors, binner).unwrap();
        let actual = wu.palette_and_counts(palette_size);
        assert_eq!(actual.1.iter().copied().sum::<u32>() as usize, colors.len());
        assert_eq!(actual.1.as_slice(), vec![COUNT; expected_palette.len()]);
        let actual_palette = {
            let mut palette = actual.0;
            palette.sort_by_key(|srgb| srgb.into_components());
            palette
        };
        assert_eq!(actual_palette, expected_palette);

        let actual = wu.color_map_and_counts(palette_size);
        assert_eq!(actual.1.iter().copied().sum::<u32>() as usize, colors.len());
        assert_eq!(actual.1.as_slice(), vec![COUNT; expected_palette.len()]);
        let actual_palette = {
            let mut palette = actual.0.into_palette();
            palette.sort_by_key(|srgb| srgb.into_components());
            palette
        };
        assert_eq!(actual_palette, expected_palette);

        #[cfg(feature = "threads")]
        {
            let wu = WuU8x3::run_slice_par(&colors, binner).unwrap();
            let actual = wu.palette_and_counts(palette_size);
            assert_eq!(actual.1.as_slice(), vec![COUNT; expected_palette.len()]);
            assert_eq!(actual.1.iter().copied().sum::<u32>() as usize, colors.len());
            let actual_palette = {
                let mut palette = actual.0;
                palette.sort_by_key(|srgb| srgb.into_components());
                palette
            };
            assert_eq!(actual_palette, expected_palette);

            let actual = wu.parallel_color_map_and_counts(palette_size);
            assert_eq!(actual.1.iter().copied().sum::<u32>() as usize, colors.len());
            assert_eq!(actual.1.as_slice(), vec![COUNT; expected_palette.len()]);
            let actual_palette = {
                let mut palette = actual.0.into_palette();
                palette.sort_by_key(|srgb| srgb.into_components());
                palette
            };
            assert_eq!(actual_palette, expected_palette);
        }
    }

    #[cfg(feature = "threads")]
    #[test]
    fn single_and_multi_threaded_match() {
        fn unwrap_hist<const B1: usize, const B2: usize, const B3: usize>(
            hist: &Hist<B1, B2, B3>,
        ) -> &[Stats<u32, 3, u64>] {
            #[allow(clippy::unimplemented)]
            if let Hist::U32(hist) = hist {
                hist.as_flattened()
            } else {
                unimplemented!()
            }
        }

        let colors = test_data_1024();
        let binner = BinnerU8x3::rgb();
        let palette_size = PaletteSize::MAX;

        let wu_single = WuU8x3::run_slice(&colors, binner).unwrap();
        let wu_par = WuU8x3::run_slice_par(&colors, binner).unwrap();

        for (a, b) in unwrap_hist(&wu_single.hist)
            .iter()
            .zip(unwrap_hist(&wu_par.hist))
        {
            assert_eq!(a.count, b.count);
            assert_eq!(a.components, b.components);
            #[allow(clippy::float_cmp)]
            {
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
