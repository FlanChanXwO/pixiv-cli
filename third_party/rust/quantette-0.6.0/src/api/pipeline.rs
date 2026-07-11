#[cfg(feature = "kmeans")]
use crate::kmeans::Kmeans;
use crate::{
    BoundedSlice, ImageBuf, ImageRef, IndexedColorMap, IndexedImage, IndexedImageCounts,
    LengthOutOfRange, PaletteBuf, PaletteSize, QuantizeMethod,
    color_map::{NearestNeighborColorMap, PaletteSubstitution},
    color_space::{oklab_to_srgb8, srgb8_to_oklab},
    dedup,
    dither::FloydSteinberg,
    wu::{BinnerF32x3, WuF32x3},
};
#[cfg(feature = "threads")]
use crate::{color_map::NearestNeighborParallelColorMap, color_space::srgb8_to_oklab_par};
use alloc::vec;
use bytemuck::Zeroable;
use palette::{Oklab, Srgb};

/// A builder struct to specify image quantization options.
///
/// Note that the [`Oklab`] color space is used in all cases to provide reasonable results.
///
/// # Examples
///
/// First, specify any options you want:
/// ```
/// # fn main() -> Result<(), quantette::PaletteSizeFromIntError> {
/// use quantette::{Pipeline, QuantizeMethod};
///
/// let pipeline = Pipeline::new()
///     .palette_size(32u16.try_into()?)
///     .ditherer(None)
///     .quantize_method(QuantizeMethod::kmeans())
///     .parallel(true);
/// # Ok(())
/// # }
/// ```
///
/// Note that some of the options and functions above require certain cargo features to be enabled.
///
/// Then, specify the input image or color slice as well as the desired output:
/// ```
/// # fn main() -> Result<(), Box<dyn std::error::Error>> {
/// use quantette::{ImageBuf, Pipeline, QuantizeMethod};
///
/// // let img = image::open("some image")?.into_rgb8();
/// let img = image::RgbImage::new(256, 256);
/// let img = ImageBuf::try_from(img)?;
///
/// # let pipeline = Pipeline::new()
/// #     .palette_size(32u16.try_into()?)
/// #     .ditherer(None)
/// #     .quantize_method(QuantizeMethod::kmeans())
/// #     .parallel(true);
/// #
/// let quantized = pipeline
///     .clone()
///     .input_image(img.as_ref())
///     .output_srgb8_image();
///
/// assert_eq!(img.dimensions(), quantized.dimensions());
///
/// let palette = pipeline
///     .input_image(img.as_ref())
///     .output_srgb8_palette()
///     .map(|palette| palette.into_vec())
///     .unwrap_or_default();
///
/// assert_eq!(palette.len(), 1);
///
/// # Ok(())
/// # }
/// ```
#[must_use]
#[derive(Debug, Clone, PartialEq)]
pub struct Pipeline {
    /// The number of colors to put in the palette.
    k: PaletteSize,
    /// The color quantization method to use.
    quantize_method: QuantizeMethod,
    /// Whether or not to perform dithering on the image.
    ditherer: Option<FloydSteinberg>,
    /// Whether or not to dedup pixels in the image as an optimization.
    dedup: Option<bool>,
    #[cfg(feature = "threads")]
    /// Whether or not to run the pipeline in parallel.
    parallel: bool,
}
impl Pipeline {
    /// Create a new [`Pipeline`] with default options.
    pub fn new() -> Self {
        Self {
            k: PaletteSize::MAX,
            quantize_method: QuantizeMethod::Wu,
            ditherer: Some(FloydSteinberg::new()),
            dedup: None,
            #[cfg(feature = "threads")]
            parallel: false,
        }
    }

    /// Sets the palette size which determines the (maximum) number of colors to have in the palette.
    ///
    /// See the docs for [`PaletteSize`] for more information.
    ///
    /// The default palette size is [`PaletteSize::MAX`].
    ///
    /// # Examples
    ///
    /// ```
    /// # use quantette::{PaletteSize, PaletteSizeFromIntError, Pipeline};
    /// # fn main() -> Result<(), PaletteSizeFromIntError> {
    /// let pipeline = Pipeline::new()
    ///     .palette_size(24u16.try_into()?)
    ///     .palette_size(PaletteSize::from_u8_clamped(24))
    ///     .palette_size(PaletteSize::MAX);
    /// # Ok(())
    /// # }
    /// ```
    #[inline]
    pub fn palette_size(mut self, size: PaletteSize) -> Self {
        self.k = size;
        self
    }

    /// Sets the color quantization method to use.
    ///
    /// See the docs for [`QuantizeMethod`] for more details.
    ///
    /// The default quantization method is [`QuantizeMethod::Wu`].
    ///
    /// # Examples
    ///
    /// ```
    /// # fn main() -> Result<(), quantette::CreatePaletteBufError<palette::Srgb<u8>>> {
    /// use quantette::{kmeans::KmeansOptions, PaletteBuf, Pipeline, QuantizeMethod};
    /// use palette::Srgb;
    ///
    /// let colors = vec![Srgb::new(0, 0, 0), Srgb::new(255, 255, 255)];
    /// let custom_palette = PaletteBuf::try_from(colors.clone())?;
    /// let pipeline = Pipeline::new()
    ///     .quantize_method(QuantizeMethod::Wu)
    ///     .quantize_method(QuantizeMethod::try_from(colors)?)
    ///     .quantize_method(custom_palette)
    ///     .quantize_method(QuantizeMethod::kmeans())
    ///     .quantize_method(KmeansOptions::new().sampling_factor(0.5));
    /// # Ok(())
    /// # }
    /// ```
    #[inline]
    pub fn quantize_method(mut self, quantize_method: impl Into<QuantizeMethod>) -> Self {
        self.quantize_method = quantize_method.into();
        self
    }

