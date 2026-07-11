use crate::{IndexedColorMap, IndexedImage, MAX_PIXELS};
use alloc::{borrow::ToOwned, vec, vec::Vec};
use bytemuck::Zeroable;
use core::{
    error::Error,
    fmt::{self, Debug},
    hash::{Hash, Hasher},
    marker::PhantomData,
};

/// The error returned when an [`Image`] failed to be created.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct CreateImageError {
    /// The provided image width.
    width: u32,
    /// The provided image height.
    height: u32,
    /// The length of the pixel buffer.
    length: usize,
}

impl fmt::Display for CreateImageError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let Self { width, height, length } = *self;
        if width.checked_mul(height).is_some() {
            write!(
                f,
                "image dimensions of ({width}, {height}) do not match the buffer length of {length}"
            )
        } else {
            write!(
                f,
                "image dimensions of ({width}, {height}) are above the maximum number of pixels of {MAX_PIXELS}",
            )
        }
    }
}

impl Error for CreateImageError {}

/// The error returned when an [`Image`] failed to be created. Includes the pixel buffer used to try
/// and create the image.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct CreateImageBufError<T> {
    /// The underlying error/reason.
    pub error: CreateImageError,
    /// The provided container holding the pixels of the image.
    pub buffer: T,
}

impl<T> fmt::Display for CreateImageBufError<T> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        fmt::Debug::fmt(&self.error, f)
    }
}

impl<T: Debug> Error for CreateImageBufError<T> {}

/// The base image type parameterized by the type of the container.
///
/// Typically you want to use one of the other image types with a defined container:
/// - [`ImageBuf`]: an owned image backed by a [`Vec`].
/// - [`ImageRef`]: a borrowed image backed by an immutable slice reference.
/// - [`ImageMut`]: a mutable, borrowed image backed by a mutable slice reference.
#[derive(Clone, Copy, Debug)]
pub struct Image<Color, Container> {
    /// The color type stored in `pixels`.
    color: PhantomData<Color>,
    /// The width of the image.
    width: u32,
    /// The height of the image.
    height: u32,
    /// The pixel buffer or slice.
    pixels: Container,
}

/// An owned image buffer backed by a [`Vec`].
///
/// This type consists of a width, a height, and a pixel buffer in row-major order.
/// The length of the pixel [`Vec`] is guaranteed to match `width * height` and be less than or
/// equal to [`MAX_PIXELS`].
///
/// See [`ImageRef`] and [`ImageMut`] for borrowed variants of an image.
/// Use [`as_ref`](ImageBuf::as_ref) or [`as_mut`](ImageBuf::as_mut) to create a borrowed image.
///
/// # Examples
///
/// Directly creating an [`ImageBuf`] from a [`Vec`]:
///
/// ```
/// # use quantette::{ImageBuf, CreateImageBufError};
/// # use palette::Srgb;
/// # fn main() -> Result<(), CreateImageBufError<Vec<Srgb<u8>>>> {
/// let (width, height) = (512, 512);
/// let pixels = vec![Srgb::new(0, 0, 0); (width * height) as usize];
/// let image = ImageBuf::new(width, height, pixels)?;
/// # Ok(())
/// # }
/// ```
///
/// Converting a [`RgbImage`](image::RgbImage) from the [`image`] crate to an [`ImageBuf`] and vice versa:
///
/// ```
/// # use quantette::{ImageBuf, CreateImageBufError};
/// # use image::RgbImage;
/// # fn main() -> Result<(), CreateImageBufError<RgbImage>> {
/// let image = RgbImage::new(256, 256);
/// let image = ImageBuf::try_from(image)?;
/// let image: RgbImage = image.into();
/// # Ok(())
/// # }
/// ```
pub type ImageBuf<Color> = Image<Color, Vec<Color>>;

/// A borrowed image backed by a reference to a slice.
///
/// This type consists of a width, a height, and a pixel slice in row-major order.
/// The length of the pixel slice is guaranteed to match `width * height` and be less than or
/// equal to [`MAX_PIXELS`].
///
/// See [`ImageBuf`] for an owned variant of an image and [`ImageMut`] for a mutable, borrowed image.
///
/// # Examples
///
/// Directly creating an [`ImageRef`] from a slice:
///
/// ```
/// # use quantette::ImageRef;
/// # use palette::Srgb;
/// let (width, height) = (512, 512);
/// let pixels = vec![Srgb::new(0, 0, 0); (width * height) as usize];
/// let image = ImageRef::new(width, height, &pixels).unwrap();
/// ```
///
/// Converting a reference to a [`RgbImage`](image::RgbImage) from the [`image`] crate to an [`ImageRef`]:
///
/// ```
/// # use quantette::{LengthOutOfRange, ImageRef};
/// # use image::RgbImage;
/// # fn main() -> Result<(), LengthOutOfRange> {
/// let image = RgbImage::new(256, 256);
/// let image = ImageRef::try_from(&image)?;
/// # Ok(())
/// # }
/// ```
pub type ImageRef<'a, Color> = Image<Color, &'a [Color]>;

