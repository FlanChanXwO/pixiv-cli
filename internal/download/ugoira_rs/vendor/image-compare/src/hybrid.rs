use crate::prelude::*;
use crate::squared_error::root_mean_squared_error_simple;
use crate::ssim::ssim_simple;
use crate::utils::{blend_alpha, split_rgba_to_yuva};
use crate::Decompose;
use image::{Rgba, RgbaImage};
use itertools::izip;
use std::borrow::Cow;

fn merge_similarity_channels_yuva(
    input: &[GraySimilarityImage; 4],
    alpha: &GrayImage,
    alpha_second: &GrayImage,
) -> Similarity {
    const ALPHA_VIS_MIN: f32 = 0.1;
    const U8_MAX: f32 = u8::MAX as f32;
    const A_BAR_NORM: f32 = 2. * U8_MAX;

    let mut image = RGBASimilarityImage::new(input[0].width(), input[0].height());
    let mut deviation = Vec::new();
    deviation.resize((input[0].width() * input[0].height()) as usize, 0.0);
    izip!(
        image.pixels_mut(),
        input[0].pixels(),
        input[1].pixels(),
        input[2].pixels(),
        input[3].pixels(),
        alpha.pixels(),
        alpha_second.pixels(),
        deviation.iter_mut()
    )
    .for_each(
        |(rgba, y, u, v, a_d, alpha_source, alpha_source_second, deviation)| {
            let y = y[0].clamp(0.0, 1.0);
            let u = u[0].clamp(0.0, 1.0);
            let v = v[0].clamp(0.0, 1.0);
            let a_d = a_d[0].clamp(0.0, 1.0);
            let alpha_bar = (alpha_source[0] as f32 + alpha_source_second[0] as f32) / A_BAR_NORM;
            let alpha_bar = if alpha_bar.is_finite() {
                alpha_bar
            } else {
                1.0
            };

            let color_diff = ((u).powi(2) + (v).powi(2)).sqrt().clamp(0.0, 1.0);
            let min_sim = y.min(color_diff).min(a_d);
            //the lower the alpha the fewer differences are visible in color and structure (and alpha)

            let dev = if alpha_bar > 0. {
                (min_sim / alpha_bar).clamp(0., 1.)
            } else {
                1.0
            };
            let alpha_vis = (ALPHA_VIS_MIN + a_d * (1.0 - ALPHA_VIS_MIN)).clamp(0., 1.);

            *deviation = dev;
            *rgba = Rgba([1. - y, 1. - u, 1. - v, alpha_vis]);
        },
    );

    let score = deviation.iter().map(|s| *s as f64).sum::<f64>() / deviation.len() as f64;

    Similarity {
        image: image.into(),
        score,
    }
}

fn merge_similarity_channels_yuv(input: &[GraySimilarityImage; 3]) -> Similarity {
    let mut image = RGBSimilarityImage::new(input[0].width(), input[0].height());
    let mut deviation = Vec::new();
    deviation.resize((input[0].width() * input[0].height()) as usize, 0.0);
    izip!(
        image.pixels_mut(),
        input[0].pixels(),
        input[1].pixels(),
        input[2].pixels(),
        deviation.iter_mut()
    )
    .for_each(|(rgb, y, u, v, deviation)| {
        let y = y[0].clamp(0.0, 1.0);
        let u = u[0].clamp(0.0, 1.0);
        let v = v[0].clamp(0.0, 1.0);
        let color_diff = ((u).powi(2) + (v).powi(2)).sqrt().clamp(0.0, 1.0);
        //f32 for keeping numerical stability for hybrid compare in 0.2.-branch
        *deviation += y.min(color_diff);
        *rgb = Rgb([1. - y, 1. - u, 1. - v]);
    });

    let score = deviation.iter().map(|s| *s as f64).sum::<f64>() / deviation.len() as f64;
    Similarity {
        image: image.into(),
        score,
    }
}

/// Hybrid comparison for RGBA images.
/// Will do MSSIM on luma, then RMS on U and V and alpha channels.
/// The calculation of the score is then pixel-wise the minimum of each pixels similarity.
/// To account for perceived indifference in lower alpha regions, this down-weights the difference
/// linearly with mean alpha channel.
pub fn rgba_hybrid_compare(
    first: &RgbaImage,
    second: &RgbaImage,
) -> Result<Similarity, CompareError> {
    if first.dimensions() != second.dimensions() {
        return Err(CompareError::DimensionsDiffer);
    }

    let first = split_rgba_to_yuva(first);
    let second = split_rgba_to_yuva(second);

    let (_, mssim_result) = ssim_simple(&first[0], &second[0])?;
    let (_, u_result) = root_mean_squared_error_simple(&first[1], &second[1])?;
    let (_, v_result) = root_mean_squared_error_simple(&first[2], &second[2])?;

    let (_, alpha_result) = root_mean_squared_error_simple(&first[3], &second[3])?;

    let results = [mssim_result, u_result, v_result, alpha_result];

    Ok(merge_similarity_channels_yuva(
        &results, &first[3], &second[3],
    ))
}

