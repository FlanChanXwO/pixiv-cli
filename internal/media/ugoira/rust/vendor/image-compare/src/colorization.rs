use image::{DynamicImage, GrayImage, ImageBuffer, Luma, Rgb, RgbImage, Rgba, RgbaImage};

/// a single-channel f32 typed image containing a result-score for each pixel
pub type GraySimilarityImage = ImageBuffer<Luma<f32>, Vec<f32>>;
/// a three-channel f32 typed image containing a result-score per color channel for each pixel
pub type RGBSimilarityImage = ImageBuffer<Rgb<f32>, Vec<f32>>;
/// a four-channel f32 typed image containing a result-score per color channel for each pixel
pub type RGBASimilarityImage = ImageBuffer<Rgba<f32>, Vec<f32>>;

#[derive(Debug)]
#[allow(clippy::upper_case_acronyms)]
pub enum SimilarityImage {
    Gray(GraySimilarityImage),
    RGB(RGBSimilarityImage),
    RGBA(RGBASimilarityImage),
}

impl From<GraySimilarityImage> for SimilarityImage {
    fn from(value: GraySimilarityImage) -> Self {
        SimilarityImage::Gray(value)
    }
}
impl From<RGBASimilarityImage> for SimilarityImage {
    fn from(value: RGBASimilarityImage) -> Self {
        SimilarityImage::RGBA(value)
    }
}
impl From<RGBSimilarityImage> for SimilarityImage {
    fn from(value: RGBSimilarityImage) -> Self {
        SimilarityImage::RGB(value)
    }
}

fn gray_map(img: &GraySimilarityImage) -> DynamicImage {
    let mut img_gray = GrayImage::new(img.width(), img.height());
    for row in 0..img.height() {
        for col in 0..img.width() {
            let new_val = img.get_pixel(col, row)[0].clamp(0., 1.) * 255.;
            img_gray.put_pixel(col, row, Luma([new_val as u8]));
        }
    }
    img_gray.into()
}

fn to_color_map(img: &RGBSimilarityImage) -> DynamicImage {
    let mut img_rgb = RgbImage::new(img.width(), img.height());
    for row in 0..img.height() {
        for col in 0..img.width() {
            let pixel = img.get_pixel(col, row);
            let mut new_pixel = [0u8; 3];
            for channel in 0..3 {
                new_pixel[channel] = (pixel[channel].clamp(0., 1.) * 255.) as u8;
            }
            img_rgb.put_pixel(col, row, Rgb(new_pixel));
        }
    }
    img_rgb.into()
}

fn to_color_map_rgba(img: &RGBASimilarityImage) -> DynamicImage {
    let mut img_rgba = RgbaImage::new(img.width(), img.height());
    for row in 0..img.height() {
        for col in 0..img.width() {
            let pixel = img.get_pixel(col, row);
            let mut new_pixel = [0u8; 4];
            for channel in 0..4 {
                new_pixel[channel] = (pixel[channel].clamp(0., 1.) * 255.) as u8;
            }
            img_rgba.put_pixel(col, row, Rgba(new_pixel));
        }
    }
    img_rgba.into()
}

impl SimilarityImage {
    pub fn to_color_map(&self) -> DynamicImage {
        match self {
            SimilarityImage::Gray(gray) => gray_map(gray),
            SimilarityImage::RGB(rgb) => to_color_map(rgb),
            SimilarityImage::RGBA(rgba) => to_color_map_rgba(rgba),
        }
    }
}

#[derive(Debug)]
/// the resulting struct containing both an image of per pixel diffs as well as an average score
pub struct Similarity {
    /// Contains the resulting differences per pixel if applicable
    /// The buffer will contain the resulting values of the respective algorithms:
    /// - RMS will be between 0. for all-white vs all-black and 1.0 for identical
    /// - SSIM usually is near 1. for similar, near 0. for different but can take on negative values for negative covariances
    /// - Hybrid mode will be inverse: 0. means no difference, 1.0 is maximum difference. For details see [`crate::hybrid::rgb_hybrid_compare`]    
    pub image: SimilarityImage,
    /// the average score of the image
    pub score: f64,
}
