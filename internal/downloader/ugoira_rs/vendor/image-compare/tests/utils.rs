use cucumber::{given, then, when, World};
use image::DynamicImage;
use image_compare::prelude::*;
extern crate image;

#[cfg(feature = "yuv_compare")] // Do not remove, ensures rgb_to_yuv stays public not pub(crate) or private
use image_compare::utils::rgb_to_yuv;
#[cfg(feature = "yuv_compare")] // Do not remove, ensures yuv_to_rgb stays public not pub(crate) or private
use image_compare::utils::yuv_to_rgb;

// `World` is your shared, likely mutable state.
#[derive(Debug, World, Default)]
pub struct SplitWorld {
    image: Option<DynamicImage>,
    channel_r: Option<GrayImage>,
    channel_g: Option<GrayImage>,
    channel_b: Option<GrayImage>,
    channel_y: Option<GrayImage>,
    channel_u: Option<GrayImage>,
    channel_v: Option<GrayImage>,
    success: Option<bool>
}

#[cfg(feature = "yuv_compare")]
#[given(expr = "the image {string} is loaded")]
fn load_images(world: &mut SplitWorld, image: String) {
    let full_image = image::open(image.clone()).expect(format!("Could not find test-image {}", image).as_str());
    world.image = Some(full_image.clone());
    world.channel_y = Some(image::open(image.clone().replace(".png", "_y_channel.png")).expect("Could not load y_channel").to_luma8()); // Automatically loads images
    world.channel_u = Some(image::open(image.clone().replace(".png", "_u_channel.png")).expect("Could not load u_channel").to_luma8());
    world.channel_v = Some(image::open(image.clone().replace(".png", "_v_channel.png")).expect("Could not load v_channel").to_luma8());
    world.channel_r = Some(image::open(image.clone().replace(".png", "_r_channel.png")).expect("Could not load r_channel").to_luma8());
    world.channel_g = Some(image::open(image.clone().replace(".png", "_g_channel.png")).expect("Could not load g_channel").to_luma8());
    world.channel_b = Some(image::open(image.clone().replace(".png", "_b_channel.png")).expect("Could not load b_channel").to_luma8());
}

#[cfg(feature = "yuv_compare")]
#[when(expr = "comparing yuv channels")]
fn decompose_yuv(world: &mut SplitWorld) {
    use image_compare::utils::Decompose;

    let [channel_y, channel_u, channel_v] = world.image.as_ref().unwrap().to_rgb8().split_to_yuv();
    let y_comp = image_compare::gray_similarity_structure(&Algorithm::RootMeanSquared, &channel_y, world.channel_y.as_ref().unwrap()).expect("RMS Gray compare error").score == 1.0;
    let u_comp = image_compare::gray_similarity_structure(&Algorithm::RootMeanSquared, &channel_u, world.channel_u.as_ref().unwrap()).expect("RMS Gray compare error").score == 1.0;
    let v_comp = image_compare::gray_similarity_structure(&Algorithm::RootMeanSquared, &channel_v, world.channel_v.as_ref().unwrap()).expect("RMS Gray compare error").score == 1.0;

    world.success = Some(y_comp && u_comp && v_comp);
}

#[cfg(feature = "yuv_compare")]
#[when(expr = "comparing rgb channels")]
fn decompose_rgb(world: &mut SplitWorld) {
    use image_compare::utils::Decompose;

    let [channel_r, channel_g, channel_b] = world.image.as_ref().unwrap().to_rgb8().split_channels();
    let r_comp = image_compare::gray_similarity_structure(&Algorithm::RootMeanSquared, &channel_r, world.channel_r.as_ref().unwrap()).expect("RMS Gray compare error").score == 1.0;
    let g_comp = image_compare::gray_similarity_structure(&Algorithm::RootMeanSquared, &channel_g, world.channel_g.as_ref().unwrap()).expect("RMS Gray compare error").score == 1.0;
    let b_comp = image_compare::gray_similarity_structure(&Algorithm::RootMeanSquared, &channel_b, world.channel_b.as_ref().unwrap()).expect("RMS Gray compare error").score == 1.0;

    world.success = Some(r_comp && g_comp && b_comp);
}

#[cfg(feature = "yuv_compare")]
#[then(expr = "check split success")]
fn check_success(world: &mut SplitWorld) {
    assert!(world.success.unwrap() == true);
}


#[tokio::main]
async fn main() {
    #[cfg(feature = "yuv_compare")]
    SplitWorld::run("tests/features/decompose.feature").await;
}