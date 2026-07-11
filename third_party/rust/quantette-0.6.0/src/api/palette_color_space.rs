use crate::{
    CreatePaletteBufError, PaletteBuf,
    color_space::{oklab_to_srgb8, srgb8_to_oklab},
};
use alloc::vec::Vec;
use palette::{Oklab, Srgb};

/// A [`PaletteBuf`] of colors in one of the supported color spaces for
/// [`QuantizeMethod::CustomPalette`](crate::QuantizeMethod::CustomPalette).
#[derive(Debug, Clone, PartialEq)]
pub enum PaletteInColorSpace {
    /// A [`PaletteBuf`] of colors in the [`Srgb`] color space.
    Srgb8(PaletteBuf<Srgb<u8>>),
    /// A [`PaletteBuf`] of colors in the [`Oklab`] color space.
    Oklab(PaletteBuf<Oklab>),
}
impl PaletteInColorSpace {
    /// Convert to a new [`PaletteBuf<Srgb<u8>>`].
    #[must_use]
    pub fn to_srgb8(&self) -> PaletteBuf<Srgb<u8>> {
        match self {
            Self::Srgb8(palette) => palette.clone(),
            Self::Oklab(palette) => PaletteBuf::from_mapping(palette, oklab_to_srgb8),
        }
    }

    /// Convert to a new [`PaletteBuf<Oklab>`].
    #[must_use]
    pub fn to_oklab(&self) -> PaletteBuf<Oklab> {
        match self {
            Self::Srgb8(palette) => PaletteBuf::from_mapping(palette, srgb8_to_oklab),
            Self::Oklab(palette) => palette.clone(),
        }
    }

    /// Convert to a [`PaletteBuf<Srgb<u8>>`], returning `self` if it is already in [`Srgb`].
    #[must_use]
    pub fn into_srgb8(self) -> PaletteBuf<Srgb<u8>> {
        match self {
            Self::Srgb8(palette) => palette,
            Self::Oklab(palette) => PaletteBuf::from_mapping(&palette, oklab_to_srgb8),
        }
    }

    /// Convert to a [`PaletteBuf<Oklab>`], returning `self` if it is already in [`Oklab`].
    #[must_use]
    pub fn into_oklab(self) -> PaletteBuf<Oklab> {
        match self {
            Self::Srgb8(palette) => PaletteBuf::from_mapping(&palette, srgb8_to_oklab),
            Self::Oklab(palette) => palette,
        }
    }
}

impl From<PaletteBuf<Srgb<u8>>> for PaletteInColorSpace {
    #[inline]
    fn from(palette: PaletteBuf<Srgb<u8>>) -> Self {
        Self::Srgb8(palette)
    }
}

impl From<PaletteBuf<Oklab>> for PaletteInColorSpace {
    #[inline]
    fn from(palette: PaletteBuf<Oklab>) -> Self {
        Self::Oklab(palette)
    }
}

impl TryFrom<Vec<Srgb<u8>>> for PaletteInColorSpace {
    type Error = CreatePaletteBufError<Srgb<u8>>;

    #[inline]
    fn try_from(colors: Vec<Srgb<u8>>) -> Result<Self, Self::Error> {
        PaletteBuf::try_from(colors).map(Into::into)
    }
}

impl TryFrom<Vec<Oklab>> for PaletteInColorSpace {
    type Error = CreatePaletteBufError<Oklab>;

    #[inline]
    fn try_from(colors: Vec<Oklab>) -> Result<Self, Self::Error> {
        PaletteBuf::try_from(colors).map(Into::into)
    }
}