/// A mutable, borrowed image backed by a mutable reference to a slice.
///
/// This type consists of a width, a height, and a pixel slice in row-major order.
/// The length of the pixel slice is guaranteed to match `width * height` and be less than or
/// equal to [`MAX_PIXELS`].
///
/// See [`ImageBuf`] for an owned variant of an image and [`ImageRef`] for an immutable, borrowed image.
///
/// # Examples
///
/// Directly creating an [`ImageMut`] from a slice:
///
/// ```
/// # use quantette::ImageMut;
/// # use palette::Srgb;
/// let (width, height) = (512, 512);
/// let mut pixels = vec![Srgb::new(0, 0, 0); (width * height) as usize];
/// let image = ImageMut::new(width, height, &mut pixels).unwrap();
/// ```
///
/// Converting a mutable reference to a [`RgbImage`](image::RgbImage) from the [`image`] crate
/// to an [`ImageMut`]:
///
/// ```
/// # use quantette::{LengthOutOfRange, ImageMut};
/// # use image::RgbImage;
/// # fn main() -> Result<(), LengthOutOfRange> {
/// let mut image = RgbImage::new(256, 256);
/// let image = ImageMut::try_from(&mut image)?;
/// # Ok(())
/// # }
/// ```
pub type ImageMut<'a, Color> = Image<Color, &'a mut [Color]>;

impl<Color, Container> Image<Color, Container> {
    /// Returns the width and height of the [`Image`].
    #[inline]
    pub fn dimensions(&self) -> (u32, u32) {
        (self.width, self.height)
    }

    /// Returns the width of the [`Image`].
    #[inline]
    pub fn width(&self) -> u32 {
        self.width
    }

    /// Returns the height of the [`Image`].
    #[inline]
    pub fn height(&self) -> u32 {
        self.height
    }

    /// Returns whether the [`Image`] has zero pixels.
    #[inline]
    pub fn is_empty(&self) -> bool {
        self.width == 0 || self.height == 0
    }

    /// Returns a reference to the underlying pixel container.
    #[inline]
    pub fn as_inner(&self) -> &Container {
        &self.pixels
    }

    /// Returns the underlying pixel container.
    #[must_use]
    #[inline]
    pub fn into_inner(self) -> Container {
        self.pixels
    }
}

impl<Color, Container: AsRef<[Color]>> Image<Color, Container> {
    /// Create a new [`Image`] without validating invariants.
    #[inline]
    pub(crate) fn new_unchecked(width: u32, height: u32, pixels: Container) -> Self {
        debug_assert_eq!(
            width.checked_mul(height).map(|len| len as usize),
            Some(pixels.as_ref().len())
        );
        Self {
            color: PhantomData,
            width,
            height,
            pixels,
        }
    }

    /// Create a new [`Image`] from a width, a height, and a `Container` of pixels.
    ///
    /// # Errors
    ///
    /// The provided `pixels` is returned as an `Err` if any of the following are true:
    /// - The length of `pixels` and `width * height` do not match.
    /// - `width * height` overflows a `u32`.
    #[inline]
    pub fn new(
        width: u32,
        height: u32,
        pixels: Container,
    ) -> Result<Self, CreateImageBufError<Container>> {
        let length = pixels.as_ref().len();
        if width.checked_mul(height).map(|len| len as usize) == Some(length) {
            Ok(Self::new_unchecked(width, height, pixels))
        } else {
            let error = CreateImageError { width, height, length };
            Err(CreateImageBufError { error, buffer: pixels })
        }
    }

    /// Returns the number of pixels in the [`Image`] specified by `width * height`.
    #[allow(clippy::cast_possible_truncation)]
    #[inline]
    pub fn num_pixels(&self) -> u32 {
        self.pixels.as_ref().len() as u32
    }

    /// Returns a reference to the underlying pixels as a slice.
    #[inline]
    pub fn as_slice(&self) -> &[Color] {
        self.pixels.as_ref()
    }

