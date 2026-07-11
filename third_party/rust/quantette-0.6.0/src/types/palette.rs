use crate::LengthOutOfRange;
use alloc::{borrow::ToOwned, vec::Vec};
use core::{
    borrow::{Borrow, BorrowMut},
    error::Error,
    fmt::{self, Debug},
    num::{NonZeroU8, NonZeroU16},
    ops::{Deref, DerefMut, Index, IndexMut},
};
use ref_cast::{RefCastCustom, ref_cast_custom};

/// The error returned when attempting to convert an out of range integer into a [`PaletteSize`].
#[derive(Debug, Copy, Clone, PartialEq, Eq)]
pub struct PaletteSizeFromIntError(());

impl fmt::Display for PaletteSizeFromIntError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str("out of range conversion from integer to palette size")
    }
}
impl Error for PaletteSizeFromIntError {}

/// This type is used to specify the number of colors in a palette.
///
/// This is a simple new type wrapper around `u16` with the invariant that it must be
/// in the range `1..=256` specified by [`PaletteSize::MIN`] and [`PaletteSize::MAX`].
///
/// # Examples
///
/// A [`PaletteSize`] can be created from a `u8`, `u16`, `usize`, [`NonZeroU8`], or [`NonZeroU16`].
/// To do so, use either:
/// - The clamping functions like [`from_usize_clamped`](`PaletteSize::from_usize_clamped`).
/// - The `TryFrom` trait implementations for [`PaletteSize`].
///   - There are also `const` compatible functions like [`try_from_u16`](PaletteSize::try_from_u16).
///
/// You can also use the [`PaletteSize::MIN`] or [`PaletteSize::MAX`] constants.
///
/// ```
/// # use core::num::{NonZeroU16, NonZeroU8};
/// # use quantette::{PaletteSize, PaletteSizeFromIntError};
/// # fn main() -> Result<(), PaletteSizeFromIntError> {
/// let size: PaletteSize = 64u16.try_into()?;
/// assert_eq!(size, 64u16);
/// assert_eq!(PaletteSize::try_from(16usize)?, 16usize);
/// assert_eq!(PaletteSize::try_from_u16(256), Some(PaletteSize::MAX));
/// assert_eq!(PaletteSize::try_from_u16(1024), None);
/// assert_eq!(PaletteSize::from_u16_clamped(1024), PaletteSize::MAX);
/// assert_eq!(PaletteSize::from_nz_u8(NonZeroU8::MIN), PaletteSize::MIN);
/// # Ok(())
/// # }
/// ```
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
#[repr(transparent)]
pub struct PaletteSize(NonZeroU16);

impl PaletteSize {
    /// The smallest possible palette size, which is `1`.
    pub const MIN: Self = Self(NonZeroU16::MIN);

    /// The largest possible palette size, which is `256`.
    pub const MAX: Self = Self(NonZeroU16::new(u8::MAX as u16 + 1).unwrap());

    /// Returns a [`PaletteSize`] as a [`NonZeroU16`].
    #[inline]
    pub const fn as_nz_u16(&self) -> NonZeroU16 {
        self.0
    }

    /// Returns a [`PaletteSize`] as a `u16`.
    #[inline]
    pub const fn as_u16(&self) -> u16 {
        self.as_nz_u16().get()
    }

    /// Returns a [`PaletteSize`] as a `usize`.
    #[inline]
    pub const fn as_usize(&self) -> usize {
        self.as_u16() as usize
    }

    /// Create a [`PaletteSize`] directly from the given [`NonZeroU16`]
    /// without ensuring that it is less than or equal to [`PaletteSize::MAX`].
    #[inline]
    const fn new_unchecked(value: NonZeroU16) -> Self {
        debug_assert!(value.get() <= Self::MAX.as_u16());
        Self(value)
    }

    /// Create a [`PaletteSize`] from a [`NonZeroU16`], returning `None` if the provided `value`
    /// is greater than [`PaletteSize::MAX`].
    #[must_use]
    #[inline]
    pub const fn try_from_nz_u16(value: NonZeroU16) -> Option<Self> {
        if value.get() <= Self::MAX.as_u16() {
            Some(Self::new_unchecked(value))
        } else {
            None
        }
    }

    /// Create a [`PaletteSize`] from a `u16`, returning `None` if the provided `value`
    /// is less than [`PaletteSize::MIN`] or greater than [`PaletteSize::MAX`].
    #[must_use]
    #[inline]
    pub const fn try_from_u16(value: u16) -> Option<Self> {
        if let Some(len) = NonZeroU16::new(value) {
            Self::try_from_nz_u16(len)
        } else {
            None
        }
    }

