#[cfg(feature = "kmeans")]
use crate::kmeans::KmeansOptions;
use crate::{CreatePaletteBufError, PaletteBuf, PaletteInColorSpace};
use alloc::vec::Vec;
use palette::{Oklab, Srgb};

/// The set of supported color quantization methods.
///
/// If the `kmeans` feature is enabled, then support will be added for that method.
/// Otherwise, only Wu's color quantization method and custom palettes are supported.
///
/// See the descriptions on each enum variant for more information.
#[derive(Debug, Clone, PartialEq)]
pub enum QuantizeMethod {
    /// Quantize an image into a custom color palette.
    ///
    /// This method will simply map each pixel to its closest color in the palette.
    CustomPalette(PaletteInColorSpace),
    /// Wu's color quantizer (Greedy Orthogonal Bipartitioning).
    ///
    /// This method is quick and gives good or at least decent results.
    ///
    /// See the [`wu`](crate::wu) module for more details.
    Wu,
    #[cfg(feature = "kmeans")]
    /// Color quantization using k-means clustering.
    ///
    /// This method is slower than Wu's color quantizer but gives more accurate results.
    ///
    /// See the [`kmeans`](crate::kmeans) module for more details.
    Kmeans(KmeansOptions),
}
impl QuantizeMethod {
    #[cfg(feature = "kmeans")]
    /// Create a new [`QuantizeMethod::Kmeans`] with the default [`KmeansOptions`].
    #[must_use]
    pub const fn kmeans() -> Self {
        Self::Kmeans(KmeansOptions::new())
    }
}

impl From<PaletteInColorSpace> for QuantizeMethod {
    #[inline]
    fn from(palette: PaletteInColorSpace) -> Self {
        Self::CustomPalette(palette)
    }
}

impl From<PaletteBuf<Srgb<u8>>> for QuantizeMethod {
    #[inline]
    fn from(palette: PaletteBuf<Srgb<u8>>) -> Self {
        Self::CustomPalette(palette.into())
    }
}

impl From<PaletteBuf<Oklab>> for QuantizeMethod {
    #[inline]
    fn from(palette: PaletteBuf<Oklab>) -> Self {
        Self::CustomPalette(palette.into())
    }
}

#[cfg(feature = "kmeans")]
impl From<KmeansOptions> for QuantizeMethod {
    #[inline]
    fn from(options: KmeansOptions) -> Self {
        Self::Kmeans(options)
    }
}

impl TryFrom<Vec<Srgb<u8>>> for QuantizeMethod {
    type Error = CreatePaletteBufError<Srgb<u8>>;

    #[inline]
    fn try_from(colors: Vec<Srgb<u8>>) -> Result<Self, Self::Error> {
        PaletteBuf::try_from(colors).map(Into::into)
    }
}

impl TryFrom<Vec<Oklab>> for QuantizeMethod {
    type Error = CreatePaletteBufError<Oklab>;

    #[inline]
    fn try_from(colors: Vec<Oklab>) -> Result<Self, Self::Error> {
        PaletteBuf::try_from(colors).map(Into::into)
    }
}
