//! 开发期 GIF 质量门禁。
//!
//! 默认测试只校验 `image-compare` 接线；需要外部 ffmpeg 的对照通过
//! `PIXIV_UGOIRA_QUALITY_FFMPEG=1` 显式启用，避免离线测试依赖系统命令。

use std::fs::{self, File};
use std::io::{Cursor, Read, Write};
use std::path::{Path, PathBuf};
use std::process::Command;

use gif::{DisposalMethod, Repeat};
use image::{DynamicImage, ImageBuffer, ImageFormat, Rgb, RgbImage, RgbaImage};
use image_compare::Algorithm;
use tempfile::tempdir;
use ugoira_rs::{encode_gif, parse_frames_json, CancellationToken, UgoiraFrame};
use zip::write::SimpleFileOptions;
use zip::ZipArchive;

const QUALITY_ENV: &str = "PIXIV_UGOIRA_QUALITY_FFMPEG";
const QUALITY_ZIP_ENV: &str = "PIXIV_UGOIRA_QUALITY_ZIP";
const QUALITY_FRAMES_ENV: &str = "PIXIV_UGOIRA_QUALITY_FRAMES";
const QUALITY_OUTPUT_ENV: &str = "PIXIV_UGOIRA_QUALITY_OUTPUT_DIR";
const QUALITY_FFMPEG_BIN_ENV: &str = "PIXIV_UGOIRA_QUALITY_FFMPEG_BIN";
const QUALITY_CASE_ENV: &str = "PIXIV_UGOIRA_QUALITY_CASE";

#[derive(Debug)]
struct DecodedGif {
    frames: Vec<RgbImage>,
    delays: Vec<u16>,
    repeat: Repeat,
}

#[test]
fn image_compare_reports_identical_rgb_images_as_one() {
    let image = ImageBuffer::from_pixel(16, 16, Rgb([32, 64, 128]));
    let result = image_compare::rgb_similarity_structure(&Algorithm::MSSIMSimple, &image, &image)
        .expect("same-size images should be comparable");

    assert_eq!(result.score, 1.0);
}

#[test]
fn synthetic_gif_ssim_is_not_lower_than_ffmpeg() {
    if !ffmpeg_quality_enabled() {
        eprintln!("skipping ffmpeg quality gate; set {QUALITY_ENV}=1 to enable");
        return;
    }
    require_ffmpeg();

    for (name, sources) in synthetic_scenes() {
        let dir = tempdir().expect("create synthetic quality directory");
        let zip_path = dir.path().join("frames.zip");
        let frames = write_png_zip(&zip_path, &sources);
        run_quality_case(name, &zip_path, &frames, &sources, dir.path(), true);
    }
}

#[test]
fn public_ugoira_gif_ssim_is_not_lower_than_ffmpeg() {
    if !ffmpeg_quality_enabled() {
        eprintln!("skipping public ffmpeg quality gate; set {QUALITY_ENV}=1 to enable");
        return;
    }
    require_ffmpeg();
    let zip_path = PathBuf::from(
        std::env::var_os(QUALITY_ZIP_ENV)
            .unwrap_or_else(|| panic!("{QUALITY_ZIP_ENV} is required when {QUALITY_ENV}=1")),
    );
    let frames_path = PathBuf::from(
        std::env::var_os(QUALITY_FRAMES_ENV)
            .unwrap_or_else(|| panic!("{QUALITY_FRAMES_ENV} is required when {QUALITY_ENV}=1")),
    );

    let frames_json = fs::read_to_string(&frames_path).expect("read public frames JSON");
    let frames = parse_frames_json(&frames_json).expect("parse public frames JSON");
    let sources = read_source_frames(&zip_path, &frames);
    let dir = tempdir().expect("create public quality directory");
    let case_name = std::env::var(QUALITY_CASE_ENV).unwrap_or_else(|_| "public-sample".to_string());
    run_quality_case(&case_name, &zip_path, &frames, &sources, dir.path(), false);
}

fn ffmpeg_quality_enabled() -> bool {
    std::env::var(QUALITY_ENV).as_deref() == Ok("1")
}

fn ffmpeg_binary() -> std::ffi::OsString {
    std::env::var_os(QUALITY_FFMPEG_BIN_ENV).unwrap_or_else(|| "ffmpeg".into())
}

fn require_ffmpeg() {
    let binary = ffmpeg_binary();
    let output = Command::new(&binary)
        .arg("-version")
        .output()
        .unwrap_or_else(|err| {
            panic!(
                "ffmpeg quality reference {:?} is unavailable: {err}",
                binary
            )
        });
    assert!(
        output.status.success(),
        "ffmpeg quality reference {:?} exited with {}: {}",
        binary,
        output.status,
        String::from_utf8_lossy(&output.stderr)
    );
}

