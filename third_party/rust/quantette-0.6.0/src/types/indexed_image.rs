use crate::{BoundedIndex, ImageBuf, IndexedColorMap, MAX_PIXELS, PaletteCounts};
use alloc::{borrow::ToOwned as _, vec, vec::Vec};
use core::{
    error::Error,
    fmt::{self, Debug},
};
use num_traits::AsPrimitive;
#[cfg(feature = "threads")]
use rayon::prelude::*;

/// The error returned when an [`IndexedImage`] failed to be created.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CreateIndexedImageError<Color, Index = u8> {
    /// The provided image width.
    width: u32,
    /// The provided image height.
    height: u32,
    /// The provided image palette.
    palette: Vec<Color>,
    /// The provided image indices.
    indices: Vec<Index>,
}
impl<Color, Index> CreateIndexedImageError<Color, Index> {
    /// Returns a reference to the palette [`Vec`] that was used to try and create the [`IndexedImage`].
    #[inline]
    pub fn palette(&self) -> &[Color] {
        &self.palette
    }

    /// Returns a reference to the indices [`Vec`] that was used to try and create the [`IndexedImage`].
    #[inline]
    pub fn indices(&self) -> &[Index] {
        &self.indices
    }

    /// Returns the palette and indices that were used to try and create the [`IndexedImage`].
    #[must_use]
    #[inline]
    pub fn into_parts(self) -> (Vec<Color>, Vec<Index>) {
        let Self { palette, indices, .. } = self;
        (palette, indices)
    }
}

impl<Color, Index> fmt::Display for CreateIndexedImageError<Color, Index> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let Self { width, height, .. } = *self;
        if width.checked_mul(height).is_some() {
            write!(
                f,
                "image dimensions of ({width}, {height}) do not match the indices length of {}",
                self.indices.len(),
            )
        } else {
            write!(
                f,
                "image dimensions of ({width}, {height}) are above the maximum number of pixels of {MAX_PIXELS}",
            )
        }
    }
}

impl<Color: Debug, Index: Debug> Error for CreateIndexedImageError<Color, Index> {}

/// An image represented as a palette of colors and a series of indices into that palette.
///
/// This type consists of a width, a height, a palette buffer, and an indices buffer.
/// The indices buffer is in row-major order where each index corresponds to one pixel and
/// references one of the palette colors. The length of the indices [`Vec`] is guaranteed to match
/// `width * height` and be less than or equal to [`MAX_PIXELS`].
///
/// The default index type is `u8`, but may also be `u16` or `u32`. Note that `u16` or `u32` may
/// not be fully supported by other functions throughout the crate.
///
/// # Examples
///
/// Directly creating an [`IndexedImage`] from a [`Vec`] of palette colors and indices:
///
/// ```
/// # use quantette::{IndexedImage, CreateIndexedImageError};
/// # use palette::Srgb;
/// # fn main() -> Result<(), CreateIndexedImageError<Srgb<u8>>> {
/// let (width, height) = (512, 512);
/// let palette = vec![Srgb::new(0, 0, 0)];
/// let indices = vec![0; (width * height) as usize];
/// let image = IndexedImage::new(width, height, palette, indices)?;
/// # Ok(())
/// # }
/// ```
#[derive(Clone, Debug, PartialEq, Eq, Hash)]
pub struct IndexedImage<Color, Index = u8> {
    /// The width of the image.
    width: u32,
    /// The height of the image.
    height: u32,
    /// The palette color of the image.
    palette: Vec<Color>,
    /// The indices into `palette` for each pixel of the image.
    indices: Vec<Index>,
}

impl<Color, Index: BoundedIndex> IndexedImage<Color, Index> {
    /// Create a new [`IndexedImage`] without validating invariants.
    pub(crate) fn new_unchecked(
        width: u32,
        height: u32,
        palette: Vec<Color>,
        indices: Vec<Index>,
    ) -> Self {
        debug_assert_eq!(
            width.checked_mul(height).map(|len| len as usize),
            Some(indices.len()),
        );
        debug_assert!(indices.iter().all(|i| i.as_() < palette.len()));
        Self { width, height, palette, indices }
    }

    /// Create a new [`IndexedImage`] from a `palette` of colors and `indices` into the `palette`.
    ///
    /// # Errors
    ///
    /// The provided `palette` and `indices` are returned as an `Err` if any of the following are true:
    /// - The length of `indices` and `width * height` do not match.
    /// - `width * height` overflows a `u32`.
    ///
    /// Note that this function does not validate that all indices in `indices`
    /// are less than `palette.len()`.
    pub fn new(
        width: u32,
        height: u32,
        palette: Vec<Color>,
        indices: Vec<Index>,
    ) -> Result<Self, CreateIndexedImageError<Color, Index>> {
        if width.checked_mul(height).map(|len| len as usize) == Some(indices.len()) {
            Ok(Self::new_unchecked(width, height, palette, indices))
        } else {
            Err(CreateIndexedImageError { width, height, palette, indices })
        }
    }