    #[cfg(feature = "threads")]
    /// Sets whether or not to run the pipeline in parallel across multiple threads.
    ///
    /// Use a [`rayon::ThreadPool`] to specify the number of threads if necessary.
    ///
    /// The default value is `false`.
    #[inline]
    pub fn parallel(mut self, parallel: bool) -> Self {
        self.parallel = parallel;
        self
    }

    /// Sets whether or not to apply dithering to the image.
    ///
    /// The current implementation only supports Floyd–Steinberg dithering.
    /// See the docs for [`FloydSteinberg`] for more details.
    /// To disable dithering, provide `None` as a value to this function.
    ///
    /// The default value is `Some(FloydSteinberg::new())`.
    ///
    /// # Examples
    ///
    /// ```
    /// use quantette::{dither::FloydSteinberg, Pipeline};
    ///
    /// let ditherer = FloydSteinberg::with_error_diffusion(0.75).unwrap();
    /// let pipeline = Pipeline::new()
    ///     .ditherer(None)
    ///     .ditherer(FloydSteinberg::new())
    ///     .ditherer(ditherer);
    /// ```
    #[inline]
    pub fn ditherer(mut self, ditherer: impl Into<Option<FloydSteinberg>>) -> Self {
        self.ditherer = ditherer.into();
        self
    }

    /// Sets whether or not to deduplicate pixels in the image as an optimization.
    ///
    /// If dithering is enabled (see [`ditherer`](Self::ditherer)), it is recommended to keep this
    /// option as default or set to `false`.
    ///
    /// Otherwise, with dithering disabled:
    /// - For large images it is recommended to deduplicate pixels for overall faster quantization.
    /// - For smaller images, not performing deduplication can be faster.
    ///
    /// The optimal cutoff between a "large" and "small" image depends on:
    /// - the ratio between the number of unique pixels in the image and the total number of pixels in the image
    /// - the chosen quantization method
    /// - hardware
    ///
    /// Some experimentation may be needed based on your workflow and hardware.
    ///
    /// The default value is `None` (automatically choose whether or not to dedup).
    ///
    /// # Examples
    ///
    /// ```
    /// # use quantette::Pipeline;
    /// Pipeline::new()
    ///     .dedup(None) // automatically choose whether or not to dedup
    ///     .dedup(true) // always dedup (not recommended if dithering is enabled)
    ///     .dedup(false); // never dedup
    /// ```
    #[inline]
    pub fn dedup(mut self, dedup: impl Into<Option<bool>>) -> Self {
        self.dedup = dedup.into();
        self
    }

    /// Specify a slice of [`Srgb<u8>`] colors as input.
    ///
    /// Note that only palettes can be outputted when a slice is used as input.
    /// To get an image as an output, provide an image as input using [`input_image`](Self::input_image).
    ///
    /// # Errors
    ///
    /// Returns an error if `colors` is empty or has a length greater than [`MAX_PIXELS`](crate::MAX_PIXELS).
    #[inline]
    pub fn input_slice(
        self,
        colors: &[Srgb<u8>],
    ) -> Result<PipelineWithSliceInput<'_>, LengthOutOfRange> {
        let colors = BoundedSlice::new(colors)?;
        Ok(PipelineWithSliceInput { options: self, colors })
    }

    /// Specify a sRGB image reference as input.
    ///
    /// See the [`ImageRef`] docs for more information as well as how to create one.
    #[inline]
    pub fn input_image(self, image: ImageRef<'_, Srgb<u8>>) -> PipelineWithImageRefInput<'_> {
        PipelineWithImageRefInput { options: self, image }
    }
}

impl Default for Pipeline {
    #[inline]
    fn default() -> Self {
        Self::new()
    }
}

/// A [`Pipeline`] paired with some slice of pixels, ready to be computed into a palette.
#[must_use]
#[derive(Debug, Clone, PartialEq)]
pub struct PipelineWithSliceInput<'a> {
    /// The options to use when generating the quantizied image.
    options: Pipeline,
    /// The input slice of pixels.
    colors: &'a BoundedSlice<Srgb<u8>>,
}