fn run_quality_case(
    name: &str,
    zip_path: &Path,
    frames: &[UgoiraFrame],
    sources: &[RgbImage],
    work_dir: &Path,
    enforce_each_frame: bool,
) {
    let rust_path = work_dir.join("rust.gif");
    let ffmpeg_path = work_dir.join("ffmpeg.gif");
    encode_gif(
        zip_path,
        frames,
        &rust_path,
        0,
        &CancellationToken::default(),
    )
    .expect("encode Rust GIF");
    encode_ffmpeg_gif(zip_path, frames, &ffmpeg_path, work_dir);

    let rust = decode_gif(&rust_path);
    let ffmpeg = decode_gif(&ffmpeg_path);
    assert_eq!(rust.frames.len(), sources.len(), "{name}: Rust frame count");
    assert_eq!(
        ffmpeg.frames.len(),
        sources.len(),
        "{name}: ffmpeg frame count"
    );
    assert_eq!(rust.repeat, Repeat::Infinite, "{name}: Rust loop");
    assert_eq!(ffmpeg.repeat, Repeat::Infinite, "{name}: ffmpeg loop");
    let expected_delays: Vec<u16> = frames
        .iter()
        .map(|frame| u16::try_from((frame.delay + 9) / 10).expect("synthetic delay fits GIF"))
        .collect();
    assert_eq!(rust.delays, expected_delays, "{name}: Rust frame delays");

    let scores = QualityScores {
        rust_luma: frame_luma_ssim_scores(sources, &rust.frames),
        ffmpeg_luma: frame_luma_ssim_scores(sources, &ffmpeg.frames),
        rust_rgb: frame_rgb_ssim_scores(sources, &rust.frames),
        ffmpeg_rgb: frame_rgb_ssim_scores(sources, &ffmpeg.frames),
    };
    if enforce_each_frame {
        assert_each_frame_not_lower(name, "luma", &scores.rust_luma, &scores.ffmpeg_luma);
        assert_each_frame_not_lower(name, "RGB", &scores.rust_rgb, &scores.ffmpeg_rgb);
    }
    let rust_luma_average = average(&scores.rust_luma);
    let ffmpeg_luma_average = average(&scores.ffmpeg_luma);
    let rust_rgb_average = average(&scores.rust_rgb);
    let ffmpeg_rgb_average = average(&scores.ffmpeg_rgb);
    eprintln!(
        "QUALITY_RESULT case={name} frames={} rust_luma_ssim={rust_luma_average:.9} ffmpeg_luma_ssim={ffmpeg_luma_average:.9} rust_rgb_ssim={rust_rgb_average:.9} ffmpeg_rgb_ssim={ffmpeg_rgb_average:.9}",
        sources.len()
    );
    assert!(
        rust_luma_average + 1e-12 >= ffmpeg_luma_average,
        "{name}: average Rust luma SSIM {rust_luma_average:.9} < ffmpeg {ffmpeg_luma_average:.9}"
    );
    assert!(
        rust_rgb_average + 1e-12 >= ffmpeg_rgb_average,
        "{name}: average Rust RGB SSIM {rust_rgb_average:.9} < ffmpeg {ffmpeg_rgb_average:.9}"
    );
    persist_quality_artifacts(name, &rust_path, &ffmpeg_path, frames, &scores);
}

struct QualityScores {
    rust_luma: Vec<f64>,
    ffmpeg_luma: Vec<f64>,
    rust_rgb: Vec<f64>,
    ffmpeg_rgb: Vec<f64>,
}

fn assert_each_frame_not_lower(name: &str, metric: &str, rust: &[f64], ffmpeg: &[f64]) {
    for (index, (rust_score, ffmpeg_score)) in rust.iter().zip(ffmpeg.iter()).enumerate() {
        assert!(
            rust_score + 1e-12 >= *ffmpeg_score,
            "{name}: frame {index} Rust {metric} SSIM {rust_score:.9} < ffmpeg {ffmpeg_score:.9}"
        );
    }
}

fn persist_quality_artifacts(
    name: &str,
    rust_path: &Path,
    ffmpeg_path: &Path,
    frames: &[UgoiraFrame],
    scores: &QualityScores,
) {
    let Some(output_dir) = std::env::var_os(QUALITY_OUTPUT_ENV).map(PathBuf::from) else {
        return;
    };
    fs::create_dir_all(&output_dir).expect("create quality artifact directory");
    fs::copy(rust_path, output_dir.join(format!("{name}-rust.gif")))
        .expect("persist Rust quality GIF");
    fs::copy(ffmpeg_path, output_dir.join(format!("{name}-ffmpeg.gif")))
        .expect("persist ffmpeg quality GIF");
    let mut csv = String::from(
        "frame,file,delay_ms,rust_luma_ssim,ffmpeg_luma_ssim,rust_rgb_ssim,ffmpeg_rgb_ssim\n",
    );
    for (index, frame) in frames.iter().enumerate() {
        csv.push_str(&format!(
            "{index},{},{},{:.9},{:.9},{:.9},{:.9}\n",
            frame.file,
            frame.delay,
            scores.rust_luma[index],
            scores.ffmpeg_luma[index],
            scores.rust_rgb[index],
            scores.ffmpeg_rgb[index],
        ));
    }
    fs::write(output_dir.join(format!("{name}-ssim.csv")), csv).expect("persist quality SSIM CSV");
}