    /// Returns the width and height of the [`IndexedImage`].
    #[inline]
    pub fn dimensions(&self) -> (u32, u32) {
        (self.width, self.height)
    }

    /// Returns the width the [`IndexedImage`].
    #[inline]
    pub fn width(&self) -> u32 {
        self.width
    }

    /// Returns the height of the [`IndexedImage`].
    #[inline]
    pub fn height(&self) -> u32 {
        self.height
    }

    /// Returns whether the [`IndexedImage`] has zero pixels.
    #[inline]
    pub fn is_empty(&self) -> bool {
        self.indices.is_empty()
    }

    /// Returns the number of pixels in the [`IndexedImage`] specified by `width * height`.
    #[allow(clippy::cast_possible_truncation)]
    #[inline]
    pub fn num_pixels(&self) -> u32 {
        self.indices.len() as u32
    }

    /// Returns a slice of the palette colors of the [`IndexedImage`].
    #[inline]
    pub fn palette(&self) -> &[Color] {
        &self.palette
    }

    /// Returns a mutable slice of the palette colors of the [`IndexedImage`].
    #[inline]
    pub fn palette_mut(&mut self) -> &mut [Color] {
        &mut self.palette
    }

    /// Returns a slice of the indices of the [`IndexedImage`].
    #[inline]
    pub fn indices(&self) -> &[Index] {
        &self.indices
    }

    /// Returns a mutable slice of the indices of the [`IndexedImage`].
    #[inline]
    pub fn indices_mut(&mut self) -> &mut [Index] {
        &mut self.indices
    }

    /// Consume an [`IndexedImage`] and return the inner palette [`Vec`] and indices [`Vec`].
    #[must_use]
    #[inline]
    pub fn into_parts(self) -> (Vec<Color>, Vec<Index>) {
        let Self { palette, indices, .. } = self;
        (palette, indices)
    }

    /// Create a new [`IndexedImage`] with a larger index type.
    ///
    /// See also [`into_larger_indices`](Self::into_larger_indices) for a consuming variant
    /// that does not need to clone the palette.
    #[must_use]
    pub fn to_larger_indices<NewIndex>(&self) -> IndexedImage<Color, NewIndex>
    where
        Color: Clone,
        NewIndex: BoundedIndex,
        Index: Into<NewIndex>,
    {
        let Self { width, height, palette, indices, .. } = self;
        IndexedImage::new_unchecked(
            *width,
            *height,
            palette.clone(),
            indices.iter().map(|&i| i.into()).collect(),
        )
    }

    /// Create a new [`IndexedImage`] with a smaller index type.
    ///
    /// See also [`into_smaller_indices`](Self::into_smaller_indices) for a consuming variant
    /// that does not need to clone the palette.
    ///
    /// This returns `None` if the length of the palette is larger than `NewIndex::MAX + 1`.
    #[must_use]
    pub fn to_smaller_indices<NewIndex>(&self) -> Option<IndexedImage<Color, NewIndex>>
    where
        Color: Clone,
        NewIndex: BoundedIndex,
        Index: AsPrimitive<NewIndex>,
    {
        let Self { width, height, palette, indices, .. } = self;
        (palette.len() <= NewIndex::MAX_LEN).then(|| {
            let palette = palette.to_owned();
            let indices = indices.iter().map(|i| i.as_()).collect();
            IndexedImage::new_unchecked(*width, *height, palette, indices)
        })
    }

    /// Convert the indices of an [`IndexedImage`] to a larger integer type.
    ///
    /// See [`to_larger_indices`](Self::to_larger_indices) for a non-consuming variant.
    #[must_use]
    pub fn into_larger_indices<NewIndex>(self) -> IndexedImage<Color, NewIndex>
    where
        NewIndex: BoundedIndex,
        Index: Into<NewIndex>,
    {
        let Self { width, height, palette, indices, .. } = self;
        IndexedImage::new_unchecked(
            width,
            height,
            palette,
            indices.into_iter().map(Into::into).collect(),
        )
    }

    /// Convert the indices of an [`IndexedImage`] to a smaller integer type.
    ///
    /// See [`to_smaller_indices`](Self::to_smaller_indices) for a non-consuming variant.
    ///
    /// # Errors
    ///
    /// This returns the original [`IndexedImage`] as an `Err` if the length of the palette is
    /// larger than `NewIndex::MAX + 1`.
    ///
    pub fn into_smaller_indices<NewIndex>(self) -> Result<IndexedImage<Color, NewIndex>, Self>
    where
        NewIndex: BoundedIndex,
        Index: AsPrimitive<NewIndex>,
    {
        if self.palette.len() <= NewIndex::MAX_LEN {
            let Self { width, height, palette, indices, .. } = self;
            let indices = indices.into_iter().map(AsPrimitive::as_).collect();
            Ok(IndexedImage::new_unchecked(width, height, palette, indices))
        } else {
            Err(self)
        }
    }

