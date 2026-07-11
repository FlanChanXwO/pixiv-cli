use crate::prelude::*;
use image::RgbaImage;
use itertools::izip;

/// see https://www.itu.int/rec/T-REC-T.871
pub fn rgb_to_yuv(rgb: &[f32; 3]) -> [f32; 3] {
    let py = 0. + (0.299 * rgb[0]) + (0.587 * rgb[1]) + (0.114 * rgb[2]);
    let pu = 128. - (0.168736 * rgb[0]) - (0.331264 * rgb[1]) + (0.5 * rgb[2]);
    let pv = 128. + (0.5 * rgb[0]) - (0.418688 * rgb[1]) - (0.081312 * rgb[2]);
    [py, pu, pv]
}

/// see https://www.itu.int/rec/T-REC-T.871
#[allow(dead_code)]
pub fn yuv_to_rgb(yuv: &[f32; 3]) -> [f32; 3] {
    let r = yuv[0] + (1.402 * (yuv[2] - 128.));
    let g = yuv[0] - (0.344136 * (yuv[1] - 128.)) - (0.714136 * (yuv[2] - 128.));
    let b = yuv[0] + (1.772 * (yuv[1] - 128.));
    [r, b, g]
}

pub(crate) fn split_rgba_to_yuva(source: &RgbaImage) -> [GrayImage; 4] {
    let mut y = GrayImage::new(source.width(), source.height());
    let mut u = y.clone();
    let mut v = y.clone();
    let mut a = y.clone();

    izip!(
        y.pixels_mut(),
        u.pixels_mut(),
        v.pixels_mut(),
        a.pixels_mut(),
        source.pixels()
    )
    .for_each(|(y, u, v, a, rgba)| {
        let rgba: [f32; 4] = rgba.0.map(|c| c as f32);
        let yuv = rgb_to_yuv(&rgba[0..3].try_into().unwrap());
        *y = Luma([yuv[0].clamp(0., 255.) as u8]);
        *u = Luma([yuv[1].clamp(0., 255.) as u8]);
        *v = Luma([yuv[2].clamp(0., 255.) as u8]);
        *a = Luma([rgba[3] as u8]);
    });

    [y, u, v, a]
}

fn blend(c: u8, a: u8, c_b: u8) -> u8 {
    let c = c as f32 / 255.;
    let a = a as f32 / 255.;
    let c_b = c_b as f32 / 255.;
    let blended = (c * a) + (c_b * (1. - a));
    (blended.clamp(0., 1.) * 255.) as u8
}

pub(crate) fn blend_alpha(image: &RgbaImage, color: Rgb<u8>) -> RgbImage {
    let mut buffer = ImageBuffer::new(image.width(), image.height());

    for (input, output) in image.pixels().zip(buffer.pixels_mut()) {
        let [r, g, b, a] = input.0;

        *output = Rgb([
            blend(r, a, color.0[0]),
            blend(g, a, color.0[1]),
            blend(b, a, color.0[2]),
        ]);
    }
    buffer
}

/// Holds methods to split an RgbImage into arrays of channels
pub trait Decompose {
    /// Returns an array containing 3 `GrayImage`s where each channel represents [r, g, b]
    fn split_channels(&self) -> [GrayImage; 3];
    /// Converts an image to yuv then returns an array containing 3 `GrayImage`s where each channel represents [y, u, v]
    fn split_to_yuv(&self) -> [GrayImage; 3];
}

impl Decompose for RgbImage {
    fn split_channels(&self) -> [GrayImage; 3] {
        let mut red = GrayImage::new(self.width(), self.height());
        let mut green = red.clone();
        let mut blue = red.clone();
        izip!(
            red.pixels_mut(),
            green.pixels_mut(),
            blue.pixels_mut(),
            self.pixels()
        )
        .for_each(|(r, g, b, rgb)| {
            *r = Luma([rgb[0]]);
            *g = Luma([rgb[1]]);
            *b = Luma([rgb[2]]);
        });

        [red, green, blue]
    }

    fn split_to_yuv(&self) -> [GrayImage; 3] {
        let mut y = GrayImage::new(self.width(), self.height());
        let mut u = y.clone();
        let mut v = y.clone();
        izip!(
            y.pixels_mut(),
            u.pixels_mut(),
            v.pixels_mut(),
            self.pixels()
        )
        .for_each(|(y, u, v, rgb)| {
            let yuv = rgb_to_yuv(&rgb.0.map(|c| c as f32));
            *y = Luma([yuv[0].clamp(0., 255.) as u8]);
            *u = Luma([yuv[1].clamp(0., 255.) as u8]);
            *v = Luma([yuv[2].clamp(0., 255.) as u8]);
        });

        [y, u, v]
    }
}

pub(crate) fn merge_similarity_channels(input: &[&GraySimilarityImage; 3]) -> RGBSimilarityImage {
    let mut output = RGBSimilarityImage::new(input[0].width(), input[0].height());
    izip!(
        input[0].pixels(),
        input[1].pixels(),
        input[2].pixels(),
        output.pixels_mut()
    )
    .for_each(|p| {
        *p.3 = Rgb([p.0[0], p.1[0], p.2[0]]);
    });

    output
}

pub(crate) struct Window {
    pub top_left: (u32, u32),
    pub bottom_right: (u32, u32),
}

pub(crate) struct WindowIter<'a> {
    current_index: u32,
    window: &'a Window,
}