    /// Create a [`PaletteSize`] from a `usize`, returning `None` if the provided `value`
    /// is less than [`PaletteSize::MIN`] or greater than [`PaletteSize::MAX`].
    #[must_use]
    #[inline]
    pub const fn try_from_usize(value: usize) -> Option<Self> {
        if value <= Self::MAX.as_usize() {
            #[allow(clippy::cast_possible_truncation)]
            if let Some(len) = NonZeroU16::new(value as u16) {
                Some(Self::new_unchecked(len))
            } else {
                None
            }
        } else {
            None
        }
    }

    /// Create a [`PaletteSize`] from a [`NonZeroU16`], clamping the provided `value` to
    /// a maximum of [`PaletteSize::MAX`].
    #[must_use]
    #[inline]
    pub const fn from_nz_u16_clamped(value: NonZeroU16) -> Self {
        if let Some(size) = Self::try_from_nz_u16(value) {
            size
        } else {
            Self::MAX
        }
    }

    /// Create a [`PaletteSize`] from a `u16`, clamping the provided `value` to
    /// the range specified by [`PaletteSize::MIN`] and [`PaletteSize::MAX`].
    #[must_use]
    #[inline]
    pub const fn from_u16_clamped(value: u16) -> Self {
        if let Some(len) = NonZeroU16::new(value) {
            Self::from_nz_u16_clamped(len)
        } else {
            Self::MIN
        }
    }

    /// Create a [`PaletteSize`] from a `usize`, clamping the provided `value` to
    /// the range specified by [`PaletteSize::MIN`] and [`PaletteSize::MAX`].
    #[must_use]
    #[inline]
    pub const fn from_usize_clamped(value: usize) -> Self {
        if value <= Self::MAX.as_usize() {
            #[allow(clippy::cast_possible_truncation)]
            if let Some(len) = NonZeroU16::new(value as u16) {
                Self::new_unchecked(len)
            } else {
                Self::MIN
            }
        } else {
            Self::MAX
        }
    }

    /// Create a [`PaletteSize`] from a `u8`, returning `None` if the provided `value`
    /// is less than [`PaletteSize::MIN`].
    #[must_use]
    #[inline]
    pub const fn try_from_u8(value: u8) -> Option<Self> {
        if let Some(len) = NonZeroU8::new(value) {
            Some(Self::from_nz_u8(len))
        } else {
            None
        }
    }

    /// Create a [`PaletteSize`] from a [`NonZeroU8`].
    #[allow(clippy::expect_used, clippy::missing_panics_doc)] // compiler removes the `expect` with opt_level=3
    #[must_use]
    #[inline]
    pub const fn from_nz_u8(len: NonZeroU8) -> Self {
        Self::new_unchecked(
            NonZeroU16::new(len.get() as u16).expect("nonzero u8 to be nonzero u16"),
        )
    }

    /// Create a [`PaletteSize`] from a `u8`, clamping the provided `value` to
    /// a minimum of [`PaletteSize::MIN`].
    #[must_use]
    #[inline]
    pub const fn from_u8_clamped(len: u8) -> Self {
        if let Some(size) = Self::try_from_u8(len) {
            size
        } else {
            Self::MIN
        }
    }
}

impl From<PaletteSize> for NonZeroU16 {
    #[inline]
    fn from(size: PaletteSize) -> Self {
        size.as_nz_u16()
    }
}

impl From<PaletteSize> for u16 {
    #[inline]
    fn from(size: PaletteSize) -> Self {
        size.as_u16()
    }
}

impl From<PaletteSize> for usize {
    #[inline]
    fn from(size: PaletteSize) -> Self {
        size.as_usize()
    }
}

impl TryFrom<NonZeroU16> for PaletteSize {
    type Error = PaletteSizeFromIntError;

    #[inline]
    fn try_from(value: NonZeroU16) -> Result<Self, Self::Error> {
        Self::try_from_nz_u16(value).ok_or(PaletteSizeFromIntError(()))
    }
}

impl TryFrom<u16> for PaletteSize {
    type Error = PaletteSizeFromIntError;

    #[inline]
    fn try_from(value: u16) -> Result<Self, Self::Error> {
        Self::try_from_u16(value).ok_or(PaletteSizeFromIntError(()))
    }
}

impl TryFrom<usize> for PaletteSize {
    type Error = PaletteSizeFromIntError;

    #[inline]
    fn try_from(value: usize) -> Result<Self, Self::Error> {
        Self::try_from_usize(value).ok_or(PaletteSizeFromIntError(()))
    }
}

impl From<NonZeroU8> for PaletteSize {
    #[inline]
    fn from(value: NonZeroU8) -> Self {
        Self::from_nz_u8(value)
    }
}

impl TryFrom<u8> for PaletteSize {
    type Error = PaletteSizeFromIntError;

    #[inline]
    fn try_from(value: u8) -> Result<Self, Self::Error> {
        Self::try_from_u8(value).ok_or(PaletteSizeFromIntError(()))
    }
}