    /// Replace the palette of an [`IndexedImage`] to a different color type (or to new colors
    /// of the same type).
    ///
    /// # Errors
    ///
    /// If the length of the provided `palette` does not match the length of the current palette
    /// in the [`IndexedImage`], then `self` and `palette` are returned as an `Err`. Otherwise,
    /// the new [`IndexedImage`] is returned alongside the old palette.
    #[allow(clippy::type_complexity)]
    #[inline]
    pub fn replace_palette<NewColor>(
        self,
        palette: Vec<NewColor>,
    ) -> Result<(IndexedImage<NewColor, Index>, Vec<Color>), (Self, Vec<NewColor>)> {
        if self.palette.len() == palette.len() {
            let Self {
                width,
                height,
                indices,
                palette: old_palette,
                ..
            } = self;
            Ok((
                IndexedImage::new_unchecked(width, height, palette, indices),
                old_palette,
            ))
        } else {
            Err((self, palette))
        }
    }

    /// Map the palette of an [`IndexedImage`], reusing the existing `indices`.
    ///
    /// See [`map_ref`](IndexedImage::map_ref) to instead clone the existing `indices`
    /// and retain the original [`IndexedImage`].
    ///
    /// Also see [`map_to_indexed`](IndexedImage::map_to_indexed) to map an [`IndexedImage`] to a
    /// new palette with a potentially different number of colors.
    ///
    /// Rather than being a function from `Color -> NewColor`, `mapping` takes the whole palette
    /// as input and returns a new palette. This is to allow batch or parallel mappings.
    ///
    /// # Examples
    ///
    /// It is recommended to do batch mappings for efficiency where it makes sense. E.g., using the
    /// color space conversion functions from the [`color_space`](crate::color_space) module.
    ///
    /// ```
    /// # use quantette::IndexedImage;
    /// # use palette::{Srgb, LinSrgb};
    /// use quantette::color_space::srgb8_to_oklab;
    /// let srgb_image = IndexedImage::<Srgb<u8>>::default();
    /// let oklab_image = srgb_image.map(|palette| srgb8_to_oklab(&palette));
    /// ```
    ///
    /// To instead map each color one at a time, use `into_iter`, `map`, and `collect` like normal:
    ///
    /// ```
    /// # use quantette::IndexedImage;
    /// # use palette::{Srgb, LinSrgb};
    /// let srgb_image = IndexedImage::<Srgb<u8>>::default();
    /// let lin_srgb_image: IndexedImage<LinSrgb> =
    ///     srgb_image.map(|palette| palette.into_iter().map(|srgb| srgb.into_linear()).collect());
    /// ```
    ///
    /// # Panics
    ///
    /// Panics if `mapping` returns a palette with a different length than the original palette.
    #[must_use]
    #[inline]
    pub fn map<NewColor>(
        self,
        mapping: impl FnOnce(Vec<Color>) -> Vec<NewColor>,
    ) -> IndexedImage<NewColor, Index> {
        let Self { width, height, palette, indices, .. } = self;
        let len = palette.len();
        let palette = mapping(palette);
        assert_eq!(palette.len(), len);
        IndexedImage::new_unchecked(width, height, palette, indices)
    }

    /// Map the palette of an [`IndexedImage`], cloning the existing `indices`.
    ///
    /// See [`map`](IndexedImage::map) to instead consume the original
    /// [`IndexedImage`] and avoid a clone of the `indices`.
    ///
    /// Also see [`map_to_indexed`](IndexedImage::map_to_indexed) to map an [`IndexedImage`] to a
    /// new palette with a potentially different number of colors.
    ///
    /// Rather than being a function from `Color -> NewColor`, `mapping` takes the whole palette
    /// as input and returns a new palette. This is to allow batch or parallel mappings.
    ///
    /// # Examples
    ///
    /// It is recommended to do batch mappings for efficiency where it makes sense. E.g., using the
    /// color space conversion functions from the [`color_space`](crate::color_space) module.
    ///
    /// ```
    /// # use quantette::IndexedImage;
    /// # use palette::{Srgb, LinSrgb};
    /// use quantette::color_space::srgb8_to_oklab;
    /// let srgb_image = IndexedImage::<Srgb<u8>>::default();
    /// let oklab_image = srgb_image.map_ref(srgb8_to_oklab);
    /// ```
    ///
    /// To instead map each color one at a time, use `iter`, `map`, and `collect` like normal:
    ///
    /// ```
    /// # use quantette::IndexedImage;
    /// # use palette::{Srgb, LinSrgb};
    /// let srgb_image = IndexedImage::<Srgb<u8>>::default();
    /// let lin_srgb_image: IndexedImage<LinSrgb> =
    ///     srgb_image.map_ref(|palette| palette.iter().map(|srgb| srgb.into_linear()).collect());
    /// ```
    ///
    /// # Panics
    ///
    /// Panics if `mapping` returns a palette with a different length than the original palette.
    #[must_use]
    #[inline]
    pub fn map_ref<NewColor>(
        &self,
        mapping: impl FnOnce(&[Color]) -> Vec<NewColor>,
    ) -> IndexedImage<NewColor, Index> {
        let len = self.palette().len();
        let palette = mapping(self.palette());
        assert_eq!(palette.len(), len);
        IndexedImage::new_unchecked(self.width, self.height, palette, self.indices.clone())
    }

