use crate::{IndexedColorMap, Palette, PaletteBuf};
use alloc::vec::Vec;
use core::marker::PhantomData;

/// An [`IndexedColorMap`] that maps one palette to another of the same length.
///
/// This struct has to main parts:
/// 1. A `ColorMap` which is used to map `Input` colors to indices.
/// 2. A substitute palette of any `Output` color type with the same length as the palette of `color_map`.
///
/// Indices from `color_map` are used to index the substitute palette whose colors are copied to the
/// final output or result.
pub struct PaletteSubstitution<Input, Output, ColorMap> {
    /// The input color type.
    input: PhantomData<Input>,
    /// The original color map.
    color_map: ColorMap,
    /// The substitute palette of output colors.
    palette: PaletteBuf<Output>,
}
impl<Input, Output, ColorMap> PaletteSubstitution<Input, Output, ColorMap>
where
    ColorMap: IndexedColorMap<Input>,
{
    /// Create a new [`PaletteSubstitution`] without validating invariants.
    #[inline]
    fn new_unchecked(color_map: ColorMap, palette: PaletteBuf<Output>) -> Self {
        debug_assert_eq!(color_map.palette().len(), palette.len());
        Self { input: PhantomData, palette, color_map }
    }

    /// Create a new [`PaletteSubstitution`] from an existing [`IndexedColorMap`] and a new
    /// `palette` to substitute the map's palette with.
    ///
    /// # Errors
    ///
    /// Returns `palette` and `color_map` as an error if the length of `palette` does not match the length
    /// of `color_map.palette()`.
    #[inline]
    pub fn new(
        color_map: ColorMap,
        palette: PaletteBuf<Output>,
    ) -> Result<Self, (ColorMap, PaletteBuf<Output>)> {
        if color_map.palette().len() == palette.len() {
            Ok(Self::new_unchecked(color_map, palette))
        } else {
            Err((color_map, palette))
        }
    }

    /// Create a new [`PaletteSubstitution`] from an existing [`IndexedColorMap`] and a
    /// `color_mapping` which is used to create a new palette from `color_map.palette()`.
    #[must_use]
    #[inline]
    pub fn from_color_mapping(
        color_map: ColorMap,
        color_mapping: impl FnMut(&ColorMap::Output) -> Output,
    ) -> Self {
        let palette = color_map.palette().map_ref(color_mapping);
        Self::new_unchecked(color_map, palette)
    }

    /// Create a new [`PaletteSubstitution`] from an existing [`IndexedColorMap`] and a
    /// `slice_mapping` which is used to create a new palette from `color_map.palette()`.
    ///
    /// # Panics
    ///
    /// Panics if `slice_mapping` returns a [`Vec`] with a different length than the input slice.
    #[must_use]
    #[inline]
    pub fn from_slice_mapping(
        color_map: ColorMap,
        slice_mapping: impl FnOnce(&[ColorMap::Output]) -> Vec<Output>,
    ) -> Self {
        let palette = PaletteBuf::from_mapping(color_map.palette(), slice_mapping);
        Self::new_unchecked(color_map, palette)
    }

    /// Consume a [`PaletteSubstitution`] and return the underlying color map and substitute palette.
    #[must_use]
    #[inline]
    pub fn into_parts(self) -> (ColorMap, PaletteBuf<Output>) {
        let Self { color_map, palette, .. } = self;
        (color_map, palette)
    }

    /// Returns a reference to the substitute [`Palette`].
    #[inline]
    pub fn palette(&self) -> &Palette<Output> {
        &self.palette
    }

    /// Returns a mutable reference to the substitute [`Palette`].
    #[inline]
    pub fn palette_mut(&mut self) -> &mut Palette<Output> {
        &mut self.palette
    }

    /// Returns a reference to the underlying [`IndexedColorMap`].
    #[inline]
    pub fn color_map(&self) -> &ColorMap {
        &self.color_map
    }

    /// Map the substitute palette to new colors, creating a new [`PaletteSubstitution`].
    #[must_use]
    #[inline]
    pub fn map<NewOutput>(
        self,
        mapping: impl FnMut(Output) -> NewOutput,
    ) -> PaletteSubstitution<Input, NewOutput, ColorMap> {
        let Self { input, palette, color_map } = self;
        let palette = palette.map(mapping);
        PaletteSubstitution { input, color_map, palette }
    }

    /// Replace the palette of a [`PaletteSubstitution`] to a different color type (or to new colors
    /// of the same type).
    ///
    /// # Errors
    ///
    /// If the length of the provided `palette` does not match the length of the current palette
    /// in the [`PaletteSubstitution`], then `self` and `palette` are returned as an `Err`.
    /// Otherwise, the new [`PaletteSubstitution`] is returned alongside the old palette.
    #[allow(clippy::type_complexity)]
    #[inline]
    pub fn replace_palette<NewOutput>(
        self,
        palette: PaletteBuf<NewOutput>,
    ) -> Result<
        (
            PaletteSubstitution<Input, NewOutput, ColorMap>,
            PaletteBuf<Output>,
        ),
        (Self, PaletteBuf<NewOutput>),
    > {
        if self.palette.len() == palette.len() {
            let Self { input, palette: old_palette, color_map } = self;
            let color_map = PaletteSubstitution { input, color_map, palette };
            Ok((color_map, old_palette))
        } else {
            Err((self, palette))
        }
    }
}

impl<Input, Output, ColorMap> IndexedColorMap<Input>
    for PaletteSubstitution<Input, Output, ColorMap>
where
    ColorMap: IndexedColorMap<Input>,
    Output: Clone + Send + Sync,
{
    type Output = Output;

    #[inline]
    fn into_palette(self) -> PaletteBuf<Self::Output> {
        self.into_parts().1
    }

    #[inline]
    fn palette(&self) -> &Palette<Self::Output> {
        self.palette()
    }

    #[inline]
    fn base_palette(&self) -> &Palette<Input> {
        self.color_map().base_palette()
    }

    #[inline]
    fn palette_index(&self, color: &Input) -> u8 {
        self.color_map().palette_index(color)
    }

    #[inline]
    fn map_to_indices(&self, input: &[Input]) -> Vec<u8> {
        self.color_map().map_to_indices(input)
    }

    #[inline]
    fn map_to_colors_of_palette<T: Clone + Send + Sync>(
        &self,
        palette: &Palette<T>,
        input: &[Input],
    ) -> Vec<T> {
        self.color_map().map_to_colors_of_palette(palette, input)
    }
}