impl PartialEq<NonZeroU16> for PaletteSize {
    #[inline]
    fn eq(&self, other: &NonZeroU16) -> bool {
        self.as_nz_u16() == *other
    }
}

impl PartialEq<PaletteSize> for NonZeroU16 {
    #[inline]
    fn eq(&self, other: &PaletteSize) -> bool {
        *self == other.as_nz_u16()
    }
}

impl PartialEq<u16> for PaletteSize {
    #[inline]
    fn eq(&self, other: &u16) -> bool {
        self.as_u16() == *other
    }
}

impl PartialEq<PaletteSize> for u16 {
    #[inline]
    fn eq(&self, other: &PaletteSize) -> bool {
        *self == other.as_u16()
    }
}

impl PartialEq<usize> for PaletteSize {
    #[inline]
    fn eq(&self, other: &usize) -> bool {
        self.as_usize() == *other
    }
}

impl PartialEq<PaletteSize> for usize {
    #[inline]
    fn eq(&self, other: &PaletteSize) -> bool {
        *self == other.as_usize()
    }
}

impl fmt::Display for PaletteSize {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let Self(size) = *self;
        write!(f, "{size}")
    }
}

/// A [`Palette`] is a slice with a length in the range `1..=256` specified by
/// [`Palette::MIN_LEN`] and [`Palette::MAX_LEN`].
///
/// This is an unsized type, meaning that it must always be used behind a pointer like `&`.
/// The owned version of a [`Palette`] is a [`PaletteBuf`].
///
/// See also the [`PaletteSize`] struct which is a `u16` in the range `1..=256`.
///
/// # Examples
///
/// ```
/// # use quantette::{Palette, LengthOutOfRange};
/// # use palette::Srgb;
/// # fn main() -> Result<(), LengthOutOfRange> {
/// let mut data = vec![Srgb::new(0, 0, 0)];
/// let palette = Palette::new(&data)?;
/// assert_eq!(&data, palette);
///
/// let len = palette.len();
/// let size = palette.size();
/// assert_eq!(len, size);
///
/// let first = palette[0u8];
/// assert_eq!(first, Srgb::new(0, 0, 0));
/// # Ok(())
/// # }
/// ```
#[derive(Debug, PartialEq, Eq, PartialOrd, Ord, Hash, RefCastCustom)]
#[repr(transparent)]
pub struct Palette<T>([T]);

impl<T> Palette<T> {
    /// The maximum length of a [`Palette`], which is 256.
    pub const MAX_LENGTH: u16 = PaletteSize::MAX.as_u16();
    /// The maximum length of a [`Palette`], which is 256.
    pub const MAX_LEN: usize = Self::MAX_LENGTH as usize;

    /// The minimum length of a [`Palette`], which is 1.
    pub const MIN_LENGTH: u16 = PaletteSize::MIN.as_u16();
    /// The minimum length of a [`Palette`], which is 1.
    pub const MIN_LEN: usize = Self::MIN_LENGTH as usize;

    /// Create a [`Palette`] without validating any invariants.
    #[inline]
    pub(crate) const fn new_unchecked(slice: &[T]) -> &Palette<T> {
        #[ref_cast_custom]
        #[inline]
        const fn cast<T>(slice: &[T]) -> &Palette<T>;

        debug_assert!(Self::MIN_LEN <= slice.len() && slice.len() <= Self::MAX_LEN);
        cast(slice)
    }

    /// Create a new [`Palette`] reference from a slice.
    ///
    /// # Errors
    ///
    /// Returns an error if the length of `slice` is not in the range `1..=256` specified by
    /// [`Palette::MIN_LEN`] and [`Palette::MAX_LEN`].
    #[inline]
    pub const fn new(slice: &[T]) -> Result<&Self, LengthOutOfRange> {
        match LengthOutOfRange::check_u16(slice, Self::MIN_LENGTH, Self::MAX_LENGTH) {
            Ok(_) => Ok(Self::new_unchecked(slice)),
            Err(err) => Err(err),
        }
    }

    /// Create a mutable [`Palette`] without validating any invariants.
    #[inline]
    pub(crate) const fn new_mut_unchecked(slice: &mut [T]) -> &mut Palette<T> {
        #[ref_cast_custom]
        #[inline]
        const fn cast_mut<T>(slice: &mut [T]) -> &mut Palette<T>;

        debug_assert!(Self::MIN_LEN <= slice.len() && slice.len() <= Self::MAX_LEN);
        cast_mut(slice)
    }

    /// Create a new mutable [`Palette`] reference from a mutable slice.
    ///
    /// # Errors
    ///
    /// Returns an error if the length of `slice` is not in the range `1..=256` specified by
    /// [`Palette::MIN_LEN`] and [`Palette::MAX_LEN`].
    #[inline]
    pub const fn new_mut(slice: &mut [T]) -> Result<&mut Self, LengthOutOfRange> {
        match LengthOutOfRange::check_u16(slice, Self::MIN_LENGTH, Self::MAX_LENGTH) {
            Ok(_) => Ok(Self::new_mut_unchecked(slice)),
            Err(err) => Err(err),
        }
    }