impl PipelineWithSliceInput<'_> {
    /// Runs the pipeline and returns the computed [`PaletteBuf<Oklab>`].
    #[allow(clippy::missing_panics_doc)]
    #[must_use]
    pub fn output_oklab_palette(self) -> PaletteBuf<Oklab> {
        let Self { options, colors } = self;
        let Pipeline {
            k,
            quantize_method,
            dedup,
            #[cfg(feature = "threads")]
            parallel,
            ..
        } = options;

        let binner = BinnerF32x3::oklab_from_srgb8();

        #[cfg(feature = "threads")]
        if parallel {
            return match quantize_method {
                QuantizeMethod::CustomPalette(palette) => palette.into_oklab(),
                QuantizeMethod::Wu =>
                {
                    #[allow(clippy::expect_used)]
                    if dedup.unwrap_or(colors.len() >= 2048 * 2048) {
                        let palette_counts = dedup::dedup_colors_u8_3_counts_bounded_par(colors)
                            .map(|palette| srgb8_to_oklab_par(&palette));
                        WuF32x3::run_palette_counts_par(&palette_counts, binner)
                            .expect("deduping a non-empty slice to not result in an empty slice")
                            .palette(k)
                    } else {
                        let colors = srgb8_to_oklab_par(colors);
                        WuF32x3::run_slice_bounded_par(&colors, binner).palette(k)
                    }
                }
                #[cfg(feature = "kmeans")]
                QuantizeMethod::Kmeans(options) => {
                    if dedup.unwrap_or(colors.len() >= 2048 * 2048) {
                        let image = ImageRef::new_unchecked(colors.length(), 1, colors);
                        let image = dedup::dedup_image_u8_3_counts_par(image)
                            .map(|palette| srgb8_to_oklab_par(&palette));
                        #[allow(clippy::expect_used)]
                        let centroids = WuF32x3::run_indexed_image_counts_par(&image, binner)
                            .expect("deduping a non-empty image to not result in an empty image")
                            .palette(k);
                        Kmeans::run_indexed_image_par(image.as_ref(), centroids, options)
                            .into_palette()
                    } else {
                        let colors = srgb8_to_oklab_par(colors);
                        let centroids = WuF32x3::run_slice_bounded_par(&colors, binner).palette(k);
                        Kmeans::run_slice_par_unchecked(&colors, centroids, options).into_palette()
                    }
                }
            };
        }

        match quantize_method {
            QuantizeMethod::CustomPalette(palette) => palette.into_oklab(),
            QuantizeMethod::Wu =>
            {
                #[allow(clippy::expect_used)]
                if dedup.unwrap_or(colors.len() >= 2048 * 2048) {
                    let palette_counts = dedup::dedup_colors_u8_3_counts_bounded(colors)
                        .map(|palette| srgb8_to_oklab(&palette));
                    WuF32x3::run_palette_counts(&palette_counts, binner)
                        .expect("deduping a non-empty slice to not result in an empty slice")
                        .palette(k)
                } else {
                    let colors = srgb8_to_oklab(colors);
                    let colors = BoundedSlice::new_unchecked(&colors);
                    WuF32x3::run_slice_bounded(colors, binner).palette(k)
                }
            }
            #[cfg(feature = "kmeans")]
            QuantizeMethod::Kmeans(options) => {
                if dedup.unwrap_or(colors.len() >= 2048 * 2048) {
                    let image = ImageRef::new_unchecked(colors.length(), 1, colors);
                    let image = dedup::dedup_image_u8_3_counts(image)
                        .map(|palette| srgb8_to_oklab(&palette));
                    #[allow(clippy::expect_used)]
                    let centroids = WuF32x3::run_indexed_image_counts(&image, binner)
                        .expect("deduping a non-empty image to not result in an empty image")
                        .palette(k);
                    Kmeans::run_indexed_image(image.as_ref(), centroids, options).into_palette()
                } else {
                    let colors = srgb8_to_oklab(colors);
                    let colors = BoundedSlice::new_unchecked(&colors);
                    let centroids = WuF32x3::run_slice_bounded(colors, binner).palette(k);
                    Kmeans::run_slice_bounded(colors, centroids, options).into_palette()
                }
            }
        }
    }

    /// Runs the pipeline and returns the computed [`PaletteBuf<Srgb<u8>>`].
    #[must_use]
    pub fn output_srgb8_palette(self) -> PaletteBuf<Srgb<u8>> {
        PaletteBuf::from_mapping(&self.output_oklab_palette(), oklab_to_srgb8)
    }

    /// Runs the pipeline and returns the computed [`PaletteBuf<Oklab>`] alongside the number of
    /// pixels or samples assigned to each palette color.
    #[allow(clippy::missing_panics_doc)]
    #[must_use]
    pub fn output_oklab_palette_and_counts(self) -> (PaletteBuf<Oklab>, PaletteBuf<u32>) {
        let Self { options, colors } = self;
        let Pipeline {
            k,
            quantize_method,
            dedup,
            #[cfg(feature = "threads")]
            parallel,
            ..
        } = options;

        let binner = BinnerF32x3::oklab_from_srgb8();

        #[cfg(feature = "threads")]
        if parallel {
            return match quantize_method {
                QuantizeMethod::CustomPalette(palette) => {
                    let palette = palette.into_oklab();
                    let counts = PaletteBuf::new_unchecked(vec![0; palette.len()]);
                    (palette, counts)
                }
                QuantizeMethod::Wu =>
                {
                    #[allow(clippy::expect_used)]
                    if dedup.unwrap_or(colors.len() >= 2048 * 2048) {
                        let palette_counts = dedup::dedup_colors_u8_3_counts_bounded_par(colors)
                            .map(|palette| srgb8_to_oklab_par(&palette));
                        WuF32x3::run_palette_counts_par(&palette_counts, binner)
                            .expect("deduping a non-empty slice to not result in an empty slice")
                            .palette_and_counts(k)
                    } else {
                        let colors = srgb8_to_oklab_par(colors);
                        WuF32x3::run_slice_bounded_par(&colors, binner).palette_and_counts(k)
                    }
                }
                #[cfg(feature = "kmeans")]
                QuantizeMethod::Kmeans(options) => {
                    if dedup.unwrap_or(colors.len() >= 2048 * 2048) {
                        let image = ImageRef::new_unchecked(colors.length(), 1, colors);
                        let image = dedup::dedup_image_u8_3_counts_par(image)
                            .map(|palette| srgb8_to_oklab_par(&palette));
                        #[allow(clippy::expect_used)]
                        let centroids = WuF32x3::run_indexed_image_counts_par(&image, binner)
                            .expect("deduping a non-empty image to not result in an empty image")
                            .palette(k);
                        Kmeans::run_indexed_image_par(image.as_ref(), centroids, options)
                            .into_palette_and_counts()
                    } else {
                        let colors = srgb8_to_oklab_par(colors);
                        let centroids = WuF32x3::run_slice_bounded_par(&colors, binner).palette(k);
                        Kmeans::run_slice_par_unchecked(&colors, centroids, options)
                            .into_palette_and_counts()
                    }
                }
            };
        }

        match quantize_method {
            QuantizeMethod::CustomPalette(palette) => {
                let palette = palette.into_oklab();
                let counts = PaletteBuf::new_unchecked(vec![0; palette.len()]);
                (palette, counts)
            }
            QuantizeMethod::Wu =>
            {
                #[allow(clippy::expect_used)]
                if dedup.unwrap_or(colors.len() >= 2048 * 2048) {
                    let palette_counts = dedup::dedup_colors_u8_3_counts_bounded(colors)
                        .map(|palette| srgb8_to_oklab(&palette));
                    WuF32x3::run_palette_counts(&palette_counts, binner)
                        .expect("deduping a non-empty slice to not result in an empty slice")
                        .palette_and_counts(k)
                } else {
                    let colors = srgb8_to_oklab(colors);
                    let colors = BoundedSlice::new_unchecked(&colors);
                    WuF32x3::run_slice_bounded(colors, binner).palette_and_counts(k)
                }
            }
            #[cfg(feature = "kmeans")]
            QuantizeMethod::Kmeans(options) => {
                if dedup.unwrap_or(colors.len() >= 2048 * 2048) {
                    let image = ImageRef::new_unchecked(colors.length(), 1, colors);
                    let image = dedup::dedup_image_u8_3_counts(image)
                        .map(|palette| srgb8_to_oklab(&palette));
                    #[allow(clippy::expect_used)]
                    let centroids = WuF32x3::run_indexed_image_counts(&image, binner)
                        .expect("deduping a non-empty image to not result in an empty image")
                        .palette(k);
                    Kmeans::run_indexed_image(image.as_ref(), centroids, options)
                        .into_palette_and_counts()
                } else {
                    let colors = srgb8_to_oklab(colors);
                    let colors = BoundedSlice::new_unchecked(&colors);
                    let centroids = WuF32x3::run_slice_bounded(colors, binner).palette(k);
                    Kmeans::run_slice_bounded(colors, centroids, options).into_palette_and_counts()
                }
            }
        }
    }

    /// Runs the pipeline and returns the computed [`PaletteBuf<Srgb<u8>>`] alongside the number of
    /// pixels or samples assigned to each palette color.
    #[must_use]
    pub fn output_srgb8_palette_and_counts(self) -> (PaletteBuf<Srgb<u8>>, PaletteBuf<u32>) {
        let (palette, counts) = self.output_oklab_palette_and_counts();
        let palette = PaletteBuf::from_mapping(&palette, oklab_to_srgb8);
        (palette, counts)
    }
}

