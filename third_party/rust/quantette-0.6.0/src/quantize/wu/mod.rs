//! Wu's color quantization method (Greedy Orthogonal Bipartitioning).
//!
//! This preclustering method progressively splits the histogram box with the greatest variance
//! along the dimension and bin that results in the greatest decrease in variance. It should give
//! much better results than median cut while having nearly the same computational cost. Compared to
//! k-means clustering (see the [`kmeans`](crate::kmeans) module), this quantization method is much
//! faster, but gives less accurate results.
//!
//! Currently, only colors with 3 `u8` or 3 `f32` components are supported via [`WuU8x3`] and [`WuF32x3`].
//! Those two structs have the same API. See their documentation for more details and examples.

pub(crate) mod shared;

mod f32;
mod u8;

pub use f32::*;
pub use u8::*;