    /// Create a [`Palette`] from an array.
    ///
    /// This function will cause a compile error if the length of the array is not in the range
    /// `1..=256` specified by [`Palette::MIN_LEN`] and [`Palette::MAX_LEN`].
    #[inline]
    pub const fn from_array<const N: usize>(array: &[T; N]) -> &Self {
        const { assert!(Self::MIN_LEN <= N && N <= Self::MAX_LEN) };
        Self::new_unchecked(array.as_slice())
    }

    /// Create a mutable [`Palette`] from a mutable array.
    ///
    /// This function will cause a compile error if the length of the array is not in the range
    /// `1..=256` specified by [`Palette::MIN_LEN`] and [`Palette::MAX_LEN`].
    #[inline]
    pub fn from_mut_array<const N: usize>(array: &mut [T; N]) -> &mut Self {
        const { assert!(Self::MIN_LEN <= N && N <= Self::MAX_LEN) };
        Self::new_mut_unchecked(array.as_mut_slice())
    }

    /// Returns the length of a [`Palette`] as a [`PaletteSize`].
    #[allow(clippy::missing_panics_doc)]
    #[must_use]
    #[inline]
    pub const fn size(&self) -> PaletteSize {
        let len = self.as_slice().len();
        debug_assert!(Self::MIN_LEN <= len && len <= Self::MAX_LEN);
        #[allow(clippy::expect_used, clippy::cast_possible_truncation)]
        let size = NonZeroU16::new(len as u16).expect("non-empty palette");
        PaletteSize::new_unchecked(size)
    }

    /// Convert a [`Palette`] to a slice.
    #[inline]
    pub const fn as_slice(&self) -> &[T] {
        &self.0
    }

    /// Convert a mutable [`Palette`] to a mutable slice.
    #[inline]
    pub const fn as_mut_slice(&mut self) -> &mut [T] {
        &mut self.0
    }

    /// Map each [`Palette`] color to a new color to create a new [`PaletteBuf`].
    #[must_use]
    #[inline]
    pub fn map_ref<U>(&self, mapping: impl FnMut(&T) -> U) -> PaletteBuf<U> {
        PaletteBuf::new_unchecked(self.iter().map(mapping).collect())
    }
}

impl<T> Deref for Palette<T> {
    type Target = [T];

    #[inline]
    fn deref(&self) -> &Self::Target {
        self.as_slice()
    }
}

impl<T> DerefMut for Palette<T> {
    #[inline]
    fn deref_mut(&mut self) -> &mut Self::Target {
        self.as_mut_slice()
    }
}

impl<T> AsRef<[T]> for Palette<T> {
    #[inline]
    fn as_ref(&self) -> &[T] {
        self
    }
}

impl<T> AsMut<[T]> for Palette<T> {
    #[inline]
    fn as_mut(&mut self) -> &mut [T] {
        self
    }
}

impl<T> Borrow<[T]> for Palette<T> {
    #[inline]
    fn borrow(&self) -> &[T] {
        self
    }
}

impl<T> BorrowMut<[T]> for Palette<T> {
    #[inline]
    fn borrow_mut(&mut self) -> &mut [T] {
        self
    }
}

impl<T: Clone> ToOwned for Palette<T> {
    type Owned = PaletteBuf<T>;

    fn to_owned(&self) -> Self::Owned {
        PaletteBuf::new_unchecked(self.as_slice().to_vec())
    }
}

impl<'a, T> TryFrom<&'a [T]> for &'a Palette<T> {
    type Error = LengthOutOfRange;

    #[inline]
    fn try_from(slice: &'a [T]) -> Result<Self, Self::Error> {
        Palette::new(slice)
    }
}

impl<'a, T> TryFrom<&'a mut [T]> for &'a mut Palette<T> {
    type Error = LengthOutOfRange;

    #[inline]
    fn try_from(slice: &'a mut [T]) -> Result<Self, Self::Error> {
        Palette::new_mut(slice)
    }
}

impl<'a, T> IntoIterator for &'a Palette<T> {
    type Item = &'a T;

    type IntoIter = <&'a [T] as IntoIterator>::IntoIter;

    #[inline]
    fn into_iter(self) -> Self::IntoIter {
        self.as_slice().iter()
    }
}

impl<'a, T> IntoIterator for &'a mut Palette<T> {
    type Item = &'a mut T;

    type IntoIter = <&'a mut [T] as IntoIterator>::IntoIter;

    #[inline]
    fn into_iter(self) -> Self::IntoIter {
        self.as_mut_slice().iter_mut()
    }
}

impl<T> Index<usize> for Palette<T> {
    type Output = T;

