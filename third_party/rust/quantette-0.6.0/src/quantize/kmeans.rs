//! Color quantization by k-means clustering.
//!
//! This quantization method is slower compared to Wu's quantization method (see the [`wu`](crate::wu)
//! module) but it creates more accurate palettes. Euclidean distance is used for clustering and it
//! is heavily recommended to use a perceptually uniform color space/color type like
//! [`Oklab`](palette::Oklab) as input.
//!
//! The current single-threaded implementation uses online k-means (also known as MacQueen's k-means).
//! The current multi-threaded implementation uses minibatch k-means.
//! The online and stochastic nature of these implementations allows them to escape from
//! local minima more easily compared to batch k-means (a.k.a. Lloyd's algorithm).
//!
//! Both implementations incorporate a learning rate (per centroid) that
//! decreases the influence of each successive sample to the same centroid.
//! So, increasing the number of samples has diminishing returns.
//! Rather, these methods should only need to make one pass (or even less) over the input data.
//! See the docs for [`Kmeans`] and [`KmeansOptions`] for more information.

// The k-means implementations here are based upon the following paper:
//
// Thompson, S., Celebi, M.E. & Buck, K.H. Fast color quantization using MacQueen’s k-means algorithm.
// Journal of Real-Time Image Processing, vol. 17, 1609–1624, 2020.
// https://doi.org/10.1007/s11554-019-00914-6
//
// Accessed from https://faculty.uca.edu/ecelebi/documents/JRTIP_2020a.pdf

use crate::{
    BoundedIndex, BoundedSlice, ColorComponents, ImageRef, IndexedImage, LengthOutOfRange,
    MAX_PIXELS, PaletteBuf,
    color_map::{NearestNeighborColorMap, simd_argmin_min_distance},
};
use alloc::{vec, vec::Vec};
use core::{array, num::NonZeroU32};
use num_traits::AsPrimitive;
use ordered_float::OrderedFloat;
use palette::cast::{self, AsArrays as _};
use rand::{SeedableRng as _, distr::Uniform, prelude::Distribution as _};
use rand_xoshiro::Xoroshiro128PlusPlus;

/// The various options for k-means quantization.
///
/// This struct has a builder API. See the docs for each of the following functions for more details:
/// - [`sampling_factor`](`Self::sampling_factor`)
/// - [`max_samples`](`Self::max_samples`)
/// - [`batch_size`](`Self::batch_size`)
/// - [`seed`](`Self::seed`)
///
/// # Examples
///
/// ```
/// # use quantette::kmeans::KmeansOptions;
/// KmeansOptions::new()
///     .sampling_factor(0.5)
///     .max_samples(512 * 512 / 2)
///     .batch_size(2048)
///     .seed(42);
/// ```
#[must_use]
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub struct KmeansOptions {
    /// The proportion of the input to sample.
    sampling_factor: OrderedFloat<f32>,
    /// The maximum number of pixels to sample.
    max_samples: u32,
    /// The number of samples to batch together.
    batch_size: u32,
    /// The seed for the random number generator.
    seed: u64,
}
impl KmeansOptions {
    /// Create a new [`KmeansOptions`] with default options.
    #[inline]
    pub const fn new() -> Self {
        Self {
            sampling_factor: OrderedFloat(1.0),
            max_samples: 512 * 512,
            batch_size: 4096,
            seed: 0,
        }
    }

    /// Sets the proportion of the input to sample.
    ///
    /// This is typically in the range `0.0..=1.0`. It can be above `1.0`, but this may not give
    /// noticeably better results. You can also set this to `f32::INFINITY` to instead use
    /// [`max_samples`](Self::max_samples) to limit the number of samples independent of the input size.
    ///
    /// The default sampling factor is `1.0`.
    #[inline]
    pub const fn sampling_factor(self, sampling_factor: f32) -> Self {
        Self {
            sampling_factor: OrderedFloat(sampling_factor),
            ..self
        }
    }

