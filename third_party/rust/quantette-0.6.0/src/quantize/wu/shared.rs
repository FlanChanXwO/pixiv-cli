// Referenced code: https://www.ece.mcmaster.ca/~xwu/cq.c
// and relevant paper (free access):
// Xiaolin Wu, Color quantization by dynamic programming and principal analysis,
// ACM Transactions on Graphics, vol. 11, no. 4, 348–372, 1992.
// https://doi.org/10.1145/146443.146475

use crate::{ColorComponents, PaletteBuf, PaletteSize};
use alloc::{boxed::Box, collections::BinaryHeap};
use bytemuck::Zeroable;
use core::{
    array,
    cmp::Ordering,
    marker::PhantomData,
    ops::{Add, AddAssign, Index, IndexMut, Sub},
};
use num_traits::{AsPrimitive, NumOps, Zero};
use ordered_float::OrderedFloat;
use palette::cast;

/// A hypercube over a multi-dimensional range of histogram bins.
#[derive(Clone, Copy)]
pub struct Cube<const N: usize> {
    /// The lower bin indices (inclusive).
    pub min: [u8; N],
    /// The upper bin indices (exclusive).
    pub max: [u8; N],
}
/// A cube and it's variance.
pub struct CubeVar<const N: usize>(pub Cube<N>, pub f64);

impl<const N: usize> PartialOrd for CubeVar<N> {
    fn partial_cmp(&self, other: &Self) -> Option<Ordering> {
        Some(self.cmp(other))
    }
}

impl<const N: usize> Ord for CubeVar<N> {
    fn cmp(&self, other: &Self) -> Ordering {
        OrderedFloat(self.1).cmp(&OrderedFloat(other.1))
    }
}

impl<const N: usize> Eq for CubeVar<N> {}

impl<const N: usize> PartialEq for CubeVar<N> {
    fn eq(&self, other: &Self) -> bool {
        self.1 == other.1
    }
}

/// Statistics for a histogram bin.
#[derive(Clone, Copy, PartialEq, Zeroable)]
pub struct Stats<T, const N: usize, Count = u32> {
    /// The number of pixels/colors assigned to the bin.
    pub count: Count,
    /// The component-wise sum of the colors assigned to the bin.
    pub components: [T; N],
    /// The sum of the squared components of the colors assigned to the bin.
    pub sum_squared: f64,
}

impl<T, const N: usize, Count> Add for Stats<T, N, Count>
where
    T: Copy + Add<Output = T>,
    Count: Add<Output = Count>,
{
    type Output = Self;

    #[inline]
    fn add(self, rhs: Self) -> Self::Output {
        Self {
            count: self.count + rhs.count,
            components: array::from_fn(|i| self.components[i] + rhs.components[i]),
            sum_squared: self.sum_squared + rhs.sum_squared,
        }
    }
}

impl<T, const N: usize, Count> Sub for Stats<T, N, Count>
where
    T: Copy + Sub<Output = T>,
    Count: Sub<Output = Count>,
{
    type Output = Self;

    #[inline]
    fn sub(self, rhs: Self) -> Self::Output {
        Self {
            count: self.count - rhs.count,
            components: array::from_fn(|i| self.components[i] - rhs.components[i]),
            sum_squared: self.sum_squared - rhs.sum_squared,
        }
    }
}

impl<T, const N: usize, Count> AddAssign for Stats<T, N, Count>
where
    T: Copy + AddAssign,
    Count: AddAssign,
{
    #[inline]
    fn add_assign(&mut self, rhs: Self) {
        self.count += rhs.count;
        for i in 0..N {
            self.components[i] += rhs.components[i];
        }
        self.sum_squared += rhs.sum_squared;
    }
}