    #[inline]
    fn index(&self, index: usize) -> &Self::Output {
        &self.as_slice()[index]
    }
}

impl<T> Index<u8> for Palette<T> {
    type Output = T;

    #[inline]
    fn index(&self, index: u8) -> &Self::Output {
        &self[usize::from(index)]
    }
}

impl<T> IndexMut<usize> for Palette<T> {
    #[inline]
    fn index_mut(&mut self, index: usize) -> &mut Self::Output {
        &mut self.as_mut_slice()[index]
    }
}

impl<T> IndexMut<u8> for Palette<T> {
    #[inline]
    fn index_mut(&mut self, index: u8) -> &mut Self::Output {
        &mut self[usize::from(index)]
    }
}

impl<T, U> PartialEq<Palette<U>> for [T]
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &Palette<U>) -> bool {
        self == other.as_slice()
    }
}

impl<T, U> PartialEq<[U]> for Palette<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &[U]) -> bool {
        self.as_slice() == other
    }
}

impl<T, U> PartialEq<&Palette<U>> for [T]
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &&Palette<U>) -> bool {
        self == other.as_slice()
    }
}

impl<T, U> PartialEq<[U]> for &Palette<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &[U]) -> bool {
        self.as_slice() == other
    }
}

impl<T, U> PartialEq<&mut Palette<U>> for [T]
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &&mut Palette<U>) -> bool {
        self == other.as_slice()
    }
}

impl<T, U> PartialEq<[U]> for &mut Palette<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &[U]) -> bool {
        self.as_slice() == other
    }
}

impl<T, U> PartialEq<Palette<U>> for &[T]
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &Palette<U>) -> bool {
        *self == other.as_slice()
    }
}

impl<T, U> PartialEq<&[U]> for Palette<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &&[U]) -> bool {
        self.as_slice() == *other
    }
}

impl<T, U> PartialEq<Palette<U>> for &mut [T]
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &Palette<U>) -> bool {
        *self == other.as_slice()
    }
}

impl<T, U> PartialEq<&mut [U]> for Palette<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &&mut [U]) -> bool {
        self.as_slice() == *other
    }
}

impl<T, const N: usize, U> PartialEq<Palette<U>> for [T; N]
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &Palette<U>) -> bool {
        &self[..] == other.as_slice()
    }
}

impl<T, U, const N: usize> PartialEq<[U; N]> for Palette<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &[U; N]) -> bool {
        self.as_slice() == &other[..]
    }
}

impl<T, const N: usize, U> PartialEq<&Palette<U>> for [T; N]
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &&Palette<U>) -> bool {
        &self[..] == other.as_slice()
    }
}

impl<T, U, const N: usize> PartialEq<[U; N]> for &Palette<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &[U; N]) -> bool {
        self.as_slice() == &other[..]
    }
}

impl<T, const N: usize, U> PartialEq<&mut Palette<U>> for [T; N]
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &&mut Palette<U>) -> bool {
        &self[..] == other.as_slice()
    }
}

impl<T, U, const N: usize> PartialEq<[U; N]> for &mut Palette<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &[U; N]) -> bool {
        self.as_slice() == &other[..]
    }
}

impl<T, U> PartialEq<Palette<U>> for Vec<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &Palette<U>) -> bool {
        self.as_slice() == other.as_slice()
    }
}

impl<T, U> PartialEq<Vec<U>> for Palette<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &Vec<U>) -> bool {
        self.as_slice() == other.as_slice()
    }
}

impl<T, U> PartialEq<&Palette<U>> for Vec<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &&Palette<U>) -> bool {
        self.as_slice() == other.as_slice()
    }
}

impl<T, U> PartialEq<Vec<U>> for &Palette<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &Vec<U>) -> bool {
        self.as_slice() == other.as_slice()
    }
}

impl<T, U> PartialEq<&mut Palette<U>> for Vec<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &&mut Palette<U>) -> bool {
        self.as_slice() == other.as_slice()
    }
}

impl<T, U> PartialEq<Vec<U>> for &mut Palette<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &Vec<U>) -> bool {
        self.as_slice() == other.as_slice()
    }
}

impl<T, U> PartialEq<Palette<U>> for PaletteBuf<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &Palette<U>) -> bool {
        self.as_slice() == other.as_slice()
    }
}

impl<T, U> PartialEq<PaletteBuf<U>> for Palette<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &PaletteBuf<U>) -> bool {
        self.as_slice() == other.as_slice()
    }
}

impl<T, U> PartialEq<&Palette<U>> for PaletteBuf<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &&Palette<U>) -> bool {
        self.as_slice() == other.as_slice()
    }
}

impl<T, U> PartialEq<PaletteBuf<U>> for &Palette<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &PaletteBuf<U>) -> bool {
        self.as_slice() == other.as_slice()
    }
}

impl<T, U> PartialEq<&mut Palette<U>> for PaletteBuf<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &&mut Palette<U>) -> bool {
        self.as_slice() == other.as_slice()
    }
}