    /// Convert an [`Image`] to an [`ImageRef`].
    #[inline]
    pub fn as_ref(&self) -> ImageRef<'_, Color> {
        let (width, height) = self.dimensions();
        Image::new_unchecked(width, height, self.as_slice())
    }

    /// Convert an [`Image`] to an owned [`ImageBuf`].
    #[allow(clippy::missing_panics_doc)]
    #[must_use]
    #[inline]
    pub fn to_owned(&self) -> Image<Color, Container::Owned>
    where
        Container: ToOwned,
        Container::Owned: AsRef<[Color]>,
    {
        let (width, height) = self.dimensions();
        let owned = self.pixels.to_owned();
        assert_eq!(
            self.pixels.as_ref().len(),
            owned.as_ref().len(),
            "the owned container to have the same length as the original container"
        );
        Image::new_unchecked(width, height, owned)
    }

    /// Map the pixel buffer of an [`Image`] to new buffer type and/or color type.
    ///
    /// Rather than being a function from `Color -> NewColor`, `mapping` takes the whole pixel
    /// buffer and returns a new buffer. This is to allow batch or parallel mappings.
    ///
    /// # Examples
    ///
    /// It is recommended to do batch mappings for efficiency where it makes sense. E.g., using the
    /// color space conversion functions from the [`color_space`](crate::color_space) module.
    ///
    /// ```
    /// # use quantette::ImageBuf;
    /// # use palette::{Srgb, LinSrgb};
    /// use quantette::color_space::srgb8_to_oklab;
    /// let srgb_image = ImageBuf::<Srgb<u8>>::default();
    /// let oklab_image = srgb_image.map(|pixels| srgb8_to_oklab(&pixels));
    /// ```
    ///
    /// To instead map each pixel one at a time, use `into_iter`, `map`, and `collect` like normal:
    ///
    /// ```
    /// # use quantette::ImageBuf;
    /// # use palette::{Srgb, LinSrgb};
    /// let srgb_image = ImageBuf::<Srgb<u8>>::default();
    /// let lin_srgb_image: ImageBuf<LinSrgb> =
    ///     srgb_image.map(|pixels| pixels.into_iter().map(|srgb| srgb.into_linear()).collect());
    /// ```
    ///
    /// # Panics
    ///
    /// Panics if `mapping` returns a container with a different length than the original container.
    #[must_use]
    #[inline]
    pub fn map<NewColor, NewContainer>(
        self,
        mapping: impl FnOnce(Container) -> NewContainer,
    ) -> Image<NewColor, NewContainer>
    where
        NewContainer: AsRef<[NewColor]>,
    {
        let num_pixels = self.num_pixels();
        let (width, height) = self.dimensions();
        let pixels = mapping(self.pixels);
        assert_eq!(pixels.as_ref().len(), num_pixels as usize);
        Image::new_unchecked(width, height, pixels)
    }

    /// Map the pixel buffer of an [`Image`] to new buffer type and/or color type.
    ///
    /// Rather than being a function from `Color -> NewColor`, `mapping` takes the whole pixel
    /// slice and returns a new buffer. This is to allow batch or parallel mappings.
    ///
    /// # Examples
    ///
    /// It is recommended to do batch mappings for efficiency where it makes sense. E.g., using the
    /// color space conversion functions from the [`color_space`](crate::color_space) module.
    ///
    /// ```
    /// # use quantette::ImageBuf;
    /// # use palette::{Srgb, LinSrgb};
    /// use quantette::color_space::srgb8_to_oklab;
    /// let srgb_image = ImageBuf::<Srgb<u8>>::default();
    /// let oklab_image = srgb_image.map_ref(srgb8_to_oklab);
    /// ```
    ///
    /// To instead map each pixel one at a time, use `iter`, `map`, and `collect` like normal:
    ///
    /// ```
    /// # use quantette::ImageBuf;
    /// # use palette::{Srgb, LinSrgb};
    /// let srgb_image = ImageBuf::<Srgb<u8>>::default();
    /// let lin_srgb_image: ImageBuf<LinSrgb> =
    ///     srgb_image.map_ref(|palette| palette.iter().map(|srgb| srgb.into_linear()).collect());
    /// ```
    ///
    /// # Panics
    ///
    /// Panics if `mapping` returns a container with a different length than the original container.
    #[must_use]
    #[inline]
    pub fn map_ref<NewColor, NewContainer>(
        &self,
        mapping: impl FnOnce(&[Color]) -> NewContainer,
    ) -> Image<NewColor, NewContainer>
    where
        NewContainer: AsRef<[NewColor]>,
    {
        let num_pixels = self.num_pixels();
        let (width, height) = self.dimensions();
        let pixels = mapping(self.pixels.as_ref());
        assert_eq!(pixels.as_ref().len(), num_pixels as usize);
        Image::new_unchecked(width, height, pixels)
    }

    /// Map the pixel buffer of an [`Image`] using the provided [`IndexedColorMap`].
    ///
    /// # Panics
    ///
    /// Panics if `color_map` is not a valid implementor of [`IndexedColorMap`]. That is, it returns
    /// a [`Vec`] with a different length than the input slice.
    #[must_use]
    #[inline]
    pub fn map_to_image<ColorMap: IndexedColorMap<Color>>(
        &self,
        color_map: ColorMap,
    ) -> ImageBuf<ColorMap::Output> {
        self.map_ref(|pixels| color_map.map_to_colors(pixels))
    }

    /// Convert an [`Image`] to an [`IndexedImage`] using the provided [`IndexedColorMap`].
    ///
    /// # Panics
    ///
    /// Panics if `color_map` is not a valid implementor of [`IndexedColorMap`]. That is, it returns
    /// a [`Vec`] with a different length than the input slice.
    #[must_use]
    #[inline]
    pub fn map_to_indexed<ColorMap: IndexedColorMap<Color>>(
        &self,
        color_map: ColorMap,
    ) -> IndexedImage<ColorMap::Output> {
        let indices = color_map.map_to_indices(self.as_slice());
        assert_eq!(indices.len(), self.num_pixels() as usize);
        let (width, height) = self.dimensions();
        IndexedImage::new_unchecked(width, height, color_map.into_palette().into_vec(), indices)
    }
}

