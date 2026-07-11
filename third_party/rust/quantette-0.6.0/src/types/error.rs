use core::{
    error::Error,
    fmt::{self, Debug},
};

/// The error returned when the length of a value or input is not in the supported range.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct LengthOutOfRange {
    /// The length of the provided value.
    len: usize,
    /// The minimum supported length.
    min: u32,
    /// The maximum supported length.
    max: u32,
}

impl LengthOutOfRange {
    #[cfg(feature = "image")]
    #[inline]
    pub(crate) fn check_dimensions(width: u32, height: u32) -> Result<u32, Self> {
        if let Some(len) = width.checked_mul(height) {
            Ok(len)
        } else {
            Err(Self {
                len: width as usize * height as usize,
                min: 0,
                max: crate::MAX_PIXELS,
            })
        }
    }

    #[inline]
    pub(crate) const fn check_u32<T>(slice: &[T], min: u32, max: u32) -> Result<u32, Self> {
        let len = slice.len();
        #[allow(clippy::cast_possible_truncation)]
        if min as usize <= len && len <= max as usize {
            Ok(len as u32)
        } else {
            Err(Self { len, min, max })
        }
    }

    #[inline]
    pub(crate) const fn check_u16<T>(slice: &[T], min: u16, max: u16) -> Result<u16, Self> {
        let len = slice.len();
        #[allow(clippy::cast_possible_truncation)]
        if min as usize <= len && len <= max as usize {
            Ok(len as u16)
        } else {
            Err(Self { len, min: min as u32, max: max as u32 })
        }
    }
}

impl fmt::Display for LengthOutOfRange {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let Self { len, min, max } = *self;
        if min == 0 {
            write!(
                f,
                "got an input with length {len} which is above the maximum {max}",
            )
        } else {
            write!(
                f,
                "got an input with length {len} which is not in the supported range of {min}..={max}",
            )
        }
    }
}

impl Error for LengthOutOfRange {}