    /// Map an [`IndexedImage`] to a new palette using the provided [`IndexedColorMap`].
    ///
    /// This function recalculates the `indices` for the new [`IndexedImage`], as the new palette
    /// may have a different length. To map only the palette colors see one of the following:
    /// - [`map_ref`](IndexedImage::map_ref)
    /// - [`map`](IndexedImage::map)
    /// - [`replace_palette`](IndexedImage::replace_palette)
    ///
    /// # Panics
    ///
    /// Panics if `color_map` is not a valid implementor of [`IndexedColorMap`]. That is, it returns
    /// a [`Vec`] with a different length than the input slice.
    #[must_use]
    pub fn map_to_indexed<ColorMap: IndexedColorMap<Color>>(
        &self,
        color_map: ColorMap,
    ) -> IndexedImage<ColorMap::Output> {
        let indices = color_map.map_to_indices(&self.palette);
        assert_eq!(indices.len(), self.palette.len());
        let indices = {
            let indices = indices.as_slice(); // faster for some reason
            self.indices.iter().map(|i| indices[i.as_()]).collect()
        };
        IndexedImage::new_unchecked(
            self.width,
            self.height,
            color_map.into_palette().into_vec(),
            indices,
        )
    }

    #[cfg(feature = "threads")]
    /// Map an [`IndexedImage`] to a new palette in parallel using the provided [`IndexedColorMap`].
    ///
    /// This function recalculates the `indices` for the new [`IndexedImage`], as the new palette
    /// may have a different length. To map only the palette colors see one of the following:
    /// - [`map_ref`](IndexedImage::map_ref)
    /// - [`map`](IndexedImage::map)
    /// - [`replace_palette`](IndexedImage::replace_palette)
    ///
    /// # Panics
    ///
    /// Panics if `color_map` is not a valid implementor of [`IndexedColorMap`]. That is, it returns
    /// a [`Vec`] with a different length than the input slice.
    #[must_use]
    pub fn map_to_indexed_par<ColorMap: IndexedColorMap<Color>>(
        &self,
        color_map: ColorMap,
    ) -> IndexedImage<ColorMap::Output> {
        let indices = color_map.map_to_indices(&self.palette);
        assert_eq!(indices.len(), self.palette.len());
        let indices = {
            let indices = indices.as_slice(); // faster for some reason
            self.indices.par_iter().map(|i| indices[i.as_()]).collect()
        };
        IndexedImage::new_unchecked(
            self.width,
            self.height,
            color_map.into_palette().into_vec(),
            indices,
        )
    }

    /// Convert an [`IndexedImage`] to an [`ImageBuf`] while also mapping the palette colors.
    ///
    /// This is more efficient than [`map_ref`](Self::map_ref) followed by
    /// [`to_image`](Self::to_image), since it avoids unnecessary intermediate `indices`.
    ///
    /// # Panics
    ///
    /// Panics if `color_map` is not a valid implementor of [`IndexedColorMap`]. That is, it returns
    /// a [`Vec`] with a different length than the input slice.
    #[must_use]
    pub fn map_to_image<ColorMap: IndexedColorMap<Color>>(
        &self,
        color_map: ColorMap,
    ) -> ImageBuf<ColorMap::Output> {
        let Self { width, height, .. } = *self;
        let palette = color_map.map_to_colors(&self.palette);
        assert_eq!(palette.len(), self.palette.len());
        let palette = palette.as_slice();
        let pixels = self
            .indices
            .iter()
            .map(|i| palette[i.as_()].clone())
            .collect();
        ImageBuf::new_unchecked(width, height, pixels)
    }