impl<T, const N: usize, Count> Zero for Stats<T, N, Count>
where
    T: Copy + Zero,
    Count: Zero,
{
    #[inline]
    fn zero() -> Self {
        Self {
            count: Count::zero(),
            components: [T::zero(); N],
            sum_squared: 0.0,
        }
    }

    #[inline]
    fn is_zero(&self) -> bool {
        self.count.is_zero()
    }
}

/// A new type wrapper around a 3-dimensional array.
#[repr(transparent)]
#[derive(Debug, Clone)]
pub struct Histogram3<T, const B1: usize, const B2: usize, const B3: usize>(
    pub Box<[[[T; B3]; B2]; B1]>,
);

impl<T, const B1: usize, const B2: usize, const B3: usize> Index<[usize; 3]>
    for Histogram3<T, B1, B2, B3>
{
    type Output = T;

    #[inline]
    fn index(&self, index: [usize; 3]) -> &Self::Output {
        &self.0[index[0]][index[1]][index[2]]
    }
}

impl<T, const B1: usize, const B2: usize, const B3: usize> IndexMut<[usize; 3]>
    for Histogram3<T, B1, B2, B3>
{
    #[inline]
    fn index_mut(&mut self, index: [usize; 3]) -> &mut Self::Output {
        &mut self.0[index[0]][index[1]][index[2]]
    }
}

impl<T, const B1: usize, const B2: usize, const B3: usize> Index<[u8; 3]>
    for Histogram3<T, B1, B2, B3>
{
    type Output = T;

    #[inline]
    fn index(&self, index: [u8; 3]) -> &Self::Output {
        &self[index.map(usize::from)]
    }
}

impl<T, const B1: usize, const B2: usize, const B3: usize> IndexMut<[u8; 3]>
    for Histogram3<T, B1, B2, B3>
{
    #[inline]
    fn index_mut(&mut self, index: [u8; 3]) -> &mut Self::Output {
        &mut self[index.map(usize::from)]
    }
}

impl<T, const B1: usize, const B2: usize, const B3: usize> Histogram3<T, B1, B2, B3>
where
    T: Zero + Zeroable + PartialEq,
{
    /// Create a new [`Histogram3`] from zeroed memory.
    pub fn new() -> Self {
        const {
            assert!(1 <= B1 && B1 <= u8::MAX as usize);
            assert!(1 <= B2 && B2 <= u8::MAX as usize);
            assert!(1 <= B3 && B3 <= u8::MAX as usize);
        }
        assert!(
            T::zero() == T::zeroed(),
            "the zero value for the component sum should be representable by the all zero byte pattern"
        );
        Self(bytemuck::zeroed_box())
    }
}

impl<T, const B1: usize, const B2: usize, const B3: usize> Histogram3<T, B1, B2, B3> {
    /// Returns a flat slice of histogram bins.
    #[inline]
    pub fn as_flattened(&self) -> &[T] {
        self.0.as_flattened().as_flattened()
    }

    /// Returns a flat, mutable slice of histogram bins.
    #[inline]
    pub fn as_flattened_mut(&mut self) -> &mut [T] {
        self.0.as_flattened_mut().as_flattened_mut()
    }
}

impl<const B1: usize, const B2: usize, const B3: usize> Histogram3<u8, B1, B2, B3> {
    /// Create a new lookup [`Histogram`] of indices into the given `cubes` heap.
    pub fn from_cubes(cubes: BinaryHeap<CubeVar<3>>) -> Self {
        debug_assert!(cubes.len() <= PaletteSize::MAX.as_usize());
        let mut hist = Self::new();
        for (i, CubeVar(Cube { min, max }, _)) in cubes.into_iter().enumerate() {
            for r in min[0]..max[0] {
                for g in min[1]..max[1] {
                    for b in min[2]..max[2] {
                        #[allow(clippy::cast_possible_truncation)] // See assert above.
                        {
                            hist[[r, g, b]] = i as u8;
                        }
                    }
                }
            }
        }
        hist
    }
}