/// A [`Pipeline`] paired with some image, ready to be quantized or computed into a palette.
#[must_use]
#[derive(Debug, Clone, PartialEq)]
pub struct PipelineWithImageRefInput<'a> {
    /// The options to use when generating the quantizied image.
    options: Pipeline,
    /// The input image as a flat slice of pixels.
    image: ImageRef<'a, Srgb<u8>>,
}

macro_rules! impl_pipeline {
    ($pipeline: expr, $remap_indexed: ident, $remap_image: ident, $output: ty) => {{
        fn run_indexed<ColorMap: IndexedColorMap<Oklab, Output = Oklab>>(
            image: ImageRef<'_, Srgb<u8>>,
            ditherer: Option<FloydSteinberg>,
            quantize: impl FnOnce(&IndexedImageCounts<Oklab, u32>) -> Option<ColorMap>,
        ) -> Option<$output> {
            let image =
                dedup::dedup_image_u8_3_counts(image).map(|palette| srgb8_to_oklab(&palette));
            let color_map = quantize(&image)?;
            let image = $remap_indexed(image.as_ref(), ditherer, color_map);
            Some(image)
        }

        fn run_image<ColorMap: IndexedColorMap<Oklab, Output = Oklab>>(
            image: ImageRef<'_, Srgb<u8>>,
            ditherer: Option<FloydSteinberg>,
            quantize: impl FnOnce(ImageRef<'_, Oklab>) -> Option<ColorMap>,
        ) -> Option<$output> {
            let image = image.map_ref(srgb8_to_oklab);
            let color_map = quantize(image.as_ref())?;
            let image = $remap_image(image.as_ref(), ditherer, color_map);
            Some(image)
        }

        let Self { options, image } = $pipeline;
        let Pipeline { k, quantize_method, dedup, ditherer, .. } = options;

        let binner = BinnerF32x3::oklab_from_srgb8();

        match quantize_method {
            QuantizeMethod::CustomPalette(palette) => {
                #[cfg(not(target_feature = "avx2"))]
                let dedup_threshold = (1024 * 1024 * 2) / u32::from(k.as_u16() / 8).max(1);
                #[cfg(target_feature = "avx2")]
                let dedup_threshold = (4096 * 4096) / u32::from(k.as_u16() / 8).max(1);
                let color_map = NearestNeighborColorMap::new(palette.into_oklab());
                if dedup.unwrap_or(ditherer.is_none() && image.num_pixels() >= dedup_threshold) {
                    run_indexed(image, ditherer, |_| Some(color_map))
                } else {
                    run_image(image, ditherer, |_| Some(color_map))
                }
            }
            QuantizeMethod::Wu => {
                if dedup.unwrap_or(ditherer.is_none() && image.num_pixels() >= 2048 * 2048) {
                    run_indexed(image, ditherer, |image| {
                        WuF32x3::run_indexed_image_counts(image, binner).map(|wu| wu.color_map(k))
                    })
                } else {
                    run_image(image, ditherer, |image| {
                        WuF32x3::run_image(image.as_ref(), binner).map(|wu| wu.color_map(k))
                    })
                }
            }
            #[cfg(feature = "kmeans")]
            QuantizeMethod::Kmeans(options) => {
                #[cfg(not(target_feature = "avx2"))]
                let dedup_threshold = (1024 * 1024 * 2) / u32::from(k.as_u16() / 8).max(1);
                #[cfg(target_feature = "avx2")]
                let dedup_threshold = (4096 * 4096) / u32::from(k.as_u16() / 8).max(1);
                if dedup.unwrap_or(ditherer.is_none() && image.num_pixels() >= dedup_threshold) {
                    run_indexed(image, ditherer, |image| {
                        let centroids =
                            WuF32x3::run_indexed_image_counts(image, binner)?.palette(k);
                        let kmeans = Kmeans::run_indexed_image(image.as_ref(), centroids, options);
                        Some(kmeans.into_color_map())
                    })
                } else {
                    run_image(image, ditherer, |image| {
                        let centroids = WuF32x3::run_image(image, binner)?.palette(k);
                        let kmeans = Kmeans::run_image(image, centroids, options);
                        Some(kmeans.into_color_map())
                    })
                }
            }
        }
    }};
}

#[cfg(feature = "threads")]
macro_rules! impl_pipeline_par {
    ($pipeline: expr, $remap_indexed: ident, $remap_image: ident, $output: ty) => {{
        fn run_indexed<ColorMap: IndexedColorMap<Oklab, Output = Oklab> + Sync>(
            image: ImageRef<'_, Srgb<u8>>,
            ditherer: Option<FloydSteinberg>,
            quantize: impl FnOnce(&IndexedImageCounts<Oklab, u32>) -> Option<ColorMap>,
        ) -> Option<$output> {
            let image = dedup::dedup_image_u8_3_counts_par(image)
                .map(|palette| srgb8_to_oklab_par(&palette));
            let color_map = quantize(&image)?;
            let image = $remap_indexed(image.as_ref(), ditherer, color_map);
            Some(image)
        }

        fn run_image<ColorMap: IndexedColorMap<Oklab, Output = Oklab> + Sync>(
            image: ImageRef<'_, Srgb<u8>>,
            ditherer: Option<FloydSteinberg>,
            quantize: impl FnOnce(ImageRef<'_, Oklab>) -> Option<ColorMap>,
        ) -> Option<$output> {
            let image = image.map_ref(srgb8_to_oklab_par);
            let color_map = quantize(image.as_ref())?;
            let image = $remap_image(image.as_ref(), ditherer, color_map);
            Some(image)
        }

        let Self { options, image } = $pipeline;
        let Pipeline { k, quantize_method, dedup, ditherer, .. } = options;

        let binner = BinnerF32x3::oklab_from_srgb8();

        match quantize_method {
            QuantizeMethod::CustomPalette(palette) => {
                #[cfg(not(target_feature = "avx2"))]
                let dedup_threshold = (4096 * 4096) / u32::from(k.as_u16() / 32).max(1);
                #[cfg(target_feature = "avx2")]
                let dedup_threshold = (4096 * 4096 * 2) / u32::from(k.as_u16() / 32).max(1);
                let color_map = NearestNeighborParallelColorMap::new(palette.into_oklab());
                if dedup.unwrap_or(ditherer.is_none() && image.num_pixels() >= dedup_threshold) {
                    run_indexed(image, ditherer, |_| Some(color_map))
                } else {
                    run_image(image, ditherer, |_| Some(color_map))
                }
            }
            QuantizeMethod::Wu => {
                #[cfg(not(target_feature = "avx2"))]
                let dedup_threshold = 1024 * 1024 * 2;
                #[cfg(target_feature = "avx2")]
                let dedup_threshold = 4096 * 4096;
                if dedup.unwrap_or(ditherer.is_none() && image.num_pixels() >= dedup_threshold) {
                    run_indexed(image, ditherer, |image| {
                        WuF32x3::run_indexed_image_counts_par(image, binner)
                            .map(|wu| wu.parallel_color_map(k))
                    })
                } else {
                    run_image(image, ditherer, |image| {
                        WuF32x3::run_image_par(image.as_ref(), binner)
                            .map(|wu| wu.parallel_color_map(k))
                    })
                }
            }
            #[cfg(feature = "kmeans")]
            QuantizeMethod::Kmeans(options) => {
                #[cfg(not(target_feature = "avx2"))]
                let dedup_threshold = (4096 * 4096) / u32::from(k.as_u16() / 32).max(1);
                #[cfg(target_feature = "avx2")]
                let dedup_threshold = (4096 * 4096 * 2) / u32::from(k.as_u16() / 32).max(1);
                if dedup.unwrap_or(ditherer.is_none() && image.num_pixels() >= dedup_threshold) {
                    run_indexed(image, ditherer, |image| {
                        let centroids =
                            WuF32x3::run_indexed_image_counts_par(image, binner)?.palette(k);
                        let kmeans =
                            Kmeans::run_indexed_image_par(image.as_ref(), centroids, options);
                        Some(kmeans.into_parallel_color_map())
                    })
                } else {
                    run_image(image, ditherer, |image| {
                        let centroids = WuF32x3::run_image_par(image, binner)?.palette(k);
                        let kmeans = Kmeans::run_image_par(image, centroids, options);
                        Some(kmeans.into_parallel_color_map())
                    })
                }
            }
        }
    }};
}

impl<'a> PipelineWithImageRefInput<'a> {
    /// Discard the dimensions and convert a [`PipelineWithImageRefInput`] to a
    /// [`PipelineWithSliceInput`].
    fn into_pipeline_with_slice_input(self) -> Option<PipelineWithSliceInput<'a>> {
        let Self { options, image, .. } = self;
        let colors = BoundedSlice::new(image.into_inner()).ok()?;
        Some(PipelineWithSliceInput { options, colors })
    }

    /// Runs the pipeline and returns the computed [`PaletteBuf<Srgb<u8>>`].
    ///
    /// Returns `None` if the input image was empty.
    #[must_use]
    pub fn output_srgb8_palette(self) -> Option<PaletteBuf<Srgb<u8>>> {
        self.into_pipeline_with_slice_input()
            .map(PipelineWithSliceInput::output_srgb8_palette)
    }

    /// Runs the pipeline and returns the computed [`PaletteBuf<Oklab>`].
    ///
    /// Returns `None` if the input image was empty.
    #[must_use]
    pub fn output_oklab_palette(self) -> Option<PaletteBuf<Oklab>> {
        self.into_pipeline_with_slice_input()
            .map(PipelineWithSliceInput::output_oklab_palette)
    }

    /// Runs the pipeline and returns the computed [`PaletteBuf<Srgb<u8>>`] alongside the number of
    /// pixels or samples assigned to each palette color.
    ///
    /// Returns `None` if the input image was empty.
    #[must_use]
    pub fn output_srgb8_palette_and_counts(
        self,
    ) -> Option<(PaletteBuf<Srgb<u8>>, PaletteBuf<u32>)> {
        self.into_pipeline_with_slice_input()
            .map(PipelineWithSliceInput::output_srgb8_palette_and_counts)
    }

    /// Runs the pipeline and returns the computed [`PaletteBuf<Oklab>`] alongside the number of
    /// pixels or samples assigned to each palette color.
    ///
    /// Returns `None` if the input image was empty.
    #[must_use]
    pub fn output_oklab_palette_and_counts(self) -> Option<(PaletteBuf<Oklab>, PaletteBuf<u32>)> {
        self.into_pipeline_with_slice_input()
            .map(PipelineWithSliceInput::output_oklab_palette_and_counts)
    }

    /// Runs the pipeline and returns the quantized [`IndexedImage<Srgb<u8>>`].
    #[must_use]
    pub fn output_srgb8_indexed_image(self) -> IndexedImage<Srgb<u8>> {
        self.output_oklab_indexed_image()
            .map(|palette| oklab_to_srgb8(&palette))
    }

    /// Runs the pipeline and returns the quantized [`IndexedImage<Oklab>`].
    #[must_use]
    pub fn output_oklab_indexed_image(self) -> IndexedImage<Oklab> {
        #[cfg(feature = "threads")]
        if self.options.parallel {
            return impl_pipeline_par!(
                self,
                remap_indexed_to_indexed_par,
                remap_image_to_indexed_par,
                IndexedImage<Oklab>
            )
            .unwrap_or_default();
        }

        impl_pipeline!(
            self,
            remap_indexed_to_indexed,
            remap_image_to_indexed,
            IndexedImage<Oklab>
        )
        .unwrap_or_default()
    }

    /// Runs the pipeline and returns the quantized [`ImageBuf<Srgb<u8>>`].
    #[must_use]
    pub fn output_srgb8_image(self) -> ImageBuf<Srgb<u8>> {
        fn remap_indexed(
            image: &IndexedImage<Oklab, u32>,
            ditherer: Option<FloydSteinberg>,
            color_map: impl IndexedColorMap<Oklab, Output = Oklab>,
        ) -> ImageBuf<Srgb<u8>> {
            let color_map = PaletteSubstitution::from_slice_mapping(color_map, oklab_to_srgb8);
            remap_indexed_to_image(image, ditherer, color_map)
        }

        fn remap_image(
            image: ImageRef<'_, Oklab>,
            ditherer: Option<FloydSteinberg>,
            color_map: impl IndexedColorMap<Oklab, Output = Oklab>,
        ) -> ImageBuf<Srgb<u8>> {
            let color_map = PaletteSubstitution::from_slice_mapping(color_map, oklab_to_srgb8);
            remap_image_to_image(image, ditherer, color_map)
        }

        #[cfg(feature = "threads")]
        if self.options.parallel {
            fn remap_indexed(
                image: &IndexedImage<Oklab, u32>,
                ditherer: Option<FloydSteinberg>,
                color_map: impl IndexedColorMap<Oklab, Output = Oklab> + Sync,
            ) -> ImageBuf<Srgb<u8>> {
                let color_map = PaletteSubstitution::from_slice_mapping(color_map, oklab_to_srgb8);
                remap_indexed_to_image_par(image, ditherer, color_map)
            }

            fn remap_image(
                image: ImageRef<'_, Oklab>,
                ditherer: Option<FloydSteinberg>,
                color_map: impl IndexedColorMap<Oklab, Output = Oklab> + Sync,
            ) -> ImageBuf<Srgb<u8>> {
                let color_map = PaletteSubstitution::from_slice_mapping(color_map, oklab_to_srgb8);
                remap_image_to_image_par(image, ditherer, color_map)
            }

            return impl_pipeline_par!(self, remap_indexed, remap_image, ImageBuf<Srgb<u8>>)
                .unwrap_or_default();
        }

        impl_pipeline!(self, remap_indexed, remap_image, ImageBuf<Srgb<u8>>).unwrap_or_default()
    }

    /// Runs the pipeline and returns the quantized [`ImageBuf<Oklab>`].
    #[must_use]
    pub fn output_oklab_image(self) -> ImageBuf<Oklab> {
        #[cfg(feature = "threads")]
        if self.options.parallel {
            return impl_pipeline_par!(
                self,
                remap_indexed_to_image_par,
                remap_image_to_image_par,
                ImageBuf<Oklab>
            )
            .unwrap_or_default();
        }

        impl_pipeline!(
            self,
            remap_indexed_to_image,
            remap_image_to_image,
            ImageBuf<Oklab>
        )
        .unwrap_or_default()
    }

    #[allow(clippy::type_complexity)]
    /// Runs the pipeline and returns the [`PaletteBuf<Srgb<u8>>`] and quantized [`ImageBuf<Srgb<u8>>`].
    ///
    /// Returns `None` if the input image was empty.
    #[must_use]
    pub fn output_srgb8_palette_and_image(
        self,
    ) -> Option<(PaletteBuf<Srgb<u8>>, ImageBuf<Srgb<u8>>)> {
        fn remap_indexed(
            image: &IndexedImage<Oklab, u32>,
            ditherer: Option<FloydSteinberg>,
            color_map: impl IndexedColorMap<Oklab, Output = Oklab>,
        ) -> (PaletteBuf<Srgb<u8>>, ImageBuf<Srgb<u8>>) {
            let color_map = PaletteSubstitution::from_slice_mapping(color_map, oklab_to_srgb8);
            let image = remap_indexed_to_image(image, ditherer, &color_map);
            (color_map.into_palette(), image)
        }

        fn remap_image(
            image: ImageRef<'_, Oklab>,
            ditherer: Option<FloydSteinberg>,
            color_map: impl IndexedColorMap<Oklab, Output = Oklab>,
        ) -> (PaletteBuf<Srgb<u8>>, ImageBuf<Srgb<u8>>) {
            let color_map = PaletteSubstitution::from_slice_mapping(color_map, oklab_to_srgb8);
            let image = remap_image_to_image(image, ditherer, &color_map);
            (color_map.into_palette(), image)
        }

        #[cfg(feature = "threads")]
        if self.options.parallel {
            fn remap_indexed(
                image: &IndexedImage<Oklab, u32>,
                ditherer: Option<FloydSteinberg>,
                color_map: impl IndexedColorMap<Oklab, Output = Oklab> + Sync,
            ) -> (PaletteBuf<Srgb<u8>>, ImageBuf<Srgb<u8>>) {
                let color_map = PaletteSubstitution::from_slice_mapping(color_map, oklab_to_srgb8);
                let image = remap_indexed_to_image_par(image, ditherer, &color_map);
                (color_map.into_palette(), image)
            }

            fn remap_image(
                image: ImageRef<'_, Oklab>,
                ditherer: Option<FloydSteinberg>,
                color_map: impl IndexedColorMap<Oklab, Output = Oklab> + Sync,
            ) -> (PaletteBuf<Srgb<u8>>, ImageBuf<Srgb<u8>>) {
                let color_map = PaletteSubstitution::from_slice_mapping(color_map, oklab_to_srgb8);
                let image = remap_image_to_image_par(image, ditherer, &color_map);
                (color_map.into_palette(), image)
            }

            return impl_pipeline_par!(
                self,
                remap_indexed,
                remap_image,
                (PaletteBuf<Srgb<u8>>, ImageBuf<Srgb<u8>>)
            );
        }

        impl_pipeline!(
            self,
            remap_indexed,
            remap_image,
            (PaletteBuf<Srgb<u8>>, ImageBuf<Srgb<u8>>)
        )
    }

    /// Runs the pipeline and returns the [`PaletteBuf<Oklab>`] and quantized [`ImageBuf<Oklab>`].
    ///
    /// Returns `None` if the input image was empty.
    #[must_use]
    pub fn output_oklab_palette_and_image(self) -> Option<(PaletteBuf<Oklab>, ImageBuf<Oklab>)> {
        fn remap_indexed(
            image: &IndexedImage<Oklab, u32>,
            ditherer: Option<FloydSteinberg>,
            color_map: impl IndexedColorMap<Oklab, Output = Oklab>,
        ) -> (PaletteBuf<Oklab>, ImageBuf<Oklab>) {
            let image = remap_indexed_to_image(image, ditherer, &color_map);
            (color_map.into_palette(), image)
        }

        fn remap_image(
            image: ImageRef<'_, Oklab>,
            ditherer: Option<FloydSteinberg>,
            color_map: impl IndexedColorMap<Oklab, Output = Oklab>,
        ) -> (PaletteBuf<Oklab>, ImageBuf<Oklab>) {
            let image = remap_image_to_image(image, ditherer, &color_map);
            (color_map.into_palette(), image)
        }

        #[cfg(feature = "threads")]
        if self.options.parallel {
            fn remap_indexed(
                image: &IndexedImage<Oklab, u32>,
                ditherer: Option<FloydSteinberg>,
                color_map: impl IndexedColorMap<Oklab, Output = Oklab> + Sync,
            ) -> (PaletteBuf<Oklab>, ImageBuf<Oklab>) {
                let image = remap_indexed_to_image_par(image, ditherer, &color_map);
                (color_map.into_palette(), image)
            }

            fn remap_image(
                image: ImageRef<'_, Oklab>,
                ditherer: Option<FloydSteinberg>,
                color_map: impl IndexedColorMap<Oklab, Output = Oklab> + Sync,
            ) -> (PaletteBuf<Oklab>, ImageBuf<Oklab>) {
                let image = remap_image_to_image_par(image, ditherer, &color_map);
                (color_map.into_palette(), image)
            }

            return impl_pipeline_par!(
                self,
                remap_indexed,
                remap_image,
                (PaletteBuf<Oklab>, ImageBuf<Oklab>)
            );
        }

        impl_pipeline!(
            self,
            remap_indexed,
            remap_image,
            (PaletteBuf<Oklab>, ImageBuf<Oklab>)
        )
    }
}

fn remap_image_to_image<ColorMap>(
    image: ImageRef<'_, Oklab>,
    ditherer: Option<FloydSteinberg>,
    color_map: ColorMap,
) -> ImageBuf<ColorMap::Output>
where
    ColorMap: IndexedColorMap<Oklab>,
    ColorMap::Output: Zeroable,
{
    if let Some(ditherer) = ditherer {
        ditherer.dither_image_to_image(image, color_map)
    } else {
        image.map_to_image(color_map)
    }
}

fn remap_indexed_to_image<ColorMap>(
    image: &IndexedImage<Oklab, u32>,
    ditherer: Option<FloydSteinberg>,
    color_map: ColorMap,
) -> ImageBuf<ColorMap::Output>
where
    ColorMap: IndexedColorMap<Oklab>,
    ColorMap::Output: Zeroable,
{
    if let Some(ditherer) = ditherer {
        ditherer.dither_indexed_to_image(image, color_map)
    } else {
        image.map_to_image(color_map)
    }
}

fn remap_indexed_to_indexed(
    image: &IndexedImage<Oklab, u32>,
    ditherer: Option<FloydSteinberg>,
    color_map: impl IndexedColorMap<Oklab, Output = Oklab>,
) -> IndexedImage<Oklab> {
    if let Some(ditherer) = ditherer {
        ditherer.dither_indexed_to_indexed(image, color_map)
    } else {
        image.map_to_indexed(color_map)
    }
}

fn remap_image_to_indexed(
    image: ImageRef<'_, Oklab>,
    ditherer: Option<FloydSteinberg>,
    color_map: impl IndexedColorMap<Oklab, Output = Oklab>,
) -> IndexedImage<Oklab> {
    if let Some(ditherer) = ditherer {
        ditherer.dither_image_to_indexed(image, color_map)
    } else {
        image.map_to_indexed(color_map)
    }
}

#[cfg(feature = "threads")]
fn remap_image_to_image_par<ColorMap>(
    image: ImageRef<'_, Oklab>,
    ditherer: Option<FloydSteinberg>,
    color_map: ColorMap,
) -> ImageBuf<ColorMap::Output>
where
    ColorMap: IndexedColorMap<Oklab> + Sync,
    ColorMap::Output: Zeroable,
{
    if let Some(ditherer) = ditherer {
        ditherer.dither_image_to_image_par(image, color_map)
    } else {
        image.map_to_image(color_map)
    }
}

#[cfg(feature = "threads")]
fn remap_indexed_to_image_par<ColorMap>(
    image: &IndexedImage<Oklab, u32>,
    ditherer: Option<FloydSteinberg>,
    color_map: ColorMap,
) -> ImageBuf<ColorMap::Output>
where
    ColorMap: IndexedColorMap<Oklab> + Sync,
    ColorMap::Output: Zeroable,
{
    if let Some(ditherer) = ditherer {
        ditherer.dither_indexed_to_image_par(image, color_map)
    } else {
        image.map_to_image_par(color_map)
    }
}

#[cfg(feature = "threads")]
fn remap_indexed_to_indexed_par(
    image: &IndexedImage<Oklab, u32>,
    ditherer: Option<FloydSteinberg>,
    color_map: impl IndexedColorMap<Oklab, Output = Oklab> + Sync,
) -> IndexedImage<Oklab> {
    if let Some(ditherer) = ditherer {
        ditherer.dither_indexed_to_indexed_par(image, color_map)
    } else {
        image.map_to_indexed_par(color_map)
    }
}

#[cfg(feature = "threads")]
fn remap_image_to_indexed_par(
    image: ImageRef<'_, Oklab>,
    ditherer: Option<FloydSteinberg>,
    color_map: impl IndexedColorMap<Oklab, Output = Oklab> + Sync,
) -> IndexedImage<Oklab> {
    if let Some(ditherer) = ditherer {
        ditherer.dither_image_to_indexed_par(image, color_map)
    } else {
        image.map_to_indexed(color_map)
    }
}