    #[cfg(feature = "threads")]
    /// Convert an [`IndexedImage`] to an [`ImageBuf`] in parallel while also mapping the palette colors.
    ///
    /// This is more efficient than [`map_ref`](Self::map_ref) followed by
    /// [`to_image_par`](Self::to_image_par), since it avoids unnecessary intermediate `indices`.
    ///
    /// # Panics
    ///
    /// Panics if `color_map` is not a valid implementor of [`IndexedColorMap`]. That is, it returns
    /// a [`Vec`] with a different length than the input slice.
    #[must_use]
    pub fn map_to_image_par<ColorMap: IndexedColorMap<Color>>(
        &self,
        color_map: ColorMap,
    ) -> ImageBuf<ColorMap::Output> {
        let Self { width, height, .. } = *self;
        let palette = color_map.map_to_colors(&self.palette);
        assert_eq!(palette.len(), self.palette.len());
        let palette = palette.as_slice();
        let pixels = self
            .indices
            .par_iter()
            .map(|i| palette[i.as_()].clone())
            .collect();
        ImageBuf::new_unchecked(width, height, pixels)
    }

    /// Convert an [`IndexedImage`] to an [`IndexedImageCounts`] by counting the number of times
    /// each palette index appears.
    #[must_use]
    pub fn into_indexed_image_counts(self) -> IndexedImageCounts<Color, Index> {
        let mut counts = vec![0; self.palette.len()];
        for i in &self.indices {
            counts[i.as_()] += 1;
        }
        IndexedImageCounts { image: self, counts }
    }

    /// Convert an [`IndexedImage`] to an [`ImageBuf`].
    ///
    /// See [`map_to_image`](Self::map_to_image) if you also need to map the palette colors.
    #[must_use]
    pub fn to_image(&self) -> ImageBuf<Color>
    where
        Color: Clone,
    {
        let Self { width, height, .. } = *self;
        let palette = self.palette.as_slice();
        let pixels = self
            .indices
            .iter()
            .map(|i| palette[i.as_()].clone())
            .collect();
        ImageBuf::new_unchecked(width, height, pixels)
    }

    #[cfg(feature = "threads")]
    /// Convert an [`IndexedImage`] to an [`ImageBuf`] in parallel.
    ///
    /// See [`map_to_image_par`](Self::map_to_image_par) if you also need to map the palette colors.
    #[must_use]
    pub fn to_image_par(&self) -> ImageBuf<Color>
    where
        Color: Clone + Send + Sync,
    {
        let Self { width, height, .. } = *self;
        let palette = self.palette.as_slice();
        let pixels = self
            .indices
            .par_iter()
            .map(|i| palette[i.as_()].clone())
            .collect();
        ImageBuf::new_unchecked(width, height, pixels)
    }
}

impl<Color, Index: BoundedIndex> Default for IndexedImage<Color, Index> {
    #[inline]
    fn default() -> Self {
        Self::new_unchecked(0, 0, Vec::new(), Vec::new())
    }
}

/// An image represented as a palette of colors and a series of indices into that palette.
/// This type also stores the number of times each palette color is referenced.
///
/// This type consists of a width, a height, a palette buffer, an indices buffer, and a counts buffer.
/// The indices buffer is in row-major order where each index corresponds to one pixel and
/// references one of the palette colors. The length of the indices [`Vec`] is guaranteed to match
/// `width * height` and be less than or equal to [`MAX_PIXELS`]. Similarly, the length of the
/// palette is guaranteed to be the same length as the counts buffer.
///
/// The default index type is `u8`, but may also be `u16` or `u32`. Note that `u16` or `u32` may
/// not be fully supported by other functions throughout the crate.
#[derive(Clone, Debug, PartialEq, Eq, Hash)]
pub struct IndexedImageCounts<Color, Index = u8> {
    /// The underlying [`IndexedImage`].
    image: IndexedImage<Color, Index>,
    /// The counts corresponding to each palette color.
    counts: Vec<u32>,
}

impl<Color, Index: BoundedIndex> IndexedImageCounts<Color, Index> {
    /// Create a new [`IndexedImageCounts`] without validating invariants.
    #[inline]
    pub(crate) fn new_unchecked(
        width: u32,
        height: u32,
        palette: Vec<Color>,
        counts: Vec<u32>,
        indices: Vec<Index>,
    ) -> Self {
        let image = IndexedImage::new_unchecked(width, height, palette, indices);
        Self::from_indexed_image_unchecked(image, counts)
    }

    /// Create a new [`IndexedImageCounts`] from a [`PaletteCounts`] without validating invariants.
    #[inline]
    pub(crate) fn from_palette_counts_unchecked(
        width: u32,
        height: u32,
        palette_counts: PaletteCounts<Color>,
        indices: Vec<Index>,
    ) -> Self {
        let (palette, counts) = palette_counts.into_parts();
        Self::new_unchecked(width, height, palette, counts, indices)
    }