impl<T: Zero + Zeroable + PartialEq, const B1: usize, const B2: usize, const B3: usize> Default
    for Histogram3<T, B1, B2, B3>
{
    #[inline]
    fn default() -> Self {
        Self::new()
    }
}

impl<T: Zero + Copy + AddAssign, const B1: usize, const B2: usize, const B3: usize>
    Histogram3<T, B1, B2, B3>
{
    /// Create moments from the histogram bins to allow inclusion-exclusion lookups/calculations.
    pub fn calc_cumulative_moments(&mut self) {
        #[allow(clippy::cast_possible_truncation)]
        for r in 0..(B1 as u8) {
            let area = &mut [T::zero(); B3];

            for g in 0..(B2 as u8) {
                let mut line = T::zero();

                for b in 0..(B3 as u8) {
                    let area = &mut area[usize::from(b)];
                    line += self[[r, g, b]];
                    *area += line;

                    // compiler should hoist/remove the following if statement
                    if r == 0 {
                        self[[r, g, b]] = *area;
                    } else {
                        self[[r, g, b]] = self[[r - 1, g, b]] + *area;
                    }
                }
            }
        }
    }
}

#[cfg(feature = "threads")]
impl<T, const B1: usize, const B2: usize, const B3: usize> Histogram3<T, B1, B2, B3>
where
    T: AddAssign + Copy,
{
    /// Merge multiple [`Histogram3`]s together by element-wise summing their histogram bins together.
    #[allow(clippy::needless_pass_by_value)]
    pub fn merge_partial(mut a: Self, b: Self) -> Self {
        for (a, &b) in a.as_flattened_mut().iter_mut().zip(b.as_flattened()) {
            *a += b;
        }
        a
    }
}

/// Trait for types that allow inclusion-exclusion lookups for any `N` dimensional slice.
pub trait InclusionExclusion<T, const N: usize> {
    /// Returns the number of bins in each dimension.
    fn dims(&self) -> [u8; N];

    /// Returns the sum of the histogram bins specified by the given cube.
    fn volume(&self, cube: Cube<N>) -> T;

    /// Returns the sum of the histogram bins specified by the given cube
    /// but with one of the dimensions fixed to the given bin.
    fn volume_at(&self, cube: Cube<N>, dim: u8, bin: u8) -> T;
}

impl<H: InclusionExclusion<T, N>, T, const N: usize> InclusionExclusion<T, N> for &H {
    fn dims(&self) -> [u8; N] {
        (*self).dims()
    }

    fn volume(&self, cube: Cube<N>) -> T {
        (*self).volume(cube)
    }

    fn volume_at(&self, cube: Cube<N>, dim: u8, bin: u8) -> T {
        (*self).volume_at(cube, dim, bin)
    }
}

/// This macro generates code for a fixed number of recursive calls to a volume function.
macro_rules! ndvolume {
    ($self: ident, $min: ident, $max: ident, $index: ident; $n: literal $(, $ns: literal)* $(,)?) => {{
        $index[$n] = $max[$n] - 1;
        let upper = ndvolume!($self, $min, $max, $index; $($ns,)*);

        let lower = if $min[$n] == 0 {
            Zero::zero()
        } else {
            $index[$n] = $min[$n] - 1;
            ndvolume!($self, $min, $max, $index; $($ns,)*)
        };

        upper - lower
    }};
    ($self: ident, $min: ident, $max: ident, $index: ident;) => {
        $self[$index]
    };
}

impl<T, const B1: usize, const B2: usize, const B3: usize> InclusionExclusion<T, 3>
    for Histogram3<T, B1, B2, B3>
