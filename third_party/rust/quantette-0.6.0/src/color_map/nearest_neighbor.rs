use crate::{ColorComponents, IndexedColorMap, Palette, PaletteBuf};
use alloc::vec::Vec;
use core::{array, marker::PhantomData};
use palette::cast::{self, AsArrays as _};
use wide::{CmpLe as _, f32x8, u32x8};

/// An [`IndexedColorMap`] that maps input colors to their nearest palette color according to
/// euclidean distance.
#[derive(Clone, Debug)]
pub struct NearestNeighborColorMap<Color, Component, const N: usize> {
    /// The component type of `Color`.
    component: PhantomData<Component>,
    /// The palette of colors.
    pub(crate) palette: PaletteBuf<Color>,
    /// The palette colors laid out in AoSoA format.
    pub(crate) data: Vec<[f32x8; N]>,
}
impl<Color, Component, const N: usize> NearestNeighborColorMap<Color, Component, N>
where
    Color: ColorComponents<Component, N>,
    Component: Copy + Into<f32>,
{
    /// Create a new [`NearestNeighborColorMap`] from a `palette` of colors.
    #[must_use]
    pub fn new(palette: PaletteBuf<Color>) -> Self {
        let mut data = Vec::with_capacity(palette.len().div_ceil(8));

        let (chunks, remainder) = palette.as_arrays().as_chunks::<8>();
        data.extend(
            chunks
                .iter()
                .map(|chunk| array::from_fn(|i| f32x8::new(chunk.map(|x| x[i].into())))),
        );

        if !remainder.is_empty() {
            let mut arr = [[f32::INFINITY; 8]; N];
            for (i, &color) in remainder.iter().enumerate() {
                for (arr, c) in arr.iter_mut().zip(color) {
                    arr[i] = c.into();
                }
            }
            data.push(arr.map(f32x8::new));
        }

        Self { component: PhantomData, palette, data }
    }
}

/// Compute the chunk index and lane index of the nearest palette color to `color` according to
/// euclidean distance.
#[inline]
pub(crate) fn simd_argmin_min_distance<const N: usize>(
    data: &[[f32x8; N]],
    color: [f32; N],
) -> ((u8, u8), f32) {
    let incr = u32x8::ONE;
    let mut cur_chunk = u32x8::ZERO;
    let mut min_chunk = cur_chunk;
    let mut min_distance = f32x8::splat(f32::INFINITY);

    let color = color.map(f32x8::splat);

    for chunk in data {
        #[allow(clippy::expect_used)]
        let distance = array::from_fn::<_, N, _>(|i| {
            let diff = color[i] - chunk[i];
            diff * diff
        })
        .into_iter()
        .reduce(|a, b| a + b)
        .expect("N != 0");

        let mask: u32x8 = bytemuck::cast(distance.simd_le(min_distance));
        min_chunk = mask.blend(cur_chunk, min_chunk);
        min_distance = min_distance.fast_min(distance);
        cur_chunk += incr;
    }

    let mut min_lane = 0;
    let mut min_dist = f32::INFINITY;
    for (i, v) in min_distance.to_array().into_iter().enumerate() {
        if v < min_dist {
            min_dist = v;
            min_lane = i;
        }
    }

    let min_chunk = min_chunk.as_array()[min_lane];

    #[allow(clippy::cast_possible_truncation)]
    {
        ((min_chunk as u8, min_lane as u8), min_dist)
    }
}

impl<Color, Component, const N: usize> NearestNeighborColorMap<Color, Component, N>
where
    Color: ColorComponents<Component, N>,
    Component: Copy + Into<f32>,
{
    /// Compute the index of the nearest palette color to `color` according to euclidean distance.
    #[inline]
    fn map_to_index(&self, color: &Color) -> u8 {
        let (chunk, lane) =
            simd_argmin_min_distance(&self.data, cast::into_array(*color).map(Into::into)).0;

        chunk * 8 + lane
    }

    /// Replace a slice of colors with their nearest palette color according to euclidean distance.
    #[inline]
    pub fn map_slice_in_place(&self, colors: &mut [Color]) {
        for color in colors {
            *color = self.palette[self.map_to_index(color)];
        }
    }
}

impl<Color, Component, const N: usize> NearestNeighborColorMap<Color, Component, N> {
    /// Consume a [`NearestNeighborColorMap`] and return the underlying color palette.
    #[must_use]
    #[inline]
    pub fn into_palette(self) -> PaletteBuf<Color> {
        self.palette
    }

    /// Return a reference to the underlying color palette.
    #[inline]
    pub fn palette(&self) -> &Palette<Color> {
        &self.palette
    }
}

impl<Color, Component, const N: usize> IndexedColorMap<Color>
    for NearestNeighborColorMap<Color, Component, N>
where
    Color: ColorComponents<Component, N>,
    Component: Copy + Into<f32>,
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
        self.map_to_index(color)
    }
}

#[cfg(feature = "threads")]
mod parallel {
    use super::NearestNeighborColorMap;
    use crate::{ColorComponents, IndexedColorMap, Palette, PaletteBuf};
    use rayon::prelude::*;