impl<T, U> PartialEq<PaletteBuf<U>> for &mut Palette<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &PaletteBuf<U>) -> bool {
        self.as_slice() == other.as_slice()
    }
}

/// The error returned when an [`PaletteBuf`] failed to be created.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CreatePaletteBufError<T>(Vec<T>);

impl<T> CreatePaletteBufError<T> {
    /// Returns a slice reference to the [`Vec`] that was used to try and create the [`PaletteBuf`].
    #[inline]
    pub fn as_slice(&self) -> &[T] {
        &self.0
    }

    /// Returns the [`Vec`] that was used to try and create the [`PaletteBuf`].
    #[must_use]
    #[inline]
    pub fn into_vec(self) -> Vec<T> {
        self.0
    }
}

impl<T> fmt::Display for CreatePaletteBufError<T> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "got an input with length {} which is not in the supported range of {}..={}",
            self.0.len(),
            PaletteBuf::<T>::MIN_LENGTH,
            PaletteBuf::<T>::MAX_LENGTH,
        )
    }
}

impl<T: Debug> Error for CreatePaletteBufError<T> {}

/// A [`PaletteBuf`] is an owned [`Palette`].
///
/// That is, an owned slice with a length in the range `1..=256` specified by
/// [`PaletteBuf::MIN_LEN`] and [`PaletteBuf::MAX_LEN`].
///
/// See also the [`PaletteSize`] struct which is a `u16` in the range `1..=256`.
///
/// # Examples
///
/// ```
/// # use quantette::{PaletteBuf, CreatePaletteBufError};
/// # use palette::Srgb;
/// # fn main() -> Result<(), CreatePaletteBufError<Srgb<u8>>> {
/// let mut data = vec![Srgb::<u8>::new(0, 0, 0)];
/// let palette = PaletteBuf::new(data.clone())?;
/// assert_eq!(data, palette);
///
/// let len = palette.len();
/// let size = palette.size();
/// assert_eq!(len, size);
///
/// let first = palette[0u8];
/// assert_eq!(first, Srgb::new(0, 0, 0));
/// # Ok(())
/// # }
/// ```
#[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord, Hash)]
#[repr(transparent)]
pub struct PaletteBuf<T>(Vec<T>);

impl<T> PaletteBuf<T> {
    /// The maximum length of a [`PaletteBuf`], which is 256.
    pub const MAX_LENGTH: u16 = PaletteSize::MAX.as_u16();
    /// The maximum length of a [`PaletteBuf`], which is 256.
    pub const MAX_LEN: usize = Self::MAX_LENGTH as usize;

    /// The minimum length of a [`PaletteBuf`], which is 1.
    pub const MIN_LENGTH: u16 = PaletteSize::MIN.as_u16();
    /// The minimum length of a [`PaletteBuf`], which is 1.
    pub const MIN_LEN: usize = Self::MIN_LENGTH as usize;

    /// Create a new [`PaletteBuf`] without validating invariants.
    #[inline]
    pub(crate) fn new_unchecked(vec: Vec<T>) -> Self {
        debug_assert!(Self::MIN_LEN <= vec.len() && vec.len() <= Self::MAX_LEN);
        Self(vec)
    }

    /// Create a new [`PaletteBuf`] from a [`Vec`].
    ///
    /// # Errors
    ///
    /// Returns an error if the length of `vec` is not in the range `1..=256` specified by
    /// [`PaletteBuf::MIN_LEN`] and [`PaletteBuf::MAX_LEN`].
    #[inline]
    pub fn new(vec: Vec<T>) -> Result<Self, CreatePaletteBufError<T>> {
        if Self::MIN_LEN <= vec.len() && vec.len() <= Self::MAX_LEN {
            Ok(Self::new_unchecked(vec))
        } else {
            Err(CreatePaletteBufError(vec))
        }
    }

    /// Create a [`PaletteBuf`] from an array.
    ///
    /// This function will cause a compile error if the length of the array is not in the range
    /// `1..=256` specified by [`Palette::MIN_LEN`] and [`Palette::MAX_LEN`].
    #[must_use]
    #[inline]
    pub fn from_array<const N: usize>(array: [T; N]) -> Self {
        const { assert!(Self::MIN_LEN <= N && N <= Self::MAX_LEN) };
        Self::new_unchecked(array.into_iter().collect())
    }

    /// Returns the length of a [`PaletteBuf`] as a [`PaletteSize`].
    #[must_use]
    #[inline]
    pub fn size(&self) -> PaletteSize {
        self.as_palette().size()
    }

    /// Convert a [`PaletteBuf`] reference to a slice.
    #[inline]
    pub fn as_slice(&self) -> &[T] {
        &self.0
    }