where
    T: Copy + Zero + Add<Output = T> + Sub<Output = T>,
{
    #[allow(clippy::cast_possible_truncation)]
    fn dims(&self) -> [u8; 3] {
        [B1, B2, B3].map(|b| b as u8)
    }

    fn volume(&self, Cube { min, max }: Cube<3>) -> T {
        let mut index = [0u8; 3];
        ndvolume!(self, min, max, index; 0, 1, 2)
    }

    fn volume_at(&self, Cube { min, max }: Cube<3>, dim: u8, bin: u8) -> T {
        if bin == 0 {
            Zero::zero()
        } else {
            let bin = bin - 1;
            let mut index = [0u8; 3];
            match dim {
                0 => {
                    index[0] = bin;
                    ndvolume!(self, min, max, index; 1, 2)
                }
                1 => {
                    index[1] = bin;
                    ndvolume!(self, min, max, index; 0, 2)
                }
                2 => {
                    index[2] = bin;
                    ndvolume!(self, min, max, index; 0, 1)
                }
                #[allow(clippy::panic)]
                _ => panic!("dim must be < 3"),
            }
        }
    }
}

/// Wrapper struct to run Wu quantization on histograms.
pub struct Wu<Hist, Color, Component, Stats> {
    /// The histogram.
    hist: Hist,
    /// The color type.
    color: PhantomData<Color>,
    /// The color component type.
    component: PhantomData<Component>,
    /// The histogram stats type.
    stats: PhantomData<Stats>,
}

impl<Hist, Color, Component, const N: usize, Sum, Count>
    Wu<Hist, Color, Component, Stats<Sum, N, Count>>