    /// A lookup structure that maps input colors to their nearest palette color according to
    /// euclidean distance in parallel.
    #[derive(Clone, Debug)]
    pub struct NearestNeighborParallelColorMap<Color, Component, const N: usize>(
        NearestNeighborColorMap<Color, Component, N>,
    );

    impl<Color, Component, const N: usize> NearestNeighborParallelColorMap<Color, Component, N>
    where
        Color: ColorComponents<Component, N>,
        Component: Copy + Into<f32>,
    {
        /// Create a new [`NearestNeighborParallelColorMap`] from a `palette` of colors.
        #[must_use]
        #[inline]
        pub fn new(palette: PaletteBuf<Color>) -> Self {
            Self(NearestNeighborColorMap::new(palette))
        }
    }

    impl<Color, Component, const N: usize> NearestNeighborParallelColorMap<Color, Component, N> {
        /// Consume a [`NearestNeighborParallelColorMap`] and return the underlying color palette.
        #[must_use]
        #[inline]
        pub fn into_palette(self) -> PaletteBuf<Color> {
            self.0.into_palette()
        }

        /// Return a reference to the underlying color palette.
        #[inline]
        pub fn palette(&self) -> &Palette<Color> {
            self.0.palette()
        }

        /// Convert a [`NearestNeighborParallelColorMap`] to a [`NearestNeighborColorMap`].
        #[must_use]
        #[inline]
        pub fn into_serial(self) -> NearestNeighborColorMap<Color, Component, N> {
            self.0
        }
    }

    impl<Color, Component, const N: usize> NearestNeighborColorMap<Color, Component, N> {
        /// Convert a [`NearestNeighborColorMap`] to a [`NearestNeighborParallelColorMap`].
        #[must_use]
        #[inline]
        pub fn into_parallel(self) -> NearestNeighborParallelColorMap<Color, Component, N> {
            NearestNeighborParallelColorMap(self)
        }
    }

    impl<Color, Component, const N: usize>
        From<NearestNeighborParallelColorMap<Color, Component, N>>
        for NearestNeighborColorMap<Color, Component, N>
    {
        #[inline]
        fn from(color_map: NearestNeighborParallelColorMap<Color, Component, N>) -> Self {
            color_map.into_serial()
        }
    }

    impl<Color, Component, const N: usize> From<NearestNeighborColorMap<Color, Component, N>>
        for NearestNeighborParallelColorMap<Color, Component, N>
    {
        #[inline]
        fn from(color_map: NearestNeighborColorMap<Color, Component, N>) -> Self {
            color_map.into_parallel()
        }
    }

    impl<Color, Component, const N: usize> NearestNeighborParallelColorMap<Color, Component, N>
    where
        Color: ColorComponents<Component, N>,
        Component: Copy + Into<f32> + Sync,
    {
        /// Replace a slice of colors with their nearest palette color according to euclidean
        /// distance.
        #[inline]
        pub fn map_slice_in_place(&self, colors: &mut [Color]) {
            colors
                .par_iter_mut()
                .for_each(|color| *color = self.palette_color(color))
        }
    }

    impl<Color, Component, const N: usize> IndexedColorMap<Color>
        for NearestNeighborParallelColorMap<Color, Component, N>
    where
        Color: ColorComponents<Component, N>,
        Component: Copy + Into<f32> + Sync,
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
            self.0.base_palette()
        }

        #[inline]
        fn palette_index(&self, color: &Color) -> u8 {
            self.0.map_to_index(color)
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

        #[inline]
        fn map_to_colors(&self, input: &[Color]) -> Vec<Self::Output> {
            input
                .par_iter()
                .map(|color| self.palette_color(color))
                .collect()
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
    use palette::{Oklab, cast::IntoArrays as _};

    fn test_palette() -> PaletteBuf<Oklab> {
        let mut centroids = test_data_256();
        centroids.truncate(249u8.try_into().unwrap()); // use non-multiple of 8 to test remainder handling
        PaletteBuf::from_mapping(&centroids, srgb8_to_oklab)
    }

    #[test]
    fn naive_nearest_neighbor_oracle() {
        fn squared_euclidean_distance<const N: usize>(x: [f32; N], y: [f32; N]) -> f32 {
            let mut dist = 0.0;
            for c in 0..N {
                let d = x[c] - y[c];
                dist += d * d;
            }
            dist
        }

        let centroids = test_palette();
        let points = srgb8_to_oklab(&test_data_1024());
        let nearest = NearestNeighborColorMap::new(centroids.clone());
        let centroids = centroids.into_arrays();
        for color in points.into_arrays() {
            let expected = centroids
                .iter()
                .map(|&centroid| OrderedFloat(squared_euclidean_distance(centroid, color)))
                .min()
                .unwrap()
                .0;

            let ((chunk, lane), actual2) = simd_argmin_min_distance(&nearest.data, color);
            let index = usize::from(chunk) * 8 + usize::from(lane);
            let actual1 = squared_euclidean_distance(color, centroids[index]);

            #[allow(clippy::float_cmp)]
            {
                assert_eq!(expected, actual1);
                assert_eq!(expected, actual2);
            }
        }
    }
}