impl<Color, Container: AsMut<[Color]>> Image<Color, Container> {
    /// Returns a mutable reference to the underlying pixels as a mutable slice.
    #[inline]
    pub fn as_mut_slice(&mut self) -> &mut [Color] {
        self.pixels.as_mut()
    }

    /// Convert an [`Image`] to an [`ImageMut`].
    #[inline]
    pub fn as_mut(&mut self) -> ImageMut<'_, Color> {
        let (width, height) = self.dimensions();
        Image::new_unchecked(width, height, self.as_mut_slice())
    }
}

impl<Color: Zeroable> ImageBuf<Color> {
    /// Create a new [`ImageBuf`] from zeroed memory.
    ///
    /// Returns `None` if `width * height` overflows a `u32`.
    #[must_use]
    #[inline]
    pub fn zeroed(width: u32, height: u32) -> Option<Self> {
        let len = width.checked_mul(height)?;
        let pixels = bytemuck::zeroed_vec(len as usize);
        Some(Self::new_unchecked(width, height, pixels))
    }
}

impl<Color: Clone> ImageBuf<Color> {
    /// Create a new [`ImageBuf`] by cloning a specific color.
    ///
    /// Returns `None` if `width * height` overflows a `u32`.
    #[must_use]
    #[inline]
    pub fn from_pixel(width: u32, height: u32, pixel: Color) -> Option<Self> {
        let len = width.checked_mul(height)?;
        let pixels = vec![pixel; len as usize];
        Some(Self::new_unchecked(width, height, pixels))
    }
}

impl<Color> Default for ImageBuf<Color> {
    #[inline]
    fn default() -> Self {
        Self::new_unchecked(0, 0, Vec::new())
    }
}

impl<Color> Default for ImageRef<'_, Color> {
    #[inline]
    fn default() -> Self {
        Self::new_unchecked(0, 0, &[])
    }
}

impl<Color> Default for ImageMut<'_, Color> {
    #[inline]
    fn default() -> Self {
        Self::new_unchecked(0, 0, &mut [])
    }
}

impl<ColorA, ColorB, ContainerA, ContainerB> PartialEq<Image<ColorB, ContainerB>>
    for Image<ColorA, ContainerA>
where
    ColorA: PartialEq<ColorB>,
    ContainerA: AsRef<[ColorA]>,
    ContainerB: AsRef<[ColorB]>,
{
    fn eq(&self, other: &Image<ColorB, ContainerB>) -> bool {
        self.dimensions() == other.dimensions() && self.as_slice() == other.as_slice()
    }
}

impl<Color, Container> Eq for Image<Color, Container>
where
    Color: PartialEq<Color>,
    Container: AsRef<[Color]>,
{
}

