use crate::{LengthOutOfRange, MAX_PIXELS};
use bytemuck::Zeroable;
use core::{
    borrow::{Borrow, BorrowMut},
    fmt::Debug,
    ops::{Deref, DerefMut, Index, IndexMut},
};
use num_traits::AsPrimitive;
use ref_cast::{RefCastCustom, ref_cast_custom};

pub(super) mod traits {
    pub trait Sealed {}
}
/// A trait for unsigned integer types that can be used as indices.
pub trait BoundedIndex:
    AsPrimitive<usize> + Zeroable + Copy + Send + Sync + Debug + traits::Sealed + 'static
{
    /// The maximum length supported by this index type.
    const MAX_LEN: usize;
    /// The maximum length supported by this index type.
    const MAX_LENGTH: Self::Length;

    /// The minimal integer type that can represent [`MAX_LEN`](BoundedIndex::MAX_LEN).
    type Length: From<Self> + AsPrimitive<usize> + Copy + Send + Sync + Debug + 'static;
}

impl traits::Sealed for u8 {}

impl BoundedIndex for u8 {
    const MAX_LEN: usize = Self::MAX_LENGTH as usize;
    const MAX_LENGTH: u16 = u8::MAX as u16 + 1;

    type Length = u16;
}

impl traits::Sealed for u16 {}

impl BoundedIndex for u16 {
    const MAX_LEN: usize = Self::MAX_LENGTH as usize;
    const MAX_LENGTH: u32 = u16::MAX as u32 + 1;

    type Length = u32;
}

impl traits::Sealed for u32 {}

impl BoundedIndex for u32 {
    const MAX_LEN: usize = Self::MAX_LENGTH as usize;
    const MAX_LENGTH: u32 = u32::MAX;

    type Length = u32;
}

/// A non-empty slice with a length less than or equal to [`MAX_PIXELS`].
#[derive(Debug, PartialEq, Eq, PartialOrd, Ord, Hash, RefCastCustom)]
#[repr(transparent)]
pub(crate) struct BoundedSlice<T>([T]);

impl<T> BoundedSlice<T> {
    /// The minimum length of a [`BoundedSlice`], which is `1`.
    pub const MIN_LENGTH: u32 = 1;

    /// The minimum length of a [`BoundedSlice`], which is `1`.
    pub const MIN_LEN: usize = Self::MIN_LENGTH as usize;

    /// The maximum length of a [`BoundedSlice`], which is [`MAX_PIXELS`].
    pub const MAX_LENGTH: u32 = MAX_PIXELS;

    /// The maximum length of a [`BoundedSlice`], which is [`MAX_PIXELS`].
    pub const MAX_LEN: usize = Self::MAX_LENGTH as usize;

    /// Create a new [`BoundedSlice`] without checking invariants.
    #[inline]
    pub(crate) const fn new_unchecked(slice: &[T]) -> &BoundedSlice<T> {
        #[ref_cast_custom]
        #[inline]
        const fn cast<T>(slice: &[T]) -> &BoundedSlice<T>;

        #[cfg_attr(target_pointer_width = "32", allow(clippy::absurd_extreme_comparisons))]
        {
            debug_assert!(Self::MIN_LEN <= slice.len() && slice.len() <= Self::MAX_LEN);
        }
        cast(slice)
    }

    /// Create a new [`BoundedSlice`].
    #[inline]
    pub const fn new(slice: &[T]) -> Result<&Self, LengthOutOfRange> {
        match LengthOutOfRange::check_u32(slice, Self::MIN_LENGTH, Self::MAX_LENGTH) {
            Ok(_) => Ok(Self::new_unchecked(slice)),
            Err(err) => Err(err),
        }
    }

    /// Create a new mutable [`BoundedSlice`] without checking invariants.
    #[inline]
    pub(crate) const fn new_unchecked_mut(slice: &mut [T]) -> &mut BoundedSlice<T> {
        #[ref_cast_custom]
        #[inline]
        const fn cast_mut<T>(slice: &mut [T]) -> &mut BoundedSlice<T>;

        #[cfg_attr(target_pointer_width = "32", allow(clippy::absurd_extreme_comparisons))]
        {
            debug_assert!(Self::MIN_LEN <= slice.len() && slice.len() <= Self::MAX_LEN);
        }
        cast_mut(slice)
    }

