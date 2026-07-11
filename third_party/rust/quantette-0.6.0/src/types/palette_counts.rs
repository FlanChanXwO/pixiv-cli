use alloc::vec::Vec;
use core::{
    error::Error,
    fmt::{self, Debug},
};

/// The reason a [`PaletteCounts`] failed to be created.
#[non_exhaustive]
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CreatePaletteCountsErrorReason {
    /// The length of the provided palette and the provided counts did not match.
    LengthMismatch,
    /// The sum of counts overflowed a `u32`.
    Overflow,
}
impl fmt::Display for CreatePaletteCountsErrorReason {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::LengthMismatch => write!(
                f,
                "the length of the provided palette and the provided counts do not match",
            ),
            Self::Overflow => {
                write!(f, "the sum of counts overflowed a u32")
            }
        }
    }
}

impl Error for CreatePaletteCountsErrorReason {}

/// The error returned when a [`PaletteCounts`] failed to be created.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CreatePaletteCountsError<Color> {
    /// The reason the [`PaletteCounts`] failed to be created.
    pub reason: CreatePaletteCountsErrorReason,
    /// The provided palette of colors.
    pub palette: Vec<Color>,
    /// The provided counts for each color.
    pub counts: Vec<u32>,
}

impl<Color> fmt::Display for CreatePaletteCountsError<Color> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.reason)
    }
}

impl<Color: Debug> Error for CreatePaletteCountsError<Color> {}

/// A palette of colors and their corresponding counts.
///
/// Certain algorithms are faster when operating on deduplicated data,
/// in which case [`PaletteCounts`] will be a supported input. To create
/// a [`PaletteCounts`], see the [`dedup`](crate::dedup) module or [`PaletteCounts::new`].
#[derive(Clone, Debug, PartialEq, Eq, Hash)]
pub struct PaletteCounts<Color> {
    /// The palette of colors.
    palette: Vec<Color>,
    /// The counts corresponding to each `palette` color.
    counts: Vec<u32>,
    /// The sum of `counts`.
    total_count: u32,
}

impl<Color> PaletteCounts<Color> {
    /// Create a new [`PaletteCounts`] without validating invariants.
    #[inline]
    pub(crate) fn new_unchecked(palette: Vec<Color>, counts: Vec<u32>, total_count: u32) -> Self {
        debug_assert_eq!(palette.len(), counts.len());
        debug_assert_eq!(total_count, counts.iter().copied().sum::<u32>());
        Self { palette, counts, total_count }
    }

    /// Create a new [`PaletteCounts`] from a [`Vec`] of palette colors
    /// and a [`Vec`] of corresponding counts.
    ///
    /// # Errors
    ///
    /// The provided `palette` and `counts` are returned as an `Err` if any of the following are true:
    /// - The length of `palette` and `counts` do not match.
    /// - The sum of `counts` is greater than [`MAX_PIXELS`](crate::MAX_PIXELS) (overflows a `u32`).
    #[inline]
    pub fn new(
        palette: Vec<Color>,
        counts: Vec<u32>,
    ) -> Result<Self, CreatePaletteCountsError<Color>> {
        let reason = if palette.len() == counts.len() {
            if let Some(total_count) = counts.iter().copied().try_fold(0, u32::checked_add) {
                return Ok(Self::new_unchecked(palette, counts, total_count));
            }
            CreatePaletteCountsErrorReason::Overflow
        } else {
            CreatePaletteCountsErrorReason::LengthMismatch
        };
        Err(CreatePaletteCountsError { reason, palette, counts })
    }

    /// Consume a [`PaletteCounts`] and return the inner [`Vec`] of palette colors
    /// and [`Vec`] of counts.
    #[must_use]
    #[inline]
    pub fn into_parts(self) -> (Vec<Color>, Vec<u32>) {
        let Self { palette, counts, .. } = self;
        (palette, counts)
    }

    /// Returns a slice of the inner palette colors.
    ///
    /// To get a mutable slice, see [`palette_mut`](Self::palette_mut).
    /// To get an owned [`Vec`], see [`into_parts`](Self::into_parts).
    #[inline]
    pub fn palette(&self) -> &[Color] {
        &self.palette
    }