    /// Create a new [`IndexedImageCounts`] from an [`IndexedImage`] without validating invariants.
    #[inline]
    pub(crate) fn from_indexed_image_unchecked(
        image: IndexedImage<Color, Index>,
        counts: Vec<u32>,
    ) -> Self {
        debug_assert_eq!(image.palette.len(), counts.len());
        debug_assert_eq!(
            image.indices().len(),
            counts.iter().copied().sum::<u32>() as usize,
        );
        Self { image, counts }
    }

    /// Returns the width and height of the [`IndexedImageCounts`].
    #[inline]
    pub fn dimensions(&self) -> (u32, u32) {
        self.image.dimensions()
    }

    /// Returns the width of the [`IndexedImageCounts`].
    #[inline]
    pub fn width(&self) -> u32 {
        self.image.width()
    }

    /// Returns the height of the [`IndexedImageCounts`].
    #[inline]
    pub fn height(&self) -> u32 {
        self.image.height()
    }

    /// Returns whether the [`IndexedImageCounts`] has zero pixels.
    #[inline]
    pub fn is_empty(&self) -> bool {
        self.image.is_empty()
    }

    /// Returns the number of pixels in the [`IndexedImageCounts`] specified by `width * height`.
    #[inline]
    pub fn num_pixels(&self) -> u32 {
        self.image.num_pixels()
    }

    /// Returns a reference to an [`IndexedImage`] from a reference to an [`IndexedImageCounts`].
    #[inline]
    pub fn as_indexed_image(&self) -> &IndexedImage<Color, Index> {
        &self.image
    }

    /// Returns a slice of the palette colors of the [`IndexedImageCounts`].
    #[inline]
    pub fn palette(&self) -> &[Color] {
        self.image.palette()
    }

    /// Returns a mutable slice of the palette colors of the [`IndexedImageCounts`].
    #[inline]
    pub fn palette_mut(&mut self) -> &mut [Color] {
        self.image.palette_mut()
    }

    /// Returns a slice of the inner counts of the [`IndexedImageCounts`].
    #[inline]
    pub fn counts(&self) -> &[u32] {
        &self.counts
    }

    /// Returns a slice of the indices of the [`IndexedImageCounts`].
    #[inline]
    pub fn indices(&self) -> &[Index] {
        self.image.indices()
    }

    /// Returns the sum of the [`counts`](Self::counts) for all palette colors.
    ///
    /// This operation is `O(1)` and is the same as [`num_pixels`](Self::num_pixels).
    #[inline]
    pub fn total_count(&self) -> u32 {
        self.image.num_pixels()
    }

    /// Consume an [`IndexedImageCounts`] and convert it to an [`IndexedImage`].
    #[must_use]
    #[inline]
    pub fn into_indexed_image(self) -> IndexedImage<Color, Index> {
        self.image
    }

    /// Consume an [`IndexedImageCounts`] and convert it to a [`PaletteCounts`].
    #[must_use]
    #[inline]
    pub fn into_palette_counts(self) -> PaletteCounts<Color> {
        let total_count = self.total_count();
        let Self { image, counts, .. } = self;
        PaletteCounts::new_unchecked(image.palette, counts, total_count)
    }

    /// Consume an [`IndexedImageCounts`] and return the inner [`IndexedImage`]
    /// and [`Vec`] of counts.
    #[must_use]
    #[inline]
    pub fn into_parts(self) -> (IndexedImage<Color, Index>, Vec<u32>) {
        let Self { image, counts, .. } = self;
        (image, counts)
    }

    /// Replace the palette of an [`IndexedImageCounts`] to a different color type (or to new colors
    /// of the same type).
    ///
    /// # Errors
    ///
    /// If the length of the provided `palette` does not match the length of the current palette
    /// in the [`IndexedImageCounts`], then `self` and `palette` are returned as an `Err`.
    /// Otherwise, the new [`IndexedImageCounts`] is returned alongside the old palette.
    #[allow(clippy::type_complexity)]
    #[inline]
    pub fn replace_palette<NewColor>(
        self,
        palette: Vec<NewColor>,
    ) -> Result<(IndexedImageCounts<NewColor, Index>, Vec<Color>), (Self, Vec<NewColor>)> {
        if self.palette().len() == palette.len() {
            let IndexedImage {
                width,
                height,
                palette: old_palette,
                indices,
                ..
            } = self.image;
            Ok((
                IndexedImageCounts::new_unchecked(width, height, palette, self.counts, indices),
                old_palette,
            ))
        } else {
            Err((self, palette))
        }
    }