    /// Sets the maximum number of pixels to sample from the input.
    ///
    /// The number of samples determined by the [`sampling_factor`](Self::sampling_factor) is
    /// proportional to the input size. However, samples after a certain point will likely not
    /// affect the results in a significant way, since the k-means quantization incorporates a
    /// learning rate that increasingly diminishes the impact of new samples. So, you can use this
    /// option to limit the number samples in the case of a large input.
    ///
    /// Also, you can set [`sampling_factor`](Self::sampling_factor) to `f32::INFINITY`,
    /// which means that this setting will determine the exact number of samples that will occur.
    ///
    /// The default maximum samples is `262144`.
    #[inline]
    pub const fn max_samples(self, max_samples: u32) -> Self {
        Self { max_samples, ..self }
    }

    /// Sets the number of samples to batch together each iteration. This option is only used by the
    /// parallel versions of the k-means quantization functions.
    ///
    /// Increasing the batch size reduces the running time but with dimishing returns.
    /// Smaller batch sizes are more accurate but are slower to run.
    ///
    /// The default batch size is `4096`.
    #[inline]
    pub const fn batch_size(self, batch_size: u32) -> Self {
        Self { batch_size, ..self }
    }

    /// Sets the seed number used for the random number generators.
    ///
    /// The default seed is `0`.
    #[inline]
    pub const fn seed(self, seed: u64) -> Self {
        Self { seed, ..self }
    }

    /// Returns the current sampling factor.
    ///
    /// See [`sampling_factor`](Self::sampling_factor) for more information.
    #[inline]
    pub const fn get_sampling_factor(&self) -> f32 {
        self.sampling_factor.0
    }

    /// Returns the current maximum number of samples.
    ///
    /// See [`max_samples`](Self::max_samples) for more information.
    #[inline]
    pub const fn get_max_samples(&self) -> u32 {
        self.max_samples
    }

    /// Returns the current batch size.
    ///
    /// See [`batch_size`](Self::batch_size) for more information.
    #[inline]
    pub const fn get_batch_size(&self) -> u32 {
        self.batch_size
    }

    /// Returns the current seed number.
    ///
    /// See [`seed`](Self::seed) for more information.
    #[inline]
    pub const fn get_seed(&self) -> u64 {
        self.seed
    }

    /// Returns the number of samples based on the provided `len`.
    #[inline]
    fn num_samples(&self, len: u32) -> Option<NonZeroU32> {
        if len == 0 || self.batch_size == 0 {
            None
        } else {
            #[allow(clippy::cast_sign_loss, clippy::cast_possible_truncation)]
            let samples = (f64::from(len) * f64::from(self.sampling_factor.0)) as u32;
            NonZeroU32::new(samples.min(self.max_samples))
        }
    }
}

impl Default for KmeansOptions {
    #[inline]
    fn default() -> Self {
        Self::new()
    }
}

/// A struct holding the mutable state for k-means.
struct State<Color, Component, const N: usize> {
    /// The nearest neighbor lookup for the current centroids.
    nearest: NearestNeighborColorMap<Color, Component, N>,
    /// The number of samples added to each centroid.
    counts: PaletteBuf<u32>,
}

