#![crate_name = "image_compare"]
//! # Comparing gray images using structure
//! This crate allows to compare grayscale images using either structure or histogramming methods.
//! The easiest use is loading two images, converting them to grayscale and running a comparison:
//! ```no_run
//! use image_compare::Algorithm;
//! let image_one = image::open("image1.png").expect("Could not find test-image").into_luma8();
//! let image_two = image::open("image2.png").expect("Could not find test-image").into_luma8();
//! let result = image_compare::gray_similarity_structure(&Algorithm::MSSIMSimple, &image_one, &image_two).expect("Images had different dimensions");
//! ```
//! Check the [`Algorithm`] enum for implementation details
//!
//! # Comparing gray images using histogram
//!
//! Histogram comparisons are possible using the histogram comparison function
//! ```no_run
//! use image_compare::Metric;
//! let image_one = image::open("image1.png").expect("Could not find test-image").into_luma8();
//! let image_two = image::open("image2.png").expect("Could not find test-image").into_luma8();
//! let result = image_compare::gray_similarity_histogram(Metric::Hellinger, &image_one, &image_two).expect("Images had different dimensions");
//! ```
//! Check the [`Metric`] enum for implementation details
//!
//! # Comparing rgb images using hybrid mode
//!
//! hybrid mode allows to decompose the image to structure and color channels (YUV) which
//! are compared separately but then combined into a common result.
//! ## Direct usage on two RGB8 images
//! ```no_run
//! let image_one = image::open("image1.png").expect("Could not find test-image").into_rgb8();
//! let image_two = image::open("image2.png").expect("Could not find test-image").into_rgb8();
//! let result = image_compare::rgb_hybrid_compare(&image_one, &image_two).expect("Images had different dimensions");
//! ```
//!
//! ## Compare the similarity of two maybe-rgba images in front a given background color
//! If an image is RGBA it will be blended with a background of the given color.
//! RGB images will not be modified.
//!
//! ```no_run
//! use image::Rgb;
//! let image_one = image::open("image1.png").expect("Could not find test-image").into_rgba8();
//! let image_two = image::open("image2.png").expect("Could not find test-image").into_rgb8();
//! let white = Rgb([255,255,255]);
//! let result = image_compare::rgba_blended_hybrid_compare((&image_one).into(), (&image_two).into(), white).expect("Images had different dimensions");
//! ```
//!
//! # Comparing two RGBA8 images using hybrid mode
//!
//! hybrid mode allows to decompose the image to structure, color and alpha channels (YUVA) which
//! are compared separately but then combined into a common result.
//! ```no_run
//! let image_one = image::open("image1.png").expect("Could not find test-image").into_rgba8();
//! let image_two = image::open("image2.png").expect("Could not find test-image").into_rgba8();
//! let result = image_compare::rgba_hybrid_compare(&image_one, &image_two).expect("Images had different dimensions");
//! ```
//!
//! # Using structure results
//! All structural comparisons return a result struct that contains the similarity score.
//! For the score 1.0 is perfectly similar, 0.0 is dissimilar and some algorithms even provide up to -1.0 for inverse.
//! Furthermore, the algorithm may produce a similarity map (MSSIM, RMS and hybrid compare do) that can be evaluated per pixel or converted to a visualization:
//! ```no_run
//! let image_one = image::open("image1.png").expect("Could not find test-image").into_rgba8();
//! let image_two = image::open("image2.png").expect("Could not find test-image").into_rgba8();
//! let result = image_compare::rgba_hybrid_compare(&image_one, &image_two).expect("Images had different dimensions");
//! if result.score < 0.95 {
//!   let diff_img = result.image.to_color_map();
//!   diff_img.save("diff_image.png").expect("Could not save diff image");
//! }
//! ```

#![warn(missing_docs)]
#![warn(unused_qualifications)]
#![deny(deprecated)]

mod colorization;
mod histogram;
mod hybrid;
mod squared_error;
mod ssim;


#[cfg(not(feature="yuv_compare"))] // Tests cannot be implemented to check the functionality of this feature gate, please be mindful of this.
mod utils;

/// Provides some utilities to make yuv image management and conversion easier.
#[cfg(feature="yuv_compare")] // Exposes rgb/yuv conversions and split to yuv publicly, others were left to be pub crate or private
pub mod utils;                // All exposed APIs have tests, and compilation will fail if they ever become non-public

#[doc(hidden)]
pub mod prelude {
    pub use image::{GrayImage, ImageBuffer, Luma, Rgb, RgbImage};
    use thiserror::Error;
    /// The enum for selecting a grayscale comparison implementation
    pub enum Algorithm {
        /// A simple RMSE implementation - will return: <img src="https://render.githubusercontent.com/render/math?math=1-\sqrt{\frac{(\sum_{x,y=0}^{x,y=w,h}\left(f(x,y)-g(x,y)\right)^2)}{w*h}}">
        RootMeanSquared,
        /// a simple MSSIM implementation - will run SSIM (implemented as on wikipedia: <img src="https://render.githubusercontent.com/render/math?math=\mathrm{SSIM}(x,y)={\frac {(2\mu _{x}\mu _{y}+c_{1})(2\sigma _{xy}+c_{2})}{(\mu _{x}^{2}+\mu _{y}^{2}+c_{1})(\sigma _{x}^{2}+\sigma _{y}^{2}+c_{2})}}"> ) over 8x8 px windows and average the results
        MSSIMSimple,
    }