    /// Convert a mutable [`PaletteBuf`] reference to a mutable slice.
    #[inline]
    pub fn as_mut_slice(&mut self) -> &mut [T] {
        &mut self.0
    }

    /// Convert a [`PaletteBuf`] reference to a [`Palette`].
    #[inline]
    pub fn as_palette(&self) -> &Palette<T> {
        Palette::new_unchecked(&self.0)
    }

    /// Convert a mutable [`PaletteBuf`] reference to a mutable [`Palette`].
    #[inline]
    pub fn as_mut_palette(&mut self) -> &mut Palette<T> {
        Palette::new_mut_unchecked(&mut self.0)
    }

    /// Convert a [`PaletteBuf`] to the underlying [`Vec`].
    #[must_use]
    #[inline]
    pub fn into_vec(self) -> Vec<T> {
        self.0
    }

    /// Map each [`PaletteBuf`] color to a new color to create a new [`PaletteBuf`].
    ///
    /// See [`map_ref`](Palette::map_ref) for a non-consuming alternative.
    #[must_use]
    #[inline]
    pub fn map<U>(self, mapping: impl FnMut(T) -> U) -> PaletteBuf<U> {
        PaletteBuf::new_unchecked(self.into_iter().map(mapping).collect())
    }

    /// Map a [`Palette`] to a new [`PaletteBuf`] by adapting a function that takes a slice as input
    /// and returns a [`Vec`] as output.
    ///
    /// To instead use a function that maps one element at a time, see [`map_ref`](Palette::map_ref)
    /// or [`map`](Self::map).
    ///
    /// # Examples
    ///
    /// ```
    /// # use quantette::{PaletteBuf, CreatePaletteBufError};
    /// # use palette::Srgb;
    /// # fn main() -> Result<(), CreatePaletteBufError<Srgb<u8>>> {
    /// use quantette::color_space::srgb8_to_oklab;
    /// let srgb_palette = PaletteBuf::<Srgb<u8>>::new(vec![Srgb::new(0, 0, 0)])?;
    /// let oklab_palette = PaletteBuf::from_mapping(&srgb_palette, srgb8_to_oklab);
    /// # Ok(())
    /// # }
    /// ```
    ///
    /// # Panics
    ///
    /// Panics if `mapping` returns a [`Vec`] with a different length than the input slice.
    #[must_use]
    #[inline]
    pub fn from_mapping<U>(
        palette: &Palette<T>,
        mapping: impl FnOnce(&[T]) -> Vec<U>,
    ) -> PaletteBuf<U> {
        let mapped = mapping(palette);
        assert_eq!(palette.len(), mapped.len());
        PaletteBuf::new_unchecked(mapped)
    }

    /// Shorten a [`PaletteBuf`] to `length` number of elements.
    ///
    /// If `length` is greater than or equal to the [`PaletteBuf`]'s current length,
    /// then this has no effect.
    #[inline]
    pub fn truncate(&mut self, length: PaletteSize) {
        self.0.truncate(length.as_usize());
    }
}

impl<T> Deref for PaletteBuf<T> {
    type Target = Palette<T>;

    #[inline]
    fn deref(&self) -> &Self::Target {
        self.as_palette()
    }
}

impl<T> DerefMut for PaletteBuf<T> {
    #[inline]
    fn deref_mut(&mut self) -> &mut Self::Target {
        self.as_mut_palette()
    }
}

impl<T> AsRef<[T]> for PaletteBuf<T> {
    #[inline]
    fn as_ref(&self) -> &[T] {
        self
    }
}

impl<T> AsRef<Palette<T>> for PaletteBuf<T> {
    #[inline]
    fn as_ref(&self) -> &Palette<T> {
        self
    }
}

impl<T> AsRef<PaletteBuf<T>> for PaletteBuf<T> {
    #[inline]
    fn as_ref(&self) -> &PaletteBuf<T> {
        self
    }
}

impl<T> AsMut<[T]> for PaletteBuf<T> {
    #[inline]
    fn as_mut(&mut self) -> &mut [T] {
        self
    }
}

impl<T> AsMut<Palette<T>> for PaletteBuf<T> {
    #[inline]
    fn as_mut(&mut self) -> &mut Palette<T> {
        self
    }
}

impl<T> AsMut<PaletteBuf<T>> for PaletteBuf<T> {
    #[inline]
    fn as_mut(&mut self) -> &mut PaletteBuf<T> {
        self
    }
}

impl<T> Borrow<[T]> for PaletteBuf<T> {
    #[inline]
    fn borrow(&self) -> &[T] {
        self
    }
}

impl<T> Borrow<Palette<T>> for PaletteBuf<T> {
    #[inline]
    fn borrow(&self) -> &Palette<T> {
        self
    }
}

impl<T> BorrowMut<[T]> for PaletteBuf<T> {
    #[inline]
    fn borrow_mut(&mut self) -> &mut [T] {
        self
    }
}