impl<Color, Component, const N: usize> State<Color, Component, N>
where
    Color: ColorComponents<Component, N>,
    Component: Copy + Into<f32> + 'static,
    f32: AsPrimitive<Component>,
{
    /// Create a new [`State`] with the given initial `centroids`.
    #[inline]
    fn new(centroids: PaletteBuf<Color>) -> Self {
        let counts = PaletteBuf::new_unchecked(vec![0; centroids.len()]);
        let nearest = NearestNeighborColorMap::new(centroids);
        State { nearest, counts }
    }

    /// Add a color to the centroid at the given `chunk` and `lane`.
    #[inline]
    fn add_sample_to(&mut self, chunk: u8, lane: u8, color: [f32; N]) {
        let Self { nearest, counts, .. } = self;

        let i = chunk * 8 + lane;

        let count = counts[usize::from(i)] + 1;
        // We use a learning rate of 0.5 => count^(-0.5)
        #[allow(clippy::cast_possible_truncation)]
        let rate = {
            #[cfg(feature = "std")]
            {
                (1.0 / f64::from(count).sqrt()) as f32
            }
            #[cfg(not(feature = "std"))]
            {
                (1.0 / libm::sqrt(count.into())) as f32
            }
        };

        for (c, x) in nearest.data[usize::from(chunk)].iter_mut().zip(color) {
            let c = &mut c.as_mut_array()[usize::from(lane)];
            *c += rate * (x - *c);
        }

        counts[usize::from(i)] = count;
    }

    /// Add a color to its nearest centroid.
    #[inline]
    fn add_sample(&mut self, color: [Component; N]) {
        let color = color.map(Into::into);
        let (chunk, lane) = simd_argmin_min_distance(&self.nearest.data, color).0;
        self.add_sample_to(chunk, lane, color);
    }

    fn online_kmeans(
        &mut self,
        num_pixels: u32,
        index_to_color: impl Fn(u32) -> Color,
        samples: NonZeroU32,
        options: KmeansOptions,
    ) {
        const BATCH_SIZE: u32 = 256;

        let samples = samples.get();
        #[allow(clippy::expect_used)]
        let distribution = Uniform::new(0, num_pixels).expect("num_pixels != 0");
        let rng = &mut Xoroshiro128PlusPlus::seed_from_u64(options.seed);
        let mut batch = Vec::with_capacity(BATCH_SIZE as usize);

        let mut add_samples = |state: &mut State<Color, Component, N>, n: u32| {
            batch.extend((0..n).map(|_| index_to_color(distribution.sample(rng))));
            for &color in batch.as_arrays() {
                state.add_sample(color);
            }
            batch.clear();
        };

        for _ in 0..(samples / BATCH_SIZE) {
            add_samples(self, BATCH_SIZE);
        }
        add_samples(self, samples % BATCH_SIZE);
    }
}

/// The struct for k-means quantization.
///
/// See the [module](self) documentation and [`KmeansOptions`] docs for more information.
///
/// Slices, [`ImageRef`], and [`IndexedImage`] are supported as inputs via:
/// - [`run_slice`](Self::run_slice)
/// - [`run_image`](Self::run_image)
/// - [`run_indexed_image`](Self::run_indexed_image)
///
/// Parallel versions are also available if the `threads` feature is enabled.
///
/// The functions listed above need initial centroids as inputs. You can use Wu's quantization
/// method from the [`wu`](crate::wu) module to generate a palette to use as the initial centroids.
/// Otherwise, supply your own custom [`PaletteBuf`]. The resulting palette will have the same
/// number of colors as the number of initial centroids.
///
/// To produce the final output, use one of the following:
/// - [`into_palette`](Self::into_palette)
/// - [`into_palette_and_counts`](Self::into_palette_and_counts)
/// - [`into_color_map`](Self::into_color_map)
/// - [`into_color_map_and_counts`](Self::into_color_map_and_counts)
///
/// # Examples
///
/// Minimal example:
/// ```
/// # fn main() -> Result<(), Box<dyn std::error::Error>> {
/// use quantette::{PaletteBuf, kmeans::{Kmeans, KmeansOptions}};
/// use palette::Srgb;
///
/// let input = vec![Srgb::<u8>::new(0, 0, 0)];
/// let centroids = PaletteBuf::new(vec![Srgb::new(0, 0, 0)])?;
/// let palette = Kmeans::run_slice(&input, centroids.clone(), KmeansOptions::new())?.into_palette();
/// assert_eq!(palette.len(), centroids.len());
/// # Ok(())
/// # }
/// ```
///
/// Full image quantization example using [`wu`](crate::wu) to create the initial centroids:
/// ```
/// # fn main() -> Result<(), Box<dyn std::error::Error>> {
/// use quantette::{
///     kmeans::{Kmeans, KmeansOptions},
///     wu::{BinnerF32x3, WuF32x3},
///     ImageBuf, PaletteBuf, PaletteSize,
/// };
/// use palette::Oklab;
///
/// let image = ImageBuf::new(1, 1, vec![Oklab::new(0.0, 0.0, 0.0)])?;
/// let binner = BinnerF32x3::oklab_from_srgb8();
/// let centroids = WuF32x3::run_image(image.as_ref(), binner).unwrap().palette(PaletteSize::MAX);
/// let color_map = Kmeans::run_image(image.as_ref(), centroids, KmeansOptions::new()).into_color_map();
/// let quantized = image.map_to_image(&color_map);
/// assert_eq!(image.dimensions(), quantized.dimensions());
/// # Ok(())
/// # }
/// ```
#[must_use]
pub struct Kmeans<Color, Component, const N: usize> {
    /// The kmeans state, or the original centroids in the case the options specified no samples.
    result: Result<State<Color, Component, N>, PaletteBuf<Color>>,
}