    #[derive(Error, Debug)]
    /// The errors that can occur during comparison of the images
    pub enum CompareError {
        #[error("The dimensions of the input images are not identical")]
        DimensionsDiffer,
        #[error("Comparison calculation failed: {0}")]
        CalculationFailed(String),
    }

    pub use crate::colorization::GraySimilarityImage;
    pub use crate::colorization::RGBASimilarityImage;
    pub use crate::colorization::RGBSimilarityImage;
    pub use crate::colorization::Similarity;
}

#[doc(inline)]
pub use histogram::Metric;
#[doc(inline)]
pub use prelude::Algorithm;
#[doc(inline)]
pub use prelude::CompareError;
#[doc(inline)]
pub use prelude::Similarity;

use prelude::*;
use utils::Decompose;

/// Comparing gray images using structure.
///
/// # Arguments
///
/// * `algorithm` - The comparison algorithm to use
///
/// * `first` - The first of the images to compare
///
/// * `second` - The first of the images to compare
pub fn gray_similarity_structure(
    algorithm: &Algorithm,
    first: &GrayImage,
    second: &GrayImage,
) -> Result<Similarity, CompareError> {
    if first.dimensions() != second.dimensions() {
        return Err(CompareError::DimensionsDiffer);
    }
    match algorithm {
        Algorithm::RootMeanSquared => root_mean_squared_error_simple(first, second),
        Algorithm::MSSIMSimple => ssim_simple(first, second),
    }
    .map(|(score, i)| Similarity {
        image: i.into(),
        score,
    })
}

/// Comparing rgb images using structure.
/// RGB structure similarity is performed by doing a channel split and taking the maximum deviation (minimum similarity) for the result.
/// The image contains the complete deviations.
/// # Arguments
///
/// * `algorithm` - The comparison algorithm to use
///
/// * `first` - The first of the images to compare
///
/// * `second` - The first of the images to compare
///
/// ### Experimental:
/// As you can see from the pinning tests in cucumber - the differences are quite small, the runtime difference is rather large though.
pub fn rgb_similarity_structure(
    algorithm: &Algorithm,
    first: &RgbImage,
    second: &RgbImage,
) -> Result<Similarity, CompareError> {
    if first.dimensions() != second.dimensions() {
        return Err(CompareError::DimensionsDiffer);
    }

    let first_channels = first.split_channels();
    let second_channels = second.split_channels();
    let mut results = Vec::new();

    for channel in 0..3 {
        match algorithm {
            Algorithm::RootMeanSquared => {
                results.push(root_mean_squared_error_simple(
                    &first_channels[channel],
                    &second_channels[channel],
                )?);
            }
            Algorithm::MSSIMSimple => {
                results.push(ssim_simple(
                    &first_channels[channel],
                    &second_channels[channel],
                )?);
            }
        }
    }
    let input = results.iter().map(|(_, i)| i).collect::<Vec<_>>();
    let image = utils::merge_similarity_channels(&input.try_into().unwrap());
    let score = results.iter().map(|(s, _)| *s).fold(1., f64::min);
    Ok(Similarity {
        image: image.into(),
        score,
    })
}

/// Comparing gray images using histogram
/// # Arguments
///
/// * `metric` - The distance metric to use
///
/// * `first` - The first of the images to compare
///
/// * `second` - The first of the images to compare
pub fn gray_similarity_histogram(
    metric: Metric,
    first: &GrayImage,
    second: &GrayImage,
) -> Result<f64, CompareError> {
    if first.dimensions() != second.dimensions() {
        return Err(CompareError::DimensionsDiffer);
    }
    histogram::img_compare(first, second, metric)
}

#[doc(inline)]
pub use hybrid::rgb_hybrid_compare;

use crate::squared_error::root_mean_squared_error_simple;
use crate::ssim::ssim_simple;
#[doc(inline)]
pub use hybrid::rgba_hybrid_compare;

#[doc(inline)]
pub use hybrid::rgba_blended_hybrid_compare;

#[doc(inline)]
#[cfg(feature = "yuv_compare")]
pub use hybrid::yuv_hybrid_compare;

pub use hybrid::BlendInput;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn dimensions_differ_test_gray_structure() {
        let first = GrayImage::new(1, 1);
        let second = GrayImage::new(2, 2);
        let result = gray_similarity_structure(&Algorithm::RootMeanSquared, &first, &second);
        assert!(result.is_err());
    }

    #[test]
    fn dimensions_differ_test_rgb_structure() {
        let first = RgbImage::new(1, 1);
        let second = RgbImage::new(2, 2);
        let result = rgb_similarity_structure(&Algorithm::RootMeanSquared, &first, &second);
        assert!(result.is_err());
    }

    #[test]
    fn dimensions_differ_test_gray_histos() {
        let first = GrayImage::new(1, 1);
        let second = GrayImage::new(2, 2);
        let result = gray_similarity_histogram(Metric::Hellinger, &first, &second);
        assert!(result.is_err());
    }
}