impl<Color, Container> Hash for Image<Color, Container>
where
    Color: Hash,
    Container: AsRef<[Color]>,
{
    fn hash<H: Hasher>(&self, state: &mut H) {
        self.width.hash(state);
        self.height.hash(state);
        self.pixels.as_ref().hash(state);
    }
}

#[cfg(feature = "image")]
mod image_integration {
    use super::{CreateImageBufError, CreateImageError, Image, ImageBuf, ImageMut, ImageRef};
    use crate::LengthOutOfRange;
    use core::ops::{Deref, DerefMut};
    use image::{ImageBuffer, Pixel, Rgb};
    use palette::{
        ArrayExt, Srgb,
        cast::{
            ArrayCast, ComponentsAs as _, ComponentsAsMut as _, ComponentsInto as _,
            IntoComponents as _,
        },
    };

    impl<Component> From<ImageBuf<Srgb<Component>>> for ImageBuffer<Rgb<Component>, Vec<Component>>
    where
        Srgb<Component>: ArrayCast,
        <Srgb<Component> as ArrayCast>::Array: ArrayExt<Item = Component>,
        Rgb<Component>: Pixel<Subpixel = Component>,
        Vec<Component>: Deref<Target = [<Rgb<Component> as Pixel>::Subpixel]>,
    {
        #[allow(clippy::expect_used)]
        fn from(image: ImageBuf<Srgb<Component>>) -> Self {
            let Image { width, height, pixels, .. } = image;
            ImageBuffer::from_raw(width, height, pixels.into_components())
                .expect("buffer is large enough")
        }
    }

    impl<Component> TryFrom<ImageBuffer<Rgb<Component>, Vec<Component>>> for ImageBuf<Srgb<Component>>
    where
        Srgb<Component>: ArrayCast,
        <Srgb<Component> as ArrayCast>::Array: ArrayExt<Item = Component>,
        Rgb<Component>: Pixel<Subpixel = Component>,
        Vec<Component>: Deref<Target = [<Rgb<Component> as Pixel>::Subpixel]>,
    {
        type Error = CreateImageBufError<ImageBuffer<Rgb<Component>, Vec<Component>>>;

        fn try_from(
            image: ImageBuffer<Rgb<Component>, Vec<Component>>,
        ) -> Result<Self, Self::Error> {
            let (width, height) = image.dimensions();
            if let Some(len) = width.checked_mul(height) {
                let mut buf = image.into_raw();
                buf.truncate(len as usize * 3);
                assert_eq!(buf.len(), len as usize * 3); // in case buf.len() < len * 3
                let pixels = buf.components_into();
                Ok(Self::new_unchecked(width, height, pixels))
            } else {
                let error = CreateImageError {
                    width,
                    height,
                    length: image.pixels().len(),
                };
                Err(CreateImageBufError { error, buffer: image })
            }
        }
    }

    impl<'a, Component, Container> TryFrom<&'a ImageBuffer<Rgb<Component>, Container>>
        for ImageRef<'a, Srgb<Component>>
    where
        Rgb<Component>: Pixel<Subpixel = Component>,
        Container: Deref<Target = [<Rgb<Component> as Pixel>::Subpixel]>,
    {
        type Error = LengthOutOfRange;

        fn try_from(
            image: &'a ImageBuffer<Rgb<Component>, Container>,
        ) -> Result<Self, Self::Error> {
            let (width, height) = image.dimensions();
            let len = LengthOutOfRange::check_dimensions(width, height)?;
            let slice = &image.as_raw()[..len as usize * 3];
            let pixels = slice.components_as();
            Ok(Self::new_unchecked(width, height, pixels))
        }
    }

    impl<'a, Component, Container> TryFrom<&'a mut ImageBuffer<Rgb<Component>, Container>>
        for ImageMut<'a, Srgb<Component>>
    where
        Rgb<Component>: Pixel<Subpixel = Component>,
        Container: Deref<Target = [<Rgb<Component> as Pixel>::Subpixel]> + DerefMut,
    {
        type Error = LengthOutOfRange;

        fn try_from(
            image: &'a mut ImageBuffer<Rgb<Component>, Container>,
        ) -> Result<Self, Self::Error> {
            let (width, height) = image.dimensions();
            let len = LengthOutOfRange::check_dimensions(width, height)?;
            let slice = &mut image.deref_mut()[..len as usize * 3];
            let pixels = slice.components_as_mut();
            Ok(Self::new_unchecked(width, height, pixels))
        }
    }
}

#[cfg(feature = "image")]
#[allow(unused_imports)]
pub use image_integration::*;