    /// Map the palette of an [`IndexedImageCounts`], reusing the existing `indices` and `counts`.
    ///
    /// See [`map_ref`](IndexedImageCounts::map_ref) to instead clone the existing `indices` and
    /// `counts` while retaining the original [`IndexedImageCounts`].
    ///
    /// Also see [`map_to_indexed`](IndexedImageCounts::map_to_indexed) to map an
    /// [`IndexedImageCounts`] to a new palette with a potentially different number of colors.
    ///
    /// Rather than being a function from `Color -> NewColor`, `mapping` takes the whole palette
    /// as input and returns a new palette. This is to allow batch or parallel mappings.
    ///
    /// # Examples
    ///
    /// It is recommended to do batch mappings for efficiency where it makes sense. E.g., using the
    /// color space conversion functions from the [`color_space`](crate::color_space) module.
    ///
    /// ```
    /// # use quantette::IndexedImageCounts;
    /// # use palette::{Srgb, LinSrgb};
    /// use quantette::color_space::srgb8_to_oklab;
    /// let srgb_counts = IndexedImageCounts::<Srgb<u8>>::default();
    /// let oklab_counts = srgb_counts.map(|palette| srgb8_to_oklab(&palette));
    /// ```
    ///
    /// To instead map each color one at a time, use `into_iter`, `map`, and `collect` like normal:
    ///
    /// ```
    /// # use quantette::IndexedImageCounts;
    /// # use palette::{Srgb, LinSrgb};
    /// let srgb_counts = IndexedImageCounts::<Srgb<u8>>::default();
    /// let lin_srgb_counts: IndexedImageCounts<LinSrgb> =
    ///     srgb_counts.map(|palette| palette.into_iter().map(|srgb| srgb.into_linear()).collect());
    /// ```
    ///
    /// # Panics
    ///
    /// Panics if `mapping` returns a palette with a different length than the original palette.
    #[must_use]
    #[inline]
    pub fn map<NewColor>(
        self,
        mapping: impl FnOnce(Vec<Color>) -> Vec<NewColor>,
    ) -> IndexedImageCounts<NewColor, Index> {
        let Self { image, counts } = self;
        let image = image.map(mapping);
        IndexedImageCounts::from_indexed_image_unchecked(image, counts)
    }

    /// Map the palette of an [`IndexedImageCounts`], cloning the existing `indices` and `counts`.
    ///
    /// See [`map`](IndexedImageCounts::map) to instead consume the original [`IndexedImageCounts`]
    /// and avoid a clone of the `indices` and `counts`.
    ///
    /// Also see [`map_to_indexed`](IndexedImageCounts::map_to_indexed) to map an
    /// [`IndexedImageCounts`] to a new palette with a potentially different number of colors.
    ///
    /// Rather than being a function from `Color -> NewColor`, `mapping` takes the whole palette
    /// as input and returns a new palette. This is to allow batch or parallel mappings.
    ///
    /// # Examples
    ///
    /// It is recommended to do batch mappings for efficiency where it makes sense. E.g., using the
    /// color space conversion functions from the [`color_space`](crate::color_space) module.
    ///
    /// ```
    /// # use quantette::IndexedImageCounts;
    /// # use palette::{Srgb, LinSrgb};
    /// use quantette::color_space::srgb8_to_oklab;
    /// let srgb_counts = IndexedImageCounts::<Srgb<u8>>::default();
    /// let oklab_counts = srgb_counts.map_ref(srgb8_to_oklab);
    /// ```
    ///
    /// To instead map each color one at a time, use `iter`, `map`, and `collect` like normal:
    ///
    /// ```
    /// # use quantette::IndexedImageCounts;
    /// # use palette::{Srgb, LinSrgb};
    /// let srgb_counts = IndexedImageCounts::<Srgb<u8>>::default();
    /// let lin_srgb_counts: IndexedImageCounts<LinSrgb> =
    ///     srgb_counts.map_ref(|palette| palette.iter().map(|srgb| srgb.into_linear()).collect());
    /// ```
    ///
    /// # Panics
    ///
    /// Panics if `mapping` returns a palette with a different length than the original palette.
    #[must_use]
    #[inline]
    pub fn map_ref<NewColor>(
        &self,
        mapping: impl FnOnce(&[Color]) -> Vec<NewColor>,
    ) -> IndexedImageCounts<NewColor, Index> {
        let Self { image, counts } = self;
        let image = image.map_ref(mapping);
        IndexedImageCounts::from_indexed_image_unchecked(image, counts.clone())
    }

