use palette::cast::ArrayCast;

/// Types that may be cast to and from a fixed sized array.
///
/// Quantization functions in `quantette` operate over a color type/space.
/// These types must implement [`ArrayCast`] where `Component` is the data type and `N` is the
/// number of channels.
pub trait ColorComponents<Component, const N: usize>:
    ArrayCast<Array = [Component; N]> + Copy + Send + Sync + 'static
{
}
impl<Color, Component, const N: usize> ColorComponents<Component, N> for Color where
    Color: ArrayCast<Array = [Component; N]> + Copy + Send + Sync + 'static
{
}