impl<T> BorrowMut<Palette<T>> for PaletteBuf<T> {
    #[inline]
    fn borrow_mut(&mut self) -> &mut Palette<T> {
        self
    }
}

impl<T: Clone> From<&Palette<T>> for PaletteBuf<T> {
    #[inline]
    fn from(palette: &Palette<T>) -> Self {
        palette.to_owned()
    }
}

impl<T: Clone> From<&mut Palette<T>> for PaletteBuf<T> {
    #[inline]
    fn from(palette: &mut Palette<T>) -> Self {
        palette.to_owned()
    }
}

impl<T> TryFrom<Vec<T>> for PaletteBuf<T> {
    type Error = CreatePaletteBufError<T>;

    #[inline]
    fn try_from(container: Vec<T>) -> Result<Self, Self::Error> {
        Self::new(container)
    }
}

impl<T: Clone> TryFrom<&[T]> for PaletteBuf<T> {
    type Error = LengthOutOfRange;

    #[inline]
    fn try_from(slice: &[T]) -> Result<Self, Self::Error> {
        Palette::new(slice).map(Self::from)
    }
}

impl<T: Clone> TryFrom<&mut [T]> for PaletteBuf<T> {
    type Error = LengthOutOfRange;

    #[inline]
    fn try_from(slice: &mut [T]) -> Result<Self, Self::Error> {
        Palette::new_mut(slice).map(Self::from)
    }
}

impl<T> IntoIterator for PaletteBuf<T> {
    type Item = T;

    type IntoIter = <Vec<T> as IntoIterator>::IntoIter;

    #[inline]
    fn into_iter(self) -> Self::IntoIter {
        self.0.into_iter()
    }
}

impl<'a, T> IntoIterator for &'a PaletteBuf<T> {
    type Item = &'a T;

    type IntoIter = <&'a [T] as IntoIterator>::IntoIter;

    #[inline]
    fn into_iter(self) -> Self::IntoIter {
        self.as_slice().iter()
    }
}

impl<'a, T> IntoIterator for &'a mut PaletteBuf<T> {
    type Item = &'a mut T;

    type IntoIter = <&'a mut [T] as IntoIterator>::IntoIter;

    #[inline]
    fn into_iter(self) -> Self::IntoIter {
        self.as_mut_slice().iter_mut()
    }
}

impl<T> Index<usize> for PaletteBuf<T> {
    type Output = T;

    #[inline]
    fn index(&self, index: usize) -> &Self::Output {
        &self.as_palette()[index]
    }
}

impl<T> Index<u8> for PaletteBuf<T> {
    type Output = T;

    #[inline]
    fn index(&self, index: u8) -> &Self::Output {
        &self.as_palette()[index]
    }
}

impl<T> IndexMut<usize> for PaletteBuf<T> {
    #[inline]
    fn index_mut(&mut self, index: usize) -> &mut Self::Output {
        &mut self.as_mut_palette()[index]
    }
}

impl<T> IndexMut<u8> for PaletteBuf<T> {
    #[inline]
    fn index_mut(&mut self, index: u8) -> &mut Self::Output {
        &mut self.as_mut_palette()[index]
    }
}

impl<T, U> PartialEq<[U]> for PaletteBuf<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &[U]) -> bool {
        self.as_slice() == other
    }
}

impl<T, U> PartialEq<PaletteBuf<U>> for [T]
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &PaletteBuf<U>) -> bool {
        self == other.as_slice()
    }
}

impl<T, U> PartialEq<&[U]> for PaletteBuf<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &&[U]) -> bool {
        self.as_slice() == *other
    }
}

impl<T, U> PartialEq<PaletteBuf<U>> for &[T]
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &PaletteBuf<U>) -> bool {
        *self == other.as_slice()
    }
}

impl<T, U> PartialEq<&mut [U]> for PaletteBuf<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &&mut [U]) -> bool {
        self.as_slice() == *other
    }
}

impl<T, U> PartialEq<PaletteBuf<U>> for &mut [T]
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &PaletteBuf<U>) -> bool {
        *self == other.as_slice()
    }
}

impl<T, U, const N: usize> PartialEq<[U; N]> for PaletteBuf<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &[U; N]) -> bool {
        self.as_slice() == &other[..]
    }
}

impl<T, const N: usize, U> PartialEq<PaletteBuf<U>> for [T; N]
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &PaletteBuf<U>) -> bool {
        &self[..] == other.as_slice()
    }
}

impl<T, U> PartialEq<Vec<U>> for PaletteBuf<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &Vec<U>) -> bool {
        self.as_slice() == other.as_slice()
    }
}

impl<T, U> PartialEq<PaletteBuf<U>> for Vec<T>
where
    T: PartialEq<U>,
{
    #[inline]
    fn eq(&self, other: &PaletteBuf<U>) -> bool {
        self.as_slice() == other.as_slice()
    }
}