impl<Color, Component, const N: usize> Kmeans<Color, Component, N>
where
    Color: ColorComponents<Component, N>,
    Component: Copy + Into<f32> + 'static,
    f32: AsPrimitive<Component>,
{
    /// Boilerplate code to run kmeans quantization.
    fn run<T>(
        len: u32,
        index_to_color: T,
        centroids: PaletteBuf<Color>,
        options: KmeansOptions,
        f: impl FnOnce(&mut State<Color, Component, N>, u32, T, NonZeroU32, KmeansOptions),
    ) -> Self {
        let result = if let Some(samples) = options.num_samples(len) {
            let mut state = State::new(centroids);
            f(&mut state, len, index_to_color, samples, options);
            Ok(state)
        } else {
            Err(centroids)
        };
        Self { result }
    }

    /// Run kmeans quantization on a [`BoundedSlice`].
    pub(crate) fn run_slice_bounded(
        colors: &BoundedSlice<Color>,
        centroids: PaletteBuf<Color>,
        options: KmeansOptions,
    ) -> Self {
        Self::run(
            colors.length(),
            |i| colors[i as usize],
            centroids,
            options,
            State::online_kmeans,
        )
    }

    /// Run k-means quantization on a slice of colors.
    ///
    /// # Errors
    ///
    /// Returns an error if the length of `colors` is greater than [`MAX_PIXELS`].
    pub fn run_slice(
        colors: &[Color],
        centroids: PaletteBuf<Color>,
        options: KmeansOptions,
    ) -> Result<Self, LengthOutOfRange> {
        LengthOutOfRange::check_u32(colors, 0, MAX_PIXELS).map(|len| {
            Self::run(
                len,
                |i| colors[i as usize],
                centroids,
                options,
                State::online_kmeans,
            )
        })
    }

    /// Run k-means quantization on an [`ImageRef`].
    pub fn run_image(
        image: ImageRef<'_, Color>,
        centroids: PaletteBuf<Color>,
        options: KmeansOptions,
    ) -> Self {
        let pixels = image.as_slice();
        Self::run(
            image.num_pixels(),
            |i| pixels[i as usize],
            centroids,
            options,
            State::online_kmeans,
        )
    }

    /// Run k-means quantization on an [`IndexedImage`].
    pub fn run_indexed_image<Index>(
        image: &IndexedImage<Color, Index>,
        centroids: PaletteBuf<Color>,
        options: KmeansOptions,
    ) -> Self
    where
        Index: BoundedIndex + Into<u32>,
        Index::Length: Into<u32>,
    {
        let palette = image.palette();
        let indices = image.indices();
        Self::run(
            image.num_pixels(),
            |i| palette[indices[i as usize].as_()],
            centroids,
            options,
            State::online_kmeans,
        )
    }

    /// Boilerplate code to prepare the final output.
    fn finalize<T>(
        self,
        f: impl FnOnce(Result<State<Color, Component, N>, PaletteBuf<Color>>) -> T,
    ) -> T {
        let Self { mut result } = self;
        if let Ok(State { nearest, .. }) = &mut result {
            for (palette, data) in nearest.palette.chunks_mut(8).zip(&nearest.data) {
                let colors = array::from_fn::<Color, 8, _>(|i| {
                    cast::from_array(data.map(|x| x.as_array()[i].as_()))
                });
                palette.copy_from_slice(&colors[..palette.len()]);
            }
        }
        f(result)
    }

    /// Returns the final color palette.
    #[must_use]
    pub fn into_palette(self) -> PaletteBuf<Color> {
        self.finalize(|result| match result {
            Ok(data) => data.nearest.into_palette(),
            Err(palette) => palette,
        })
    }

    /// Returns the final color palette and the number of samples assigned to each palette color.
    #[must_use]
    pub fn into_palette_and_counts(self) -> (PaletteBuf<Color>, PaletteBuf<u32>) {
        self.finalize(|result| match result {
            Ok(State { nearest, counts, .. }) => (nearest.into_palette(), counts),
            Err(palette) => {
                let counts = PaletteBuf::new_unchecked(vec![0; palette.len()]);
                (palette, counts)
            }
        })
    }

    /// Returns the color map and the number of samples assigned to each palette color.
    #[must_use]
    pub fn into_color_map_and_counts(
        self,
    ) -> (
        NearestNeighborColorMap<Color, Component, N>,
        PaletteBuf<u32>,
    ) {
        self.finalize(|result| match result {
            Ok(State { nearest, counts, .. }) => (nearest, counts),
            Err(palette) => {
                let counts = PaletteBuf::new_unchecked(vec![0; palette.len()]);
                let color_map = NearestNeighborColorMap::new(palette);
                (color_map, counts)
            }
        })
    }

    /// Returns the color map.
    #[must_use]
    pub fn into_color_map(self) -> NearestNeighborColorMap<Color, Component, N> {
        self.finalize(|result| match result {
            Ok(State { nearest, .. }) => nearest,
            Err(palette) => NearestNeighborColorMap::new(palette),
        })
    }
}