fn encode_ffmpeg_gif(zip_path: &Path, frames: &[UgoiraFrame], output_path: &Path, work_dir: &Path) {
    let extracted = work_dir.join("ffmpeg-frames");
    fs::create_dir(&extracted).expect("create ffmpeg frame directory");
    let mut archive =
        ZipArchive::new(File::open(zip_path).expect("open source zip")).expect("parse source zip");
    let mut concat = String::new();
    for frame in frames {
        let file_name = Path::new(&frame.file)
            .file_name()
            .and_then(|name| name.to_str())
            .expect("frame name");
        assert_eq!(file_name, frame.file, "quality frame names must be flat");
        let mut source = archive.by_name(&frame.file).expect("find frame in zip");
        let mut output = File::create(extracted.join(file_name)).expect("create extracted frame");
        std::io::copy(&mut source, &mut output).expect("extract frame");
        concat.push_str(&format!(
            "file '{file_name}'\nduration {:.3}\n",
            frame.delay as f64 / 1000.0
        ));
    }
    fs::write(extracted.join("frame_list.txt"), concat).expect("write ffmpeg concat list");

    let output = Command::new(ffmpeg_binary())
        .current_dir(&extracted)
        .args([
            "-v",
            "error",
            "-f",
            "concat",
            "-safe",
            "0",
            "-i",
            "frame_list.txt",
            "-vf",
            "split[s0][s1];[s0]palettegen=stats_mode=full[p];[s1][p]paletteuse",
            "-y",
        ])
        .arg(output_path)
        .output()
        .expect("run ffmpeg quality reference");
    assert!(
        output.status.success(),
        "ffmpeg failed: {}",
        String::from_utf8_lossy(&output.stderr)
    );
}

fn decode_gif(path: &Path) -> DecodedGif {
    let mut options = gif::DecodeOptions::new();
    options.set_color_output(gif::ColorOutput::RGBA);
    let mut decoder = options
        .read_info(File::open(path).expect("open GIF"))
        .expect("read GIF metadata");
    let width = u32::from(decoder.width());
    let height = u32::from(decoder.height());
    let repeat = decoder.repeat();
    let mut canvas = RgbaImage::new(width, height);
    let mut frames = Vec::new();
    let mut delays = Vec::new();
    while let Some(frame) = decoder.read_next_frame().expect("decode GIF frame") {
        let before = canvas.clone();
        let frame_width = u32::from(frame.width);
        let frame_height = u32::from(frame.height);
        for y in 0..frame_height {
            for x in 0..frame_width {
                let source = ((y * frame_width + x) * 4) as usize;
                let pixel = &frame.buffer[source..source + 4];
                if pixel[3] != 0 {
                    canvas.put_pixel(
                        u32::from(frame.left) + x,
                        u32::from(frame.top) + y,
                        image::Rgba([pixel[0], pixel[1], pixel[2], pixel[3]]),
                    );
                }
            }
        }
        frames.push(DynamicImage::ImageRgba8(canvas.clone()).into_rgb8());
        delays.push(frame.delay);
        match frame.dispose {
            DisposalMethod::Background => {
                for y in 0..frame_height {
                    for x in 0..frame_width {
                        canvas.put_pixel(
                            u32::from(frame.left) + x,
                            u32::from(frame.top) + y,
                            image::Rgba([0, 0, 0, 0]),
                        );
                    }
                }
            }
            DisposalMethod::Previous => canvas = before,
            DisposalMethod::Any | DisposalMethod::Keep => {}
        }
    }
    DecodedGif {
        frames,
        delays,
        repeat,
    }
}

fn frame_luma_ssim_scores(reference: &[RgbImage], actual: &[RgbImage]) -> Vec<f64> {
    reference
        .iter()
        .zip(actual)
        .map(|(reference, actual)| {
            // Luma 作为结构辅助指标；RGB SSIM 才是颜色质量主门禁。
            image_compare::gray_similarity_structure(
                &Algorithm::MSSIMSimple,
                &DynamicImage::ImageRgb8(reference.clone()).into_luma8(),
                &DynamicImage::ImageRgb8(actual.clone()).into_luma8(),
            )
            .expect("compare same-size GIF frame")
            .score
        })
        .collect()
}