    /// Create a new mutable [`BoundedSlice`].
    #[inline]
    pub const fn new_mut(slice: &mut [T]) -> Result<&mut Self, LengthOutOfRange> {
        match LengthOutOfRange::check_u32(slice, Self::MIN_LENGTH, Self::MAX_LENGTH) {
            Ok(_) => Ok(Self::new_unchecked_mut(slice)),
            Err(err) => Err(err),
        }
    }

    /// Returns the length of a [`BoundedSlice`] as a `u32`.
    #[allow(clippy::cast_possible_truncation)]
    #[must_use]
    #[inline]
    pub const fn length(&self) -> u32 {
        self.as_slice().len() as u32
    }

    /// Convert a [`BoundedSlice`] to a regular slice.
    #[inline]
    pub const fn as_slice(&self) -> &[T] {
        &self.0
    }

    /// Convert a mutable [`BoundedSlice`] to a regular mutable slice.
    #[inline]
    pub const fn as_mut_slice(&mut self) -> &mut [T] {
        &mut self.0
    }
}

impl<T> Deref for BoundedSlice<T> {
    type Target = [T];

    #[inline]
    fn deref(&self) -> &Self::Target {
        self.as_slice()
    }
}

impl<T> DerefMut for BoundedSlice<T> {
    #[inline]
    fn deref_mut(&mut self) -> &mut Self::Target {
        self.as_mut_slice()
    }
}

impl<T> AsRef<[T]> for BoundedSlice<T> {
    #[inline]
    fn as_ref(&self) -> &[T] {
        self
    }
}

impl<T> AsMut<[T]> for BoundedSlice<T> {
    #[inline]
    fn as_mut(&mut self) -> &mut [T] {
        self
    }
}

impl<T> Borrow<[T]> for BoundedSlice<T> {
    #[inline]
    fn borrow(&self) -> &[T] {
        self
    }
}

impl<T> BorrowMut<[T]> for BoundedSlice<T> {
    #[inline]
    fn borrow_mut(&mut self) -> &mut [T] {
        self
    }
}

impl<'a, T> TryFrom<&'a [T]> for &'a BoundedSlice<T> {
    type Error = LengthOutOfRange;

    #[inline]
    fn try_from(slice: &'a [T]) -> Result<Self, Self::Error> {
        BoundedSlice::new(slice)
    }
}

impl<'a, T> TryFrom<&'a mut [T]> for &'a mut BoundedSlice<T> {
    type Error = LengthOutOfRange;

    #[inline]
    fn try_from(slice: &'a mut [T]) -> Result<Self, Self::Error> {
        BoundedSlice::new_mut(slice)
    }
}

impl<'a, T> IntoIterator for &'a BoundedSlice<T> {
    type Item = &'a T;

    type IntoIter = <&'a [T] as IntoIterator>::IntoIter;

    #[inline]
    fn into_iter(self) -> Self::IntoIter {
        self.as_slice().iter()
    }
}

impl<'a, T> IntoIterator for &'a mut BoundedSlice<T> {
    type Item = &'a mut T;

    type IntoIter = <&'a mut [T] as IntoIterator>::IntoIter;

    #[inline]
    fn into_iter(self) -> Self::IntoIter {
        self.as_mut_slice().iter_mut()
    }
}

impl<T> Index<usize> for BoundedSlice<T> {
    type Output = T;

    #[inline]
    fn index(&self, index: usize) -> &Self::Output {
        &self.as_slice()[index]
    }
}

impl<T> Index<u32> for BoundedSlice<T> {
    type Output = T;

    #[inline]
    fn index(&self, index: u32) -> &Self::Output {
        &self[index as usize]
    }
}

impl<T> IndexMut<usize> for BoundedSlice<T> {
    #[inline]
    fn index_mut(&mut self, index: usize) -> &mut Self::Output {
        &mut self.as_mut_slice()[index]
    }
}

impl<T> IndexMut<u32> for BoundedSlice<T> {
    #[inline]
    fn index_mut(&mut self, index: u32) -> &mut Self::Output {
        &mut self[index as usize]
    }
}