#[cfg(feature = "threads")]
/// Module for code gated behind the `threads` feature.
mod parallel {
    use super::{Kmeans, KmeansOptions, State};
    use crate::{
        BoundedIndex, ColorComponents, ImageRef, IndexedImage, LengthOutOfRange, MAX_PIXELS,
        PaletteBuf,
        color_map::{NearestNeighborParallelColorMap, simd_argmin_min_distance},
    };
    use alloc::vec;
    use core::num::NonZeroU32;
    use num_traits::AsPrimitive;
    use palette::cast::{self, AsArrays as _};
    use rand::{SeedableRng as _, distr::Uniform, prelude::Distribution as _};
    use rand_xoshiro::Xoroshiro128PlusPlus;
    use rayon::prelude::*;

    impl<Color, Component, const N: usize> State<Color, Component, N>
    where
        Color: ColorComponents<Component, N>,
        Component: Copy + Into<f32> + 'static + Send + Sync,
        f32: AsPrimitive<Component>,
    {
        /// Run minibatch kmeans for the given number of samples.
        fn minibatch_kmeans(
            &mut self,
            _num_pixels: u32,
            colors: &[Color],
            samples: NonZeroU32,
            options: KmeansOptions,
        ) {
            /// Used to align to 64 bytes (most likely a cache line).
            // This is to prevent false sharing. It doesn't seem to make a noticeable difference,
            // even though the multiple items in `rng` below would otherwise share the same cache line.
            // We'll keep this just in case, since it may make a difference on some hardware?
            #[repr(align(64))]
            struct Align64<T>(T);

            let samples = samples.get();
            let KmeansOptions { batch_size, seed, .. } = options;
            let threads = rayon::current_num_threads();
            let chunk_size = (batch_size as usize).div_ceil(threads);

            #[allow(clippy::expect_used)]
            let distribution = Uniform::new(0, colors.len()).expect("num_pixels != 0");
            let mut rng = (0..threads)
                .scan(Xoroshiro128PlusPlus::seed_from_u64(seed), |rng, _| {
                    rng.jump();
                    Some(Align64(rng.clone()))
                })
                .collect::<Vec<_>>();

            let mut batch = vec![[0.0.as_(); N]; batch_size as usize];
            let mut assignments = vec![(0, 0); batch_size as usize];

            let colors = colors.as_arrays();

            let mut run = |state: &mut State<Color, Component, N>,
                           batch: &mut [[Component; N]],
                           assignments: &mut [(u8, u8)],
                           chunk_size| {
                batch
                    .par_chunks_mut(chunk_size)
                    .zip(assignments.par_chunks_mut(chunk_size))
                    .zip(&mut rng)
                    .for_each(|((batch, assignments), Align64(rng))| {
                        for color in &mut *batch {
                            *color = colors[distribution.sample(rng)];
                        }

                        for (color, center) in batch.iter().zip(assignments) {
                            *center = simd_argmin_min_distance(
                                &state.nearest.data,
                                color.map(Into::into),
                            )
                            .0;
                        }
                    });

                for (color, &(chunk, lane)) in batch.iter().zip(&*assignments) {
                    state.add_sample_to(chunk, lane, color.map(Into::into));
                }
            };

            for _ in 0..(samples / batch_size) {
                run(self, &mut batch, &mut assignments, chunk_size);
            }

            let remainder = (samples % batch_size) as usize;
            if remainder != 0 {
                run(
                    self,
                    &mut batch[..remainder],
                    &mut assignments[..remainder],
                    remainder.div_ceil(threads),
                );
            }
        }

        /// Run minibatch kmeans on an [`IndexedImage`] for the given number of samples.
        fn minibatch_kmeans_indexed<Index: BoundedIndex>(
            &mut self,
            _num_pixels: u32,
            image: &IndexedImage<Color, Index>,
            samples: NonZeroU32,
            options: KmeansOptions,
        ) {
            /// Used to align to 64 bytes (most likely a cache line).
            // This is to prevent false sharing.
            // It doesn't seem to make a noticeable difference,
            // even though the multiple items in `rng` below would otherwise share the same cache line.
            // We'll keep this just in case, since it may make a difference on some hardware?
            #[repr(align(64))]
            struct Align64<T>(T);

            let samples = samples.get();
            let KmeansOptions { batch_size, seed, .. } = options;
            let threads = rayon::current_num_threads();
            let chunk_size = (batch_size as usize).div_ceil(threads);

            #[allow(clippy::expect_used)]
            let distribution = Uniform::new(0, image.num_pixels()).expect("num_pixels != 0");
            let mut rng = (0..threads)
                .scan(Xoroshiro128PlusPlus::seed_from_u64(seed), |rng, _| {
                    rng.jump();
                    Some(Align64(rng.clone()))
                })
                .collect::<Vec<_>>();

            let mut batch = vec![[0.0.as_(); N]; batch_size as usize];
            let mut assignments = vec![(0, 0); batch_size as usize];

            let colors = image.palette();
            let indices = image.indices();

            let mut run = |state: &mut State<Color, Component, N>,
                           batch: &mut [[Component; N]],
                           assignments: &mut [(u8, u8)],
                           chunk_size| {
                batch
                    .par_chunks_mut(chunk_size)
                    .zip(assignments.par_chunks_mut(chunk_size))
                    .zip(&mut rng)
                    .for_each(|((batch, assignments), Align64(rng))| {
                        for color in &mut *batch {
                            let index = indices[distribution.sample(rng) as usize];
                            *color = cast::into_array(colors[index.as_()]);
                        }

                        for (color, center) in batch.iter().zip(assignments) {
                            *center = simd_argmin_min_distance(
                                &state.nearest.data,
                                color.map(Into::into),
                            )
                            .0;
                        }
                    });

                for (color, &(chunk, lane)) in batch.iter().zip(&*assignments) {
                    state.add_sample_to(chunk, lane, color.map(Into::into));
                }
            };

            for _ in 0..(samples / batch_size) {
                run(self, &mut batch, &mut assignments, chunk_size);
            }

            let remainder = (samples % batch_size) as usize;
            if remainder != 0 {
                run(
                    self,
                    &mut batch[..remainder],
                    &mut assignments[..remainder],
                    remainder.div_ceil(threads),
                );
            }
        }
    }