fn frame_rgb_ssim_scores(reference: &[RgbImage], actual: &[RgbImage]) -> Vec<f64> {
    reference
        .iter()
        .zip(actual)
        .map(|(reference, actual)| average_channel_ssim(reference, actual))
        .collect()
}

fn average_channel_ssim(reference: &RgbImage, actual: &RgbImage) -> f64 {
    assert_eq!(reference.dimensions(), actual.dimensions());
    let mut total = 0.0;
    for channel in 0..3 {
        let reference_channel =
            ImageBuffer::from_fn(reference.width(), reference.height(), |x, y| {
                image::Luma([reference.get_pixel(x, y).0[channel]])
            });
        let actual_channel = ImageBuffer::from_fn(actual.width(), actual.height(), |x, y| {
            image::Luma([actual.get_pixel(x, y).0[channel]])
        });
        total += image_compare::gray_similarity_structure(
            &Algorithm::MSSIMSimple,
            &reference_channel,
            &actual_channel,
        )
        .expect("compare same-size GIF RGB channel")
        .score;
    }
    total / 3.0
}

fn average(values: &[f64]) -> f64 {
    values.iter().sum::<f64>() / values.len() as f64
}

fn read_source_frames(zip_path: &Path, frames: &[UgoiraFrame]) -> Vec<RgbImage> {
    let mut archive =
        ZipArchive::new(File::open(zip_path).expect("open source zip")).expect("parse source zip");
    frames
        .iter()
        .map(|frame| {
            let mut entry = archive.by_name(&frame.file).expect("find source frame");
            let mut body = Vec::new();
            entry.read_to_end(&mut body).expect("read source frame");
            image::load_from_memory(&body)
                .expect("decode source frame")
                .into_rgb8()
        })
        .collect()
}

fn write_png_zip(path: &Path, sources: &[RgbImage]) -> Vec<UgoiraFrame> {
    let file = File::create(path).expect("create synthetic zip");
    let mut archive = zip::ZipWriter::new(file);
    let mut frames = Vec::with_capacity(sources.len());
    for (index, image) in sources.iter().enumerate() {
        let name = format!("{index:06}.png");
        archive
            .start_file(&name, SimpleFileOptions::default())
            .expect("start synthetic frame");
        let mut body = Cursor::new(Vec::new());
        DynamicImage::ImageRgb8(image.clone())
            .write_to(&mut body, ImageFormat::Png)
            .expect("encode synthetic PNG");
        archive
            .write_all(body.get_ref())
            .expect("write synthetic frame");
        frames.push(UgoiraFrame {
            file: name,
            delay: [80, 60, 40, 20][index % 4],
        });
    }
    archive.finish().expect("finish synthetic zip");
    frames
}

fn synthetic_scenes() -> Vec<(&'static str, Vec<RgbImage>)> {
    vec![
        ("gradient-motion", gradient_motion_scene()),
        ("color-grid", color_grid_scene()),
        ("textured-pan", textured_pan_scene()),
    ]
}

fn gradient_motion_scene() -> Vec<RgbImage> {
    (0..4)
        .map(|frame| {
            ImageBuffer::from_fn(96, 64, |x, y| {
                let mut rgb = [
                    ((x * 255) / 95) as u8,
                    ((y * 255) / 63) as u8,
                    (((x + y) * 255) / 158) as u8,
                ];
                if x >= 8 + frame * 16 && x < 28 + frame * 16 && (20..44).contains(&y) {
                    rgb = [245, 48, 180];
                }
                Rgb(rgb)
            })
        })
        .collect()
}

fn color_grid_scene() -> Vec<RgbImage> {
    (0..4)
        .map(|frame| {
            ImageBuffer::from_fn(96, 64, |x, y| {
                let cell = ((x / 8) + (y / 8) * 12 + frame * 3) as u8;
                Rgb([
                    cell.wrapping_mul(37).wrapping_add((x * 3) as u8),
                    cell.wrapping_mul(73).wrapping_add((y * 5) as u8),
                    cell.wrapping_mul(109).wrapping_add(((x + y) * 2) as u8),
                ])
            })
        })
        .collect()
}

fn textured_pan_scene() -> Vec<RgbImage> {
    (0..4)
        .map(|frame| {
            ImageBuffer::from_fn(96, 64, |x, y| {
                let shifted = x + frame * 7;
                let seed = shifted
                    .wrapping_mul(1_103_515_245)
                    .wrapping_add(y.wrapping_mul(12_345));
                let noise = (seed ^ (seed >> 11) ^ (seed >> 19)) as u8;
                Rgb([
                    noise,
                    noise.wrapping_add((y * 3) as u8),
                    noise.wrapping_add((shifted * 2) as u8),
                ])
            })
        })
        .collect()
}