/// A wrapper class accepting both RgbaImage and RgbImage for the blended hybrid comparison
pub enum BlendInput<'a> {
    /// This variant means that the image is already alpha pre-blended and therefore RGB
    PreBlended(&'a RgbImage),
    /// This variant means that the image still needs to be blended with a certain background
    RGBA(&'a RgbaImage),
}

impl<'a> BlendInput<'a> {
    fn into_blended(self, background: Rgb<u8>) -> Cow<'a, RgbImage> {
        match self {
            BlendInput::PreBlended(image) => Cow::Borrowed(image),
            BlendInput::RGBA(rgba) => Cow::Owned(blend_alpha(rgba, background)),
        }
    }
}

impl<'a> From<&'a RgbImage> for BlendInput<'a> {
    fn from(value: &'a RgbImage) -> Self {
        BlendInput::PreBlended(value)
    }
}

impl<'a> From<&'a RgbaImage> for BlendInput<'a> {
    fn from(value: &'a RgbaImage) -> Self {
        BlendInput::RGBA(value)
    }
}

/// This processes the RGBA images be pre-blending the colors with the desired background color.
/// It's faster then the full RGBA similarity and more intuitive.
pub fn rgba_blended_hybrid_compare(
    first: BlendInput,
    second: BlendInput,
    background: Rgb<u8>,
) -> Result<Similarity, CompareError> {
    let first = first.into_blended(background);
    let second = second.into_blended(background);
    rgb_hybrid_compare(&first, &second)
}

/// Comparing structure via MSSIM on Y channel, comparing color-diff-vectors on U and V summing the squares
/// Please mind that the RGBSimilarity-Image does _not_ contain plain RGB here
/// - The red channel contains 1. - similarity(ssim, y)
/// - The green channel contains 1. - similarity(rms, u)
/// - The blue channel contains 1. - similarity(rms, v)
/// This leads to a nice visualization of color and structure differences - with structural differences (meaning gray mssim diffs) leading to red rectangles
/// and and the u and v color diffs leading to color-deviations in green, blue and cyan
/// All-black meaning no differences
pub fn rgb_hybrid_compare(first: &RgbImage, second: &RgbImage) -> Result<Similarity, CompareError> {
    if first.dimensions() != second.dimensions() {
        return Err(CompareError::DimensionsDiffer);
    }

    let first_channels = first.split_to_yuv();
    let second_channels = second.split_to_yuv();

    internal_yuv_hybrid_compare(&first_channels, &second_channels)
}

/// Comparing structure via MSSIM on Y channel, comparing color-diff-vectors on U and V summing the squares
/// first_channels and second_channels are arrays, each containing 3 `GrayImage`s
/// - The first GrayImage contains the Y values - similarity(ssim, y)
/// - The second GrayImage contains the U values - similarity(rms, u)
/// - The third GrayImage contains the V values - similarity(rms, v)
/// Please mind that the SimilarityImage does _not_ contain plain RGB here
/// - The red channel contains 1. - similarity(ssim, y)
/// - The green channel contains 1. - similarity(rms, u)
/// - The blue channel contains 1. - similarity(rms, v)
/// This leads to a nice visualization of color and structure differences - with structural differences (meaning gray mssim diffs) leading to red rectangles
/// and and the u and v color diffs leading to color-deviations in green, blue and cyan
/// All-black meaning no differences
#[cfg(feature = "yuv_compare")]
pub fn yuv_hybrid_compare(first_channels: &[GrayImage; 3], second_channels: &[GrayImage; 3]) -> Result<Similarity, CompareError> {
    if (first_channels[0].dimensions() != second_channels[0].dimensions())
        || (first_channels[1].dimensions() != second_channels[1].dimensions())  // 5 checks needed, this ensures all channels are the same sizes.
        || (first_channels[2].dimensions() != second_channels[2].dimensions())  // First 3 ensure the channels each have the same resolution across the 2 images
        || (first_channels[0].dimensions() != first_channels[1].dimensions())   // Last 2 check if each channel is the same in the first image
        || (first_channels[1].dimensions() != first_channels[2].dimensions()) { // If all checks pass this leaves all 6 being the same
        return Err(CompareError::DimensionsDiffer);
    }
    internal_yuv_hybrid_compare(&first_channels, &second_channels)
}

pub(crate) fn internal_yuv_hybrid_compare(first_channels: &[GrayImage; 3], second_channels: &[GrayImage; 3]) -> Result<Similarity, CompareError> {
    let (_, mssim_result) = ssim_simple(&first_channels[0], &second_channels[0])?;
    let (_, u_result) = root_mean_squared_error_simple(&first_channels[1], &second_channels[1])?;
    let (_, v_result) = root_mean_squared_error_simple(&first_channels[2], &second_channels[2])?;

    let results = [mssim_result, u_result, v_result];

    Ok(merge_similarity_channels_yuv(&results))
}