    /// Returns a mutable slice of the inner palette colors.
    ///
    /// To get an immutable slice, see [`palette`](Self::palette).
    /// To get an owned [`Vec`], see [`into_parts`](Self::into_parts).
    #[inline]
    pub fn palette_mut(&mut self) -> &mut [Color] {
        &mut self.palette
    }

    /// Returns a slice of the inner counts.
    ///
    /// To get an owned [`Vec`], see [`into_parts`](Self::into_parts).
    #[inline]
    pub fn counts(&self) -> &[u32] {
        &self.counts
    }

    /// Returns the sum of the [`counts`](Self::counts) for all palette colors.
    ///
    /// This operation is `O(1)`. The total count is calculated on creation and never changes.
    #[inline]
    pub fn total_count(&self) -> u32 {
        self.total_count
    }

    /// Replace the palette of a [`PaletteCounts`] to a different color type (or to new colors
    /// of the same type).
    ///
    /// # Errors
    ///
    /// If the length of the provided `palette` does not match the length of the current palette
    /// in the [`PaletteCounts`], then `self` and `palette` are returned as an `Err`. Otherwise,
    /// the new [`PaletteCounts`] is returned alongside the old palette.
    #[allow(clippy::type_complexity)]
    #[inline]
    pub fn replace_palette<NewColor>(
        self,
        palette: Vec<NewColor>,
    ) -> Result<(PaletteCounts<NewColor>, Vec<Color>), (Self, Vec<NewColor>)> {
        if self.palette.len() == palette.len() {
            let Self {
                palette: old_palette,
                counts,
                total_count,
            } = self;
            Ok((
                PaletteCounts::new_unchecked(palette, counts, total_count),
                old_palette,
            ))
        } else {
            Err((self, palette))
        }
    }

    /// Map the palette of a [`PaletteCounts`], reusing the existing `counts`.
    ///
    /// See [`map_ref`](PaletteCounts::map_ref) to instead clone the existing `counts`
    /// and retain the original [`PaletteCounts`].
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
    /// # use quantette::PaletteCounts;
    /// # use palette::{Srgb, LinSrgb};
    /// use quantette::color_space::srgb8_to_oklab;
    /// let srgb_counts = PaletteCounts::<Srgb<u8>>::default();
    /// let oklab_counts = srgb_counts.map(|palette| srgb8_to_oklab(&palette));
    /// ```
    ///
    /// To instead map each color one at a time, use `into_iter`, `map`, and `collect` like normal:
    ///
    /// ```
    /// # use quantette::PaletteCounts;
    /// # use palette::{Srgb, LinSrgb};
    /// let srgb_counts = PaletteCounts::<Srgb<u8>>::default();
    /// let lin_srgb_counts: PaletteCounts<LinSrgb> =
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
    ) -> PaletteCounts<NewColor> {
        let Self { palette, counts, total_count, .. } = self;
        let len = palette.len();
        let palette = mapping(palette);
        assert_eq!(palette.len(), len);
        PaletteCounts::new_unchecked(palette, counts, total_count)
    }

    /// Map the palette colors of a [`PaletteCounts`] to a new [`PaletteCounts`],
    /// cloning the `counts` in the process.
    ///
    /// See [`map`](PaletteCounts::map) to instead consume the original [`PaletteCounts`]
    /// and avoid a clone of the `counts`.
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
    /// # use quantette::PaletteCounts;
    /// # use palette::{Srgb, LinSrgb};
    /// use quantette::color_space::srgb8_to_oklab;
    /// let srgb_counts = PaletteCounts::<Srgb<u8>>::default();
    /// let oklab_counts = srgb_counts.map_ref(srgb8_to_oklab);
    /// ```
    ///
    /// To instead map each color one at a time, use `iter`, `map`, and `collect` like normal:
    ///
    /// ```
    /// # use quantette::PaletteCounts;
    /// # use palette::{Srgb, LinSrgb};
    /// let srgb_counts = PaletteCounts::<Srgb<u8>>::default();
    /// let lin_srgb_counts: PaletteCounts<LinSrgb> =
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
    ) -> PaletteCounts<NewColor> {
        let palette = mapping(self.palette());
        PaletteCounts::new_unchecked(palette, self.counts.clone(), self.total_count)
    }
}

impl<Color> Default for PaletteCounts<Color> {
    #[inline]
    fn default() -> Self {
        Self::new_unchecked(Vec::new(), Vec::new(), 0)
    }
}
