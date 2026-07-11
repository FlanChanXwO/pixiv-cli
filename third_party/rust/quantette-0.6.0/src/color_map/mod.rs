//! Map the colors of an image to a color palette or to palette indices.

use crate::{Palette, PaletteBuf};
use alloc::{borrow::ToOwned as _, vec::Vec};

mod nearest_neighbor;
mod palette_substitution;

pub use nearest_neighbor::*;
pub use palette_substitution::*;

/// A trait for mapping colors to a retstricted set of palette colors.
///
/// Typically, implementors of this trait consist of a palette and a lookup data structure to map
/// any input color to one of the palette colors.
///
/// The main function of interest is [`palette_index`](IndexedColorMap::palette_index) which takes
/// a color as input and returns an index into [`palette`](IndexedColorMap::palette). The other
/// mapping functions like [`palette_color`](IndexedColorMap::palette_color) and
/// [`map_to_colors`](IndexedColorMap::map_to_colors) have default implementations based off
/// [`palette_index`](IndexedColorMap::palette_index), but these should be overridden with more
/// efficient implementations where it makes sense.
///
/// You can use those trait functions directly, but you likely want to use the various map functions
/// on the image types instead:
/// - [`Image::map_to_image`](crate::Image::map_to_image)
/// - [`Image::map_to_indexed`](crate::Image::map_to_indexed)
/// - [`IndexedImage::map_to_image`](crate::IndexedImage::map_to_image)
/// - [`IndexedImage::map_to_indexed`](crate::IndexedImage::map_to_indexed)
///
/// Additionally, the dither types from the [`dither`](crate::dither) module take an
/// [`IndexedColorMap`] as an input.
///
/// To create an [`IndexedColorMap`], see:
/// - [`wu`](crate::wu)
/// - [`kmeans`](crate::kmeans)
/// - [`NearestNeighborColorMap`]
/// - [`PaletteSubstitution`]
pub trait IndexedColorMap<Input> {
    /// The output color type.
    ///
    /// Oftentimes, this the same type as the [`IndexedColorMap`] `Input`. Sometimes, like for
    /// [`PaletteSubstitution`], each original palette color is mapped to a different output color,
    /// in which case `Input` may not be the same type as `Output`.
    type Output: Clone + Send + Sync;

    /// Consume this color map and return the underlying [`PaletteBuf`] of output colors.
    fn into_palette(self) -> PaletteBuf<Self::Output>;

    /// Returns a refererce to the underlying [`Palette`] of output colors.
    fn palette(&self) -> &Palette<Self::Output>;

    /// Returns a refererce to the underlying [`Palette`] of input colors.
    ///
    /// Oftentimes, this the same as [`palette`](IndexedColorMap::palette). Sometimes, like for
    /// [`PaletteSubstitution`], this will return the original palette which is used to calculate
    /// the nearest color, whereas [`palette`](IndexedColorMap::palette) will return the substitute
    /// palette used to populate the output.
    fn base_palette(&self) -> &Palette<Input>;

    /// Returns the index of the palette color for the given input `color`.
    fn palette_index(&self, color: &Input) -> u8;

    /// Returns the palette color for the given input `color`.
    #[inline]
    fn palette_color(&self, color: &Input) -> Self::Output {
        self.palette()[self.palette_index(color)].clone()
    }

    /// Map each `input` color to an index for one of the palette colors.
    #[inline]
    fn map_to_indices(&self, input: &[Input]) -> Vec<u8> {
        input
            .iter()
            .map(|color| self.palette_index(color))
            .collect()
    }

    /// Map each `input` color to a palette index, but instead of indexing the normal palette,
    /// use the provided substitute `palette` instead.
    #[inline]
    fn map_to_colors_of_palette<T: Clone + Send + Sync>(
        &self,
        palette: &Palette<T>,
        input: &[Input],
    ) -> Vec<T> {
        input
            .iter()
            .map(|color| palette[self.palette_index(color)].clone())
            .collect()
    }

    /// Map each `input` color to one of the palette colors.
    #[inline]
    fn map_to_colors(&self, input: &[Input]) -> Vec<Self::Output> {
        self.map_to_colors_of_palette(self.palette(), input)
    }
}
impl<Input, R> IndexedColorMap<Input> for &R
where
    R: IndexedColorMap<Input>,
{
    type Output = R::Output;

    #[inline]
    fn into_palette(self) -> PaletteBuf<Self::Output> {
        self.palette().to_owned()
    }

    #[inline]
    fn palette(&self) -> &Palette<Self::Output> {
        (*self).palette()
    }

    #[inline]
    fn base_palette(&self) -> &Palette<Input> {
        (*self).base_palette()
    }

    #[inline]
    fn palette_color(&self, color: &Input) -> Self::Output {
        (*self).palette_color(color)
    }

    #[inline]
    fn palette_index(&self, color: &Input) -> u8 {
        (*self).palette_index(color)
    }

    #[inline]
    fn map_to_indices(&self, input: &[Input]) -> Vec<u8> {
        (*self).map_to_indices(input)
    }

    fn map_to_colors_of_palette<T: Clone + Send + Sync>(
        &self,
        palette: &Palette<T>,
        input: &[Input],
    ) -> Vec<T> {
        (*self).map_to_colors_of_palette(palette, input)
    }

    #[inline]
    fn map_to_colors(&self, input: &[Input]) -> Vec<Self::Output> {
        (*self).map_to_colors(input)
    }
}