where
    Color: ColorComponents<Component, N>,
    Component: Copy + 'static,
    Sum: NumOps + AsPrimitive<f64> + AsPrimitive<Component>,
    Count: Zero + Add<Output = Count> + Sub<Output = Count> + AsPrimitive<u32>,
    u32: Into<Sum> + Into<Count>,
    Hist: InclusionExclusion<Stats<Sum, N, Count>, N>,
{
    /// Create a new [`Wu`] from a histogram.
    #[inline]
    pub fn new(hist: Hist) -> Self {
        Self {
            hist,
            color: PhantomData,
            component: PhantomData,
            stats: PhantomData,
        }
    }

    /// Returns the sum of the histogram bins specified by the given cube.
    #[inline]
    fn volume(&self, cube: Cube<N>) -> Stats<Sum, N, Count> {
        self.hist.volume(cube)
    }

    /// Returns the sum of the histogram bins specified by the given cube
    /// but with one of the dimensions fixed to the given bin.
    #[inline]
    fn volume_at(&self, cube: Cube<N>, dim: u8, bin: u8) -> Stats<Sum, N, Count> {
        self.hist.volume_at(cube, dim, bin)
    }

    /// Compute the variance of the given cube.
    fn variance(&self, cube: Cube<N>) -> f64 {
        if (0..N).all(|c| cube.max[c] - cube.min[c] == 1) {
            0.0
        } else {
            let Stats { count, components, sum_squared } = self.volume(cube);
            sum_squared
                - sum_of_squares(components) / f64::from(<Count as AsPrimitive<u32>>::as_(count))
        }
    }

    /// Attempts to cut the given cube to give a lower variance.
    fn cut(&self, cube: Cube<N>) -> Option<(Cube<N>, Cube<N>)> {
        let sum = self.volume(cube);

        #[allow(clippy::cast_possible_truncation)]
        (0..(N as u8))
            .filter_map(|dim| {
                let d = usize::from(dim);
                let bottom = cube.min[d];
                let top = cube.max[d];
                let base = self.volume_at(cube, dim, bottom);

                ((bottom + 1)..top)
                    .filter_map(|bin| {
                        let upper = self.volume_at(cube, dim, bin) - base;
                        let lower = sum - upper;
                        if upper.count.is_zero() || lower.count.is_zero() {
                            None
                        } else {
                            let upper2 =
                                sum_of_squares(upper.components) / f64::from(upper.count.as_());
                            let lower2 =
                                sum_of_squares(lower.components) / f64::from(lower.count.as_());
                            Some(((d, bin), -(upper2 + lower2)))
                        }
                    })
                    .min_by_key(|&(_, v)| OrderedFloat(v))
            })
            .min_by_key(|&(_, v)| OrderedFloat(v))
            .map(|((d, cut), _)| {
                let mut cube1 = cube;
                let mut cube2 = cube;
                cube1.max[d] = cut;
                cube2.min[d] = cut;
                (cube1, cube2)
            })
    }

    /// Returns the disjoint cubes resulting from Wu's color quantization method.
    fn cubes_heap(&self, k: PaletteSize) -> BinaryHeap<CubeVar<N>> {
        let whole_cube = Cube { min: [0; N], max: self.hist.dims() };
        if self.volume(whole_cube).count.is_zero() {
            return BinaryHeap::new();
        }

        let k = k.into();

        let mut queue = BinaryHeap::with_capacity(k);
        queue.push(CubeVar(whole_cube, f64::INFINITY));

        while queue.len() < k {
            // there should always be one cube, since at least one cube is added back for each popped
            #[allow(clippy::expect_used)]
            let CubeVar(cube, variance) = queue.pop().expect("at least one cube");

            if variance.partial_cmp(&0.0).is_none_or(Ordering::is_le) {
                // all cubes cannot be cut further
                queue.push(CubeVar(cube, variance));
                break;
            }

            if let Some((cube1, cube2)) = self.cut(cube) {
                queue.push(CubeVar(cube1, self.variance(cube1)));
                queue.push(CubeVar(cube2, self.variance(cube2)));
            } else {
                queue.push(CubeVar(cube, 0.0));
            }
        }

        queue
    }

    /// Returns the average color of the given cube and the number of colors in the cube.
    fn cube_color_and_count(&self, cube: Cube<N>) -> (Color, u32) {
        let Stats { count, components, .. } = self.volume(cube);
        debug_assert!(!count.is_zero());
        let count = count.as_();
        let n = count.into();
        let color = cast::from_array(components.map(|c| (c / n).as_()));
        (color, count)
    }

    /// Compute the color palette and also returns the histogram cubes.
    pub fn palette_and_cubes(&self, k: PaletteSize) -> (PaletteBuf<Color>, BinaryHeap<CubeVar<N>>) {
        let cubes = self.cubes_heap(k);
        let palette = PaletteBuf::new_unchecked(
            cubes
                .iter()
                .map(|&CubeVar(cube, _)| self.cube_color_and_count(cube).0)
                .collect(),
        );
        (palette, cubes)
    }

    /// Computes the color palette, counts for each color, and also returns the histogram cubes.
    pub fn palette_counts_and_cubes(
        &self,
        k: PaletteSize,
    ) -> (PaletteBuf<Color>, PaletteBuf<u32>, BinaryHeap<CubeVar<N>>) {
        let cubes = self.cubes_heap(k);
        let (palette, counts) = cubes
            .iter()
            .map(|&CubeVar(cube, _)| self.cube_color_and_count(cube))
            .unzip();

        (
            PaletteBuf::new_unchecked(palette),
            PaletteBuf::new_unchecked(counts),
            cubes,
        )
    }

    /// Compute the color palette.
    pub fn palette(&self, k: PaletteSize) -> PaletteBuf<Color> {
        self.palette_and_cubes(k).0
    }

    /// Compute the color palette and counts for each color.
    pub fn palette_and_counts(&self, k: PaletteSize) -> (PaletteBuf<Color>, PaletteBuf<u32>) {
        let (palette, counts, _) = self.palette_counts_and_cubes(k);
        (palette, counts)
    }
}

/// Returns the sum of the squares of the given components.
#[inline]
pub fn sum_of_squares<T: AsPrimitive<f64>, const N: usize>(components: [T; N]) -> f64 {
    let mut square = 0.0;
    for c in components {
        let c = c.as_();
        square += c * c;
    }
    square
}