    /// Map an [`IndexedImageCounts`] to a new palette using the provided [`IndexedColorMap`].
    ///
    /// This function recalculates the `indices` for the new [`IndexedImage`], as the new palette
    /// may have a different length. To map only the palette colors see one of the following:
    /// - [`map_ref`](IndexedImageCounts::map_ref)
    /// - [`map`](IndexedImageCounts::map)
    /// - [`replace_palette`](IndexedImageCounts::replace_palette)
    ///
    /// Note that this returns an [`IndexedImage`] and if you want to recalculate the `counts` to
    /// create a new [`IndexedImageCounts`], then use
    /// [`into_indexed_image_counts`](IndexedImage::into_indexed_image_counts).
    ///
    /// # Panics
    ///
    /// Panics if `color_map` is not a valid implementor of [`IndexedColorMap`]. That is, it returns
    /// a [`Vec`] with a different length than the input slice.
    #[must_use]
    #[inline]
    pub fn map_to_indexed<ColorMap: IndexedColorMap<Color>>(
        &self,
        color_map: ColorMap,
    ) -> IndexedImage<ColorMap::Output> {
        self.image.map_to_indexed(color_map)
    }

    #[cfg(feature = "threads")]
    /// Map an [`IndexedImageCounts`] to a new palette in parallel using the provided
    /// [`IndexedColorMap`].
    ///
    /// This function recalculates the `indices` for the new [`IndexedImage`], as the new palette
    /// may have a different length. To map only the palette colors see one of the following:
    /// - [`map_ref`](IndexedImageCounts::map_ref)
    /// - [`map`](IndexedImageCounts::map)
    /// - [`replace_palette`](IndexedImageCounts::replace_palette)
    ///
    /// Note that this returns an [`IndexedImage`] and if you want to recalculate the `counts` to
    /// create a new [`IndexedImageCounts`], then use
    /// [`into_indexed_image_counts`](IndexedImage::into_indexed_image_counts).
    ///
    /// # Panics
    ///
    /// Panics if `color_map` is not a valid implementor of [`IndexedColorMap`]. That is, it returns
    /// a [`Vec`] with a different length than the input slice.
    #[must_use]
    #[inline]
    pub fn map_to_indexed_par<ColorMap: IndexedColorMap<Color>>(
        &self,
        color_map: ColorMap,
    ) -> IndexedImage<ColorMap::Output> {
        self.image.map_to_indexed_par(color_map)
    }

    /// Convert an [`IndexedImageCounts`] to an [`ImageBuf`] while also mapping the palette colors.
    ///
    /// This is more efficient than [`map_ref`](Self::map_ref) followed by
    /// [`to_image`](Self::to_image), since it avoids unnecessary intermediate `indices`.
    ///
    /// # Panics
    ///
    /// Panics if `color_map` is not a valid implementor of [`IndexedColorMap`]. That is, it returns
    /// a [`Vec`] with a different length than the input slice.
    #[must_use]
    #[inline]
    pub fn map_to_image<ColorMap: IndexedColorMap<Color>>(
        &self,
        color_map: ColorMap,
    ) -> ImageBuf<ColorMap::Output> {
        self.image.map_to_image(color_map)
    }

    #[cfg(feature = "threads")]
    /// Convert an [`IndexedImageCounts`] to an [`ImageBuf`] in parallel while also mapping the
    /// palette colors.
    ///
    /// This is more efficient than [`map_ref`](Self::map_ref) followed by
    /// [`to_image_par`](Self::to_image_par), since it avoids unnecessary intermediate `indices`.
    ///
    /// # Panics
    ///
    /// Panics if `color_map` is not a valid implementor of [`IndexedColorMap`]. That is, it returns
    /// a [`Vec`] with a different length than the input slice.
    #[must_use]
    #[inline]
    pub fn map_to_image_par<ColorMap: IndexedColorMap<Color>>(
        &self,
        color_map: ColorMap,
    ) -> ImageBuf<ColorMap::Output> {
        self.image.map_to_image_par(color_map)
    }

    /// Convert an [`IndexedImageCounts`] to an [`ImageBuf`].
    ///
    /// See [`map_to_image`](Self::map_to_image) if you also need to map the palette colors.
    #[must_use]
    #[inline]
    pub fn to_image(&self) -> ImageBuf<Color>
    where
        Color: Clone,
    {
        self.image.to_image()
    }

    #[cfg(feature = "threads")]
    /// Convert an [`IndexedImageCounts`] to an [`ImageBuf`] in parallel.
    ///
    /// See [`map_to_image_par`](Self::map_to_image_par) if you also need to map the palette colors.
    #[must_use]
    #[inline]
    pub fn to_image_par(&self) -> ImageBuf<Color>
    where
        Color: Clone + Send + Sync,
    {
        self.image.to_image_par()
    }
}

impl<Color, Index: BoundedIndex> Default for IndexedImageCounts<Color, Index> {
    #[inline]
    fn default() -> Self {
        Self::from_indexed_image_unchecked(IndexedImage::default(), Vec::new())
    }
}

impl<Color, Index: BoundedIndex> AsRef<IndexedImage<Color, Index>>
    for IndexedImageCounts<Color, Index>
{
    #[inline]
    fn as_ref(&self) -> &IndexedImage<Color, Index> {
        self.as_indexed_image()
    }
}