    impl<Color, Component, const N: usize> Kmeans<Color, Component, N>
    where
        Color: ColorComponents<Component, N>,
        Component: Copy + Into<f32> + 'static + Send + Sync,
        f32: AsPrimitive<Component>,
    {
        /// Run kmeans in parallel on a slice of colors without checking that length is in bounds.
        #[allow(clippy::cast_possible_truncation)]
        pub(crate) fn run_slice_par_unchecked(
            colors: &[Color],
            centroids: PaletteBuf<Color>,
            options: KmeansOptions,
        ) -> Self {
            Self::run(
                colors.len() as u32,
                colors,
                centroids,
                options,
                State::minibatch_kmeans,
            )
        }

        /// Run k-means in parallel on a slice of colors.
        ///
        /// # Errors
        ///
        /// Returns an error if the length of `colors` is greater than [`MAX_PIXELS`].
        #[inline]
        pub fn run_slice_par(
            colors: &[Color],
            centroids: PaletteBuf<Color>,
            options: KmeansOptions,
        ) -> Result<Self, LengthOutOfRange> {
            LengthOutOfRange::check_u32(colors, 0, MAX_PIXELS)
                .map(|len| Self::run(len, colors, centroids, options, State::minibatch_kmeans))
        }

        /// Run k-means quantization in parallel on an [`ImageRef`].
        #[inline]
        pub fn run_image_par(
            image: ImageRef<'_, Color>,
            centroids: PaletteBuf<Color>,
            options: KmeansOptions,
        ) -> Self {
            Self::run_slice_par_unchecked(image.as_slice(), centroids, options)
        }

        /// Run k-means quantization in parallel on an [`IndexedImage`].
        pub fn run_indexed_image_par<Index>(
            image: &IndexedImage<Color, Index>,
            centroids: PaletteBuf<Color>,
            options: KmeansOptions,
        ) -> Self
        where
            Index: BoundedIndex + Into<u32>,
            Index::Length: Into<u32>,
        {
            Self::run(
                image.num_pixels(),
                image,
                centroids,
                options,
                State::minibatch_kmeans_indexed,
            )
        }

        /// Returns the parallel color map and the number of samples assigned to each
        /// palette color.
        #[must_use]
        #[inline]
        pub fn into_parallel_color_map_and_counts(
            self,
        ) -> (
            NearestNeighborParallelColorMap<Color, Component, N>,
            PaletteBuf<u32>,
        ) {
            let (color_map, counts) = self.into_color_map_and_counts();
            (color_map.into(), counts)
        }

        /// Returns the parallel color map.
        #[must_use]
        #[inline]
        pub fn into_parallel_color_map(
            self,
        ) -> NearestNeighborParallelColorMap<Color, Component, N> {
            self.into_color_map().into()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tests::*;
    use palette::Srgb;

    fn test_palette() -> PaletteBuf<Srgb<u8>> {
        let mut centroids = test_data_256();
        centroids.truncate(249u8.try_into().unwrap()); // use non-multiple of 8 to test remainder handling
        centroids
    }

    #[test]
    fn no_samples_gives_initial_centroids() {
        let colors = test_data_1024();
        let centroids = test_palette();
        let options = KmeansOptions::new().max_samples(0);

        let actual = Kmeans::run_slice(&colors, centroids.clone(), options)
            .unwrap()
            .into_palette_and_counts();
        let expected = (
            centroids.clone(),
            PaletteBuf::new_unchecked(vec![0; centroids.len()]),
        );
        assert_eq!(actual, expected);

        #[cfg(feature = "threads")]
        {
            let actual = Kmeans::run_slice_par(&colors, centroids.clone(), options.batch_size(64))
                .unwrap()
                .into_palette_and_counts();
            assert_eq!(actual, expected);
        }
    }

    #[cfg(feature = "threads")]
    #[test]
    fn zero_batch_size_gives_initial_centroids() {
        let colors = test_data_1024();
        let centroids = test_palette();
        let options = KmeansOptions::new().max_samples(0);

        let expected = (
            centroids.clone(),
            PaletteBuf::new_unchecked(vec![0; centroids.len()]),
        );
        let actual = Kmeans::run_slice_par(&colors, centroids.clone(), options.batch_size(64))
            .unwrap()
            .into_palette_and_counts();
        assert_eq!(actual, expected);
    }

    #[test]
    fn empty_input_gives_initial_centroids() {
        let centroids = test_palette();
        let options = KmeansOptions::new().max_samples(0);

        let actual = Kmeans::run_slice(&[], centroids.clone(), options)
            .unwrap()
            .into_palette_and_counts();
        let expected = (
            centroids.clone(),
            PaletteBuf::new_unchecked(vec![0; centroids.len()]),
        );
        assert_eq!(actual, expected);

        let actual = Kmeans::run_image(ImageRef::default(), centroids.clone(), options)
            .into_palette_and_counts();
        assert_eq!(actual, expected);

        let actual = Kmeans::run_indexed_image(
            &IndexedImage::<_, u8>::default(),
            centroids.clone(),
            options,
        )
        .into_palette_and_counts();
        assert_eq!(actual, expected);

        #[cfg(feature = "threads")]
        {
            let actual = Kmeans::run_slice_par(&[], centroids.clone(), options.batch_size(64))
                .unwrap()
                .into_palette_and_counts();
            assert_eq!(actual, expected);

            let actual = Kmeans::run_image_par(ImageRef::default(), centroids.clone(), options)
                .into_palette_and_counts();
            assert_eq!(actual, expected);

            let actual = Kmeans::run_indexed_image_par(
                &IndexedImage::<_, u8>::default(),
                centroids.clone(),
                options,
            )
            .into_palette_and_counts();
            assert_eq!(actual, expected);
        }
    }

    #[test]
    fn exact_match_image_unaffected() {
        let centroids = test_palette();

        let indices = {
            #[allow(clippy::cast_possible_truncation)]
            let indices = (0..centroids.len()).map(|i| i as u8).collect::<Vec<_>>();
            let mut indices = [indices.as_slice(); 4].concat();
            indices.rotate_right(7);
            indices
        };

        let colors = indices.iter().map(|&i| centroids[i]).collect::<Vec<_>>();

        let samples = 505;
        let options = KmeansOptions::new().max_samples(samples);
        let (palette, counts) = Kmeans::run_slice(&colors, centroids.clone(), options)
            .unwrap()
            .into_palette_and_counts();
        assert_eq!(palette, centroids);
        assert_eq!(counts.len(), centroids.len());
        assert_eq!(counts.into_iter().sum::<u32>(), samples);

        #[cfg(feature = "threads")]
        {
            let (palette, counts) =
                Kmeans::run_slice_par(&colors, centroids.clone(), options.batch_size(64))
                    .unwrap()
                    .into_palette_and_counts();
            assert_eq!(palette, centroids);
            assert_eq!(counts.len(), centroids.len());
            assert_eq!(counts.into_iter().sum::<u32>(), samples);
        }
    }
}
