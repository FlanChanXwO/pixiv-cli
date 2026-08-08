use crate::prelude::*;
use crate::utils::{draw_window_to_image, Window};
use rayon::prelude::*;

const DEFAULT_WINDOW_SIZE: u32 = 8;
const K1: f64 = 0.01;
const K2: f64 = 0.03;
const L: u8 = u8::MAX;
const C1: f64 = (K1 * L as f64) * (K1 * L as f64);
const C2: f64 = (K2 * L as f64) * (K2 * L as f64);

pub(crate) fn ssim_simple(
    first: &GrayImage,
    second: &GrayImage,
) -> Result<(f64, GraySimilarityImage), CompareError> {
    let dimension = first.dimensions();
    let mut image = GraySimilarityImage::new(dimension.0, dimension.1);
    let window = Window::from_image(first);
    let windows = window.subdivide_by_offset(DEFAULT_WINDOW_SIZE);
    let results = windows
        .par_iter()
        .map(|w| (ssim_for_window(first, second, w), w))
        .collect::<Vec<_>>();
    let score = results.iter().map(|r| r.0 * r.1.area() as f64).sum::<f64>()
        / results.iter().map(|r| r.1.area() as f64).sum::<f64>();

    results
        .iter()
        .for_each(|r| draw_window_to_image(r.1, &mut image, r.0 as f32));

    Ok((score, image))
}

fn ssim_for_window(first: &GrayImage, second: &GrayImage, window: &Window) -> f64 {
    let mean_x = mean(first, window);
    let mean_y = mean(second, window);
    let variance_x = covariance(first, mean_x, first, mean_x, window);
    let variance_y = covariance(second, mean_y, second, mean_y, window);
    let covariance = covariance(first, mean_x, second, mean_y, window);
    let counter = (2. * mean_x * mean_y + C1) * (2. * covariance + C2);
    let denominator = (mean_x.powi(2) + mean_y.powi(2) + C1) * (variance_x + variance_y + C2);
    counter / denominator
}

fn covariance(
    image_x: &GrayImage,
    mean_x: f64,
    image_y: &GrayImage,
    mean_y: f64,
    window: &Window,
) -> f64 {
    window
        .iter_pixels()
        .map(|pixel| {
            let pixel_x: f64 = image_x.get_pixel(pixel.0, pixel.1)[0].into();
            let pixel_y: f64 = image_y.get_pixel(pixel.0, pixel.1)[0].into();

            (pixel_x - mean_x) * (pixel_y - mean_y)
        })
        .sum::<f64>()
}

fn mean(image: &GrayImage, window: &Window) -> f64 {
    let sum = window
        .iter_pixels()
        .map(|pixel| image.get_pixel(pixel.0, pixel.1)[0] as f64)
        .sum::<f64>();

    sum / window.area() as f64
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn conv_avg_simple() {
        let mut img = GrayImage::new(1, 1);
        img.fill(100);
        let window = Window::new((0, 0), (0, 0));
        let conv_result = mean(&img, &window);
        assert_eq!(conv_result, 100.);
    }

    #[test]
    fn conv_avg_identity() {
        let window = Window::new((0, 0), (9, 9));
        let mut img = GrayImage::new(window.width(), window.height());
        img.fill(1);
        let conv_result = mean(&img, &window);
        assert!((conv_result - 1.).abs() < 1e-5);
    }

    #[test]
    fn test_variance() {
        let window = Window::new((0, 0), (2, 2));
        let mut img = GrayImage::new(window.width(), window.height());
        img.fill(0);
        img.put_pixel(1, 1, Luma([9]));
        let avg = mean(&img, &window);
        let var = covariance(&img, avg, &img, avg, &window);
        assert_eq!(avg, 1.0);
        //64. by the central piece being (9-1)^2, 8 in total by each element contributing (0-1)^2
        assert_eq!(var, 64. + 8.);
    }

    #[test]
    fn test_ssim_identity() {
        let window = Window::new((0, 0), (2, 2));
        let mut img = GrayImage::new(window.width(), window.height());
        img.fill(0);
        img.put_pixel(1, 1, Luma([9]));
        let ssim_value = ssim_for_window(&img, &img, &window);
        assert_eq!(ssim_value, 1.0);
    }

    #[test]
    fn test_ssim_stability() {
        let window = Window::new((0, 0), (2, 2));
        let mut img = GrayImage::new(window.width(), window.height());
        img.fill(0);
        let ssim_value = ssim_for_window(&img, &img, &window);
        assert_eq!(ssim_value, 1.0);
    }

    #[test]
    fn test_ssim_different_mean() {
        let window = Window::new((0, 0), (2, 2));
        let mut img = GrayImage::new(window.width(), window.height());
        img.fill(0);
        let mut img_second = GrayImage::new(window.width(), window.height());
        img_second.fill(5);

        let ssim_value = ssim_for_window(&img, &img_second, &window);
        assert_eq!(ssim_value, 0.20641218950876916);
    }
}
