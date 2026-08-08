use crate::prelude::*;
use itertools::izip;

pub(crate) fn root_mean_squared_error_simple(
    first: &GrayImage,
    second: &GrayImage,
) -> Result<(f64, GraySimilarityImage), CompareError> {
    let dimension = first.dimensions();
    let mut image = GraySimilarityImage::new(dimension.0, dimension.1);
    let iter = izip!(first.pixels(), second.pixels(), image.pixels_mut());

    iter.for_each(|(a, b, c)| {
        let diff = a[0] as i32 - b[0] as i32;
        let normalized = diff as f32 / u8::MAX as f32;
        let squared_root = 1. - normalized.abs();
        *c = Luma([squared_root]);
    });

    let score: f64 = 1.
        - (image
            .pixels()
            .map(|p| (1. - p[0] as f64).powi(2))
            .sum::<f64>()
            / (image.pixels().len() as f64))
            .sqrt();
    Ok((score, image))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rms_simple_10() {
        let mut first = GrayImage::new(1, 1);
        let mut second = GrayImage::new(1, 1);

        first.fill(0);
        second.fill(10);
        let (_, comparison) =
            root_mean_squared_error_simple(&first, &second).expect("Do not expect error here");
        assert_eq!(comparison.get_pixel(0, 0)[0], 1. - (10. / (255.0f32)));
    }

    #[test]
    fn rms_simple_identity() {
        let mut first = GrayImage::new(1, 1);
        let mut second = GrayImage::new(1, 1);

        first.fill(0);
        second.fill(0);
        let (score, comparison) =
            root_mean_squared_error_simple(&first, &second).expect("Do not expect error here");
        assert_eq!(comparison.get_pixel(0, 0)[0], 1.);
        assert_eq!(score, 1.);
    }

    #[test]
    fn rms_simple_max() {
        let mut first = GrayImage::new(1, 1);
        let mut second = GrayImage::new(1, 1);

        first.fill(0);
        second.fill(255);
        let (score, comparison) =
            root_mean_squared_error_simple(&first, &second).expect("Do not expect error here");
        assert_eq!(comparison.get_pixel(0, 0)[0], 0.);
        assert_eq!(score, 0.);
    }

    #[test]
    fn rms_simple() {
        let width = 3;
        let height = 2;
        let mut first = GrayImage::new(width, height);
        let mut second = GrayImage::new(width, height);

        first.fill(0);
        second.fill(0);
        second.put_pixel(1, 1, Luma([127]));

        let (comparison, _) =
            root_mean_squared_error_simple(&first, &second).expect("Do not expect error here");

        let result = 1. - ((127. / 255.0f64).powi(2) / (width * height) as f64).sqrt();
        assert!((comparison - result).abs() < 1e-5);
    }
}