impl<'a> Iterator for WindowIter<'a> {
    type Item = (u32, u32);
    fn next(&mut self) -> Option<Self::Item> {
        let result = Some((
            self.window.top_left.0 + (self.current_index % self.window.width()),
            self.window.top_left.1 + (self.current_index / self.window.width()),
        ));
        self.current_index += 1;
        if self.current_index <= self.window.area() {
            result
        } else {
            None
        }
    }
}

impl Window {
    pub fn new(top_left: (u32, u32), bottom_right: (u32, u32)) -> Window {
        Window {
            top_left,
            bottom_right,
        }
    }
    pub fn width(&self) -> u32 {
        self.bottom_right.0 - self.top_left.0 + 1
    }

    pub fn height(&self) -> u32 {
        self.bottom_right.1 - self.top_left.1 + 1
    }

    pub fn area(&self) -> u32 {
        self.width() * self.height()
    }

    pub fn subdivide_by_offset(&self, offset: u32) -> Vec<Window> {
        let mut result = Vec::new();
        for col in (self.top_left.0..self.width()).step_by(offset as usize) {
            for row in (self.top_left.1..self.height()).step_by(offset as usize) {
                result.push(Window::new(
                    (col, row),
                    (
                        (col + offset - 1).min(self.bottom_right.0),
                        (row + offset - 1).min(self.bottom_right.1),
                    ),
                ))
            }
        }
        result
    }

    pub fn iter_pixels(&'_ self) -> WindowIter<'_> {
        WindowIter {
            window: self,
            current_index: 0,
        }
    }

    pub fn from_image(image: &GrayImage) -> Window {
        Window {
            top_left: (0, 0),
            bottom_right: (image.width() - 1, image.height() - 1),
        }
    }
}

pub(crate) fn draw_window_to_image(window: &Window, image: &mut GraySimilarityImage, val: f32) {
    window
        .iter_pixels()
        .for_each(|current_pixel| image.put_pixel(current_pixel.0, current_pixel.1, Luma([val])));
}

#[cfg(test)]
mod tests {
    use super::*;
    use image::Rgba;

    #[test]
    fn window_test() {
        let rows = 2;
        let cols = 9;
        let window = Window::new((1, 1), (cols, rows));
        assert_eq!(window.height(), rows);
        assert_eq!(window.width(), cols);
        assert_eq!(window.area(), rows * cols);
    }

    #[test]
    fn window_test_edge() {
        let window = Window::new((0, 0), (0, 0));
        assert_eq!(window.height(), 1);
        assert_eq!(window.width(), 1);
        assert_eq!(window.area(), 1);
    }

    #[test]
    fn window_iterator_test() {
        let window = Window::new((0, 0), (3, 2));
        let mut iter = window.iter_pixels();

        let next = iter.next().expect("iterator should work");
        assert_eq!(next.0, 0);
        assert_eq!(next.1, 0);

        //expect column-first iteration
        let next = iter.next().expect("iterator should work");
        assert_eq!(next.0, 1);
        assert_eq!(next.1, 0);

        //row break works
        let next = iter.nth(3).expect("iterator should work");
        assert_eq!(next.0, 1);
        assert_eq!(next.1, 1);
    }

    #[test]
    fn window_subdivide_test() {
        let window = Window::new((0, 0), (8, 7));
        let windows = window.subdivide_by_offset(8);
        //all windows areas combined are the original area
        assert_eq!(windows.iter().map(|w| w.area()).sum::<u32>(), window.area());
    }

    #[test]
    fn from_image_test() {
        let img = GrayImage::new(127, 244);
        let window = Window::from_image(&img);
        assert_eq!(window.bottom_right.0, 126);
        assert_eq!(window.bottom_right.1, 243);
    }

    #[test]
    fn rgb_to_yuv_test() {
        let white = [255., 255., 255.];
        let black = [0., 0., 0.];
        let white_yuv = rgb_to_yuv(&white);
        assert_eq!(white_yuv[0], 255.);
        assert_eq!(white_yuv[1], 128.);
        assert_eq!(white_yuv[2], 128.);

        let black_yuv = rgb_to_yuv(&black);
        assert_eq!(black_yuv[0], 0.);
        assert_eq!(black_yuv[1], 128.);
        assert_eq!(black_yuv[2], 128.);
    }

    #[test]
    fn yuv_to_rgb_test() {
        let white_yuv = [255., 128., 128.];
        let black_yuv = [0., 128., 128.];
        let white = yuv_to_rgb(&white_yuv);
        assert_eq!(white[0], 255.);
        assert_eq!(white[1], 255.);
        assert_eq!(white[2], 255.);

        let black = yuv_to_rgb(&black_yuv);
        assert_eq!(black[0], 0.);
        assert_eq!(black[1], 0.);
        assert_eq!(black[2], 0.);
    }

    #[test]
    fn blend_test() {
        // black with white background but no alpha = white
        assert_eq!(blend(0, 0, 255), 255);
        // white with black background and full alpha = white
        assert_eq!(blend(255, 255, 0), 255);
        // white with black background and no alpha = black
        assert_eq!(blend(255, 0, 0), 0);
        // white with black background and half alpha = gray
        assert_eq!(blend(255, 127, 0), 127);
    }

    #[test]
    fn blend_image_test() {
        let test_image = RgbaImage::from_pixel(2, 2, Rgba([255, 0, 127, 127]));
        let pre_mult = blend_alpha(&test_image, Rgb([255, 255, 255]));
        let b_target = 127 + 127 / 2 + 1;
        assert_eq!(pre_mult.get_pixel(0, 0).0, [255u8, 127u8, b_target]);
    }
}
