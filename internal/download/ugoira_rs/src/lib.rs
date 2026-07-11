use std::borrow::Cow;
use std::ffi::{CStr, CString};
use std::fs::File;
use std::io::{self, Cursor, Read, Write};
use std::os::raw::c_char;
use std::panic::{catch_unwind, AssertUnwindSafe};
use std::path::{Path, PathBuf};
use std::ptr::null_mut;
use std::sync::atomic::{AtomicBool, Ordering};

use crc32fast::Hasher as Crc32;
use gif::{DisposalMethod, Encoder, Frame, Repeat};
use image::{imageops::FilterType, ImageReader, RgbImage, RgbaImage};
use png::{BitDepth, ColorType, Compression, Encoder as PngEncoder, Filter};
use quantette::deps::palette::Srgb;
use quantette::dither::FloydSteinberg;
use quantette::{ImageRef, PaletteBuf, PaletteSize, Pipeline, QuantizeMethod};
use serde::Deserialize;
use thiserror::Error;
use zip::ZipArchive;

// 延续原型 262,144 个统计单元的固定上界，但保存精确 sRGB 样本，避免预量化损失。
const MAX_GLOBAL_COLOR_SAMPLES: usize = 1 << 18;
const RESERVOIR_SEED: u64 = 0x9e37_79b9_7f4a_7c15;
// 公开样本与三个 synthetic 场景的 SSIM 门禁表明 0.1 可减少全局 palette 的高频噪声，
// 同时仍执行 Floyd–Steinberg error diffusion；完整数据见 docs/benchmark/ugoira-146994178.md。
const FLOYD_STEINBERG_ERROR_DIFFUSION: f32 = 0.1;
const PNG_SIGNATURE: &[u8; 8] = b"\x89PNG\r\n\x1a\n";

#[derive(Debug, Default)]
pub struct CancellationToken {
    canceled: AtomicBool,
}

impl CancellationToken {
    pub fn cancel(&self) {
        self.canceled.store(true, Ordering::Release);
    }

    fn check(&self) -> Result<(), EncodeError> {
        if self.canceled.load(Ordering::Acquire) {
            return Err(EncodeError::Canceled);
        }
        Ok(())
    }
}

#[derive(Debug, Deserialize)]
pub struct UgoiraFrame {
    pub file: String,
    pub delay: i64,
}

#[derive(Debug, Error)]
pub enum EncodeError {
    #[error("frames json is invalid: {0}")]
    FramesJson(#[from] serde_json::Error),
    #[error("zip error: {0}")]
    Zip(#[from] zip::result::ZipError),
    #[error("io error: {0}")]
    Io(#[from] std::io::Error),
    #[error("image decode error: {0}")]
    Image(#[from] image::ImageError),
    #[error("gif encode error: {0}")]
    Gif(#[from] gif::EncodingError),
    #[error("apng encode error: {0}")]
    Apng(#[from] png::EncodingError),
    #[error("ugoira frames list is empty")]
    EmptyFrames,
    #[error("ugoira encoding canceled")]
    Canceled,
    #[error("frame {file} has non-positive delay {delay_ms}ms")]
    NonPositiveDelay { file: String, delay_ms: i64 },
    #[error("frame {file} delay {delay_ms}ms exceeds GIF delay field limit")]
    DelayTooLarge { file: String, delay_ms: i64 },
    #[error(
        "frame {file} delay {delay_ms}ms cannot be represented exactly by APNG u16 delay fields"
    )]
    ApngDelayTooLarge { file: String, delay_ms: i64 },
    #[error("ugoira has {frame_count} frames, exceeding APNG frame count limit")]
    ApngFrameCountTooLarge { frame_count: usize },
    #[error("APNG frame/chunk sequence exceeds u32 format limit")]
    ApngSequenceOverflow,
    #[error("frame {file} is {width}x{height}, exceeding GIF logical screen limits")]
    FrameTooLarge {
        file: String,
        width: u32,
        height: u32,
    },
    #[error("frame {file} is {width}x{height}, expected {expected_width}x{expected_height}")]
    FrameSizeMismatch {
        file: String,
        width: u32,
        height: u32,
        expected_width: u32,
        expected_height: u32,
    },
    #[error("image dimensions must be non-zero")]
    EmptyImage,
    #[error("global color statistics counter overflowed")]
    ColorStatisticsOverflow,
    #[error("color quantization failed: {0}")]
    Quantization(String),
    #[error("{0} pointer is null")]
    NullPointer(&'static str),
    #[error("{0} is not valid UTF-8")]
    InvalidUtf8(&'static str),
    #[error("invalid animation format code {0}; expected 0 (gif) or 1 (apng)")]
    InvalidAnimationFormat(u32),
}

#[derive(Clone, Copy, Debug)]
enum AnimationFormat {
    Gif,
    Apng,
}

impl TryFrom<u32> for AnimationFormat {
    type Error = EncodeError;

    fn try_from(value: u32) -> Result<Self, Self::Error> {
        match value {
            0 => Ok(Self::Gif),
            1 => Ok(Self::Apng),
            _ => Err(EncodeError::InvalidAnimationFormat(value)),
        }
    }
}

#[derive(Debug)]
struct GlobalColorStatistics {
    samples: Vec<Srgb<u8>>,
    opaque_pixels: u64,
    has_transparency: bool,
    reservoir_state: u64,
}

impl GlobalColorStatistics {
    fn new() -> Self {
        Self {
            samples: Vec::with_capacity(MAX_GLOBAL_COLOR_SAMPLES),
            opaque_pixels: 0,
            has_transparency: false,
            reservoir_state: RESERVOIR_SEED,
        }
    }

    fn add_image(&mut self, image: &RgbaImage) -> Result<(), EncodeError> {
        for pixel in image.pixels() {
            let [red, green, blue, alpha] = pixel.0;
            if alpha == 0 {
                self.has_transparency = true;
                continue;
            }
            self.opaque_pixels = self
                .opaque_pixels
                .checked_add(1)
                .ok_or(EncodeError::ColorStatisticsOverflow)?;
            let color = Srgb::new(red, green, blue);
            if self.samples.len() < MAX_GLOBAL_COLOR_SAMPLES {
                self.samples.push(color);
                continue;
            }
            // 固定种子的 reservoir sampling 保留精确 sRGB 样本，避免 6-bit bin 在
            // k-means 前再次量化；样本内存固定，不随像素或帧数增长。
            self.reservoir_state ^= self.reservoir_state >> 12;
            self.reservoir_state ^= self.reservoir_state << 25;
            self.reservoir_state ^= self.reservoir_state >> 27;
            let random = self.reservoir_state.wrapping_mul(0x2545_f491_4f6c_dd1d);
            let selected = random % self.opaque_pixels;
            if selected < MAX_GLOBAL_COLOR_SAMPLES as u64 {
                self.samples[selected as usize] = color;
            }
        }
        Ok(())
    }

    fn weighted_samples(&self) -> Vec<Srgb<u8>> {
        self.samples.clone()
    }
}

#[derive(Debug)]
struct FirstPass {
    original_size: (u32, u32),
    output_size: (u32, u32),
    histogram: GlobalColorStatistics,
}

#[derive(Clone, Copy, Debug)]
struct FrameRegion {
    x: u32,
    y: u32,
    width: u32,
    height: u32,
}

pub fn parse_frames_json(frames_json: &str) -> Result<Vec<UgoiraFrame>, EncodeError> {
    Ok(serde_json::from_str(frames_json)?)
}

pub fn encode_gif(
    zip_path: &Path,
    frames: &[UgoiraFrame],
    output_path: &Path,
    max_edge: u32,
    cancellation: &CancellationToken,
) -> Result<(), EncodeError> {
    if frames.is_empty() {
        return Err(EncodeError::EmptyFrames);
    }
    cancellation.check()?;
    let first_pass = collect_global_histogram(zip_path, frames, max_edge, cancellation)?;
    cancellation.check()?;
    let opaque_palette = build_global_palette(&first_pass.histogram)?;
    cancellation.check()?;
    write_global_palette_gif(
        zip_path,
        frames,
        output_path,
        &first_pass,
        &opaque_palette,
        cancellation,
    )
}

pub fn encode_gif_from_json(
    zip_path: &Path,
    frames_json: &str,
    output_path: &Path,
    max_edge: u32,
    cancellation: &CancellationToken,
) -> Result<(), EncodeError> {
    let frames = parse_frames_json(frames_json)?;
    encode_gif(zip_path, &frames, output_path, max_edge, cancellation)
}

pub fn encode_apng(
    zip_path: &Path,
    frames: &[UgoiraFrame],
    output_path: &Path,
    max_edge: u32,
    cancellation: &CancellationToken,
) -> Result<(), EncodeError> {
    if frames.is_empty() {
        return Err(EncodeError::EmptyFrames);
    }
    let frame_count =
        u32::try_from(frames.len()).map_err(|_| EncodeError::ApngFrameCountTooLarge {
            frame_count: frames.len(),
        })?;
    cancellation.check()?;

    let mut archive = open_archive(zip_path)?;
    let first_meta = &frames[0];
    let first_delay = apng_delay_fraction(first_meta.delay, &first_meta.file)?;
    let first = decode_frame(&mut archive, first_meta)?;
    let original_size = first.dimensions();
    if original_size.0 == 0 || original_size.1 == 0 {
        return Err(EncodeError::EmptyImage);
    }
    let output_size = scaled_dimensions(original_size.0, original_size.1, max_edge);
    let first = resize_frame(first, output_size);

    cancellation.check()?;
    let mut output = File::create(output_path)?;
    write_apng_header(&mut output, output_size, frame_count)?;
    let mut sequence = 0u64;
    write_apng_frame(
        &mut output,
        &first,
        FrameRegion {
            x: 0,
            y: 0,
            width: output_size.0,
            height: output_size.1,
        },
        first_delay,
        true,
        &mut sequence,
        cancellation,
    )?;
    let mut previous = first;

    for frame_meta in &frames[1..] {
        cancellation.check()?;
        let delay = apng_delay_fraction(frame_meta.delay, &frame_meta.file)?;
        let current =
            decode_and_resize_frame(&mut archive, frame_meta, original_size, output_size)?;
        let region = changed_region(&previous, &current);
        write_apng_frame(
            &mut output,
            &current,
            region,
            delay,
            false,
            &mut sequence,
            cancellation,
        )?;
        previous = current;
    }
    write_png_chunk(&mut output, *b"IEND", &[])?;
    output.flush()?;
    Ok(())
}

fn scaled_dimensions(width: u32, height: u32, max_edge: u32) -> (u32, u32) {
    if width == 0 || height == 0 {
        return (0, 0);
    }
    if max_edge == 0 || width.max(height) <= max_edge {
        return (width, height);
    }
    if width >= height {
        (
            max_edge,
            ((u64::from(height) * u64::from(max_edge)) / u64::from(width)).max(1) as u32,
        )
    } else {
        (
            ((u64::from(width) * u64::from(max_edge)) / u64::from(height)).max(1) as u32,
            max_edge,
        )
    }
}

fn collect_global_histogram(
    zip_path: &Path,
    frames: &[UgoiraFrame],
    max_edge: u32,
    cancellation: &CancellationToken,
) -> Result<FirstPass, EncodeError> {
    let mut archive = open_archive(zip_path)?;
    let mut original_size = None;
    let mut output_size = None;
    let mut histogram = GlobalColorStatistics::new();

    for frame_meta in frames {
        cancellation.check()?;
        let _ = gif_delay_ticks(frame_meta.delay, &frame_meta.file)?;
        let decoded = decode_frame(&mut archive, frame_meta)?;
        let dimensions = decoded.dimensions();
        validate_frame_size(frame_meta, dimensions, original_size)?;
        if dimensions.0 == 0 || dimensions.1 == 0 {
            return Err(EncodeError::EmptyImage);
        }
        if original_size.is_none() {
            original_size = Some(dimensions);
            output_size = Some(scaled_dimensions(dimensions.0, dimensions.1, max_edge));
        }
        let resized = resize_frame(decoded, output_size.ok_or(EncodeError::EmptyImage)?);
        histogram.add_image(&resized)?;
        cancellation.check()?;
    }

    let original_size = original_size.ok_or(EncodeError::EmptyFrames)?;
    let output_size = output_size.ok_or(EncodeError::EmptyFrames)?;
    if output_size.0 > u16::MAX as u32 || output_size.1 > u16::MAX as u32 {
        return Err(EncodeError::FrameTooLarge {
            file: frames[0].file.clone(),
            width: output_size.0,
            height: output_size.1,
        });
    }
    Ok(FirstPass {
        original_size,
        output_size,
        histogram,
    })
}

fn build_global_palette(histogram: &GlobalColorStatistics) -> Result<Vec<Srgb<u8>>, EncodeError> {
    let samples = histogram.weighted_samples();
    if samples.is_empty() {
        return Ok(Vec::new());
    }
    let palette_size = if histogram.has_transparency {
        PaletteSize::from_u8_clamped(255)
    } else {
        PaletteSize::MAX
    };
    let pipeline = Pipeline::new()
        .palette_size(palette_size)
        .quantize_method(QuantizeMethod::kmeans())
        .ditherer(None)
        .dedup(true)
        .parallel(true);
    let palette = pipeline
        .input_slice(&samples)
        .map_err(|err| EncodeError::Quantization(err.to_string()))?
        .output_srgb8_palette();
    Ok(palette.into_vec())
}

fn write_global_palette_gif(
    zip_path: &Path,
    frames: &[UgoiraFrame],
    output_path: &Path,
    first_pass: &FirstPass,
    opaque_palette: &[Srgb<u8>],
    cancellation: &CancellationToken,
) -> Result<(), EncodeError> {
    let mut archive = open_archive(zip_path)?;
    let global_palette = gif_palette_bytes(opaque_palette, first_pass.histogram.has_transparency);
    let mut encoder = Encoder::new(
        File::create(output_path)?,
        first_pass.output_size.0 as u16,
        first_pass.output_size.1 as u16,
        &global_palette,
    )?;
    encoder.set_repeat(Repeat::Infinite)?;

    let quantize_method = if opaque_palette.is_empty() {
        None
    } else {
        let palette = PaletteBuf::try_from(opaque_palette.to_vec())
            .map_err(|err| EncodeError::Quantization(err.to_string()))?;
        Some(QuantizeMethod::from(palette))
    };

    for frame_meta in frames {
        cancellation.check()?;
        let decoded = decode_frame(&mut archive, frame_meta)?;
        validate_frame_size(
            frame_meta,
            decoded.dimensions(),
            Some(first_pass.original_size),
        )?;
        let resized = resize_frame(decoded, first_pass.output_size);
        let indices = quantize_frame(
            &resized,
            quantize_method.clone(),
            first_pass.histogram.has_transparency,
        )?;
        cancellation.check()?;
        let frame = Frame {
            width: first_pass.output_size.0 as u16,
            height: first_pass.output_size.1 as u16,
            delay: gif_delay_ticks(frame_meta.delay, &frame_meta.file)?,
            dispose: DisposalMethod::Background,
            transparent: first_pass.histogram.has_transparency.then_some(0),
            buffer: Cow::Owned(indices),
            ..Frame::default()
        };
        cancellation.check()?;
        encoder.write_frame(&frame)?;
        cancellation.check()?;
    }
    Ok(())
}

fn quantize_frame(
    image: &RgbaImage,
    quantize_method: Option<QuantizeMethod>,
    has_transparency: bool,
) -> Result<Vec<u8>, EncodeError> {
    let mut rgb = Vec::with_capacity(image.width() as usize * image.height() as usize * 3);
    for pixel in image.pixels() {
        rgb.extend_from_slice(&pixel.0[..3]);
    }
    let rgb = RgbImage::from_raw(image.width(), image.height(), rgb)
        .ok_or_else(|| EncodeError::Quantization("invalid RGB frame dimensions".to_string()))?;

    let mut indices = if let Some(method) = quantize_method {
        let image_ref =
            ImageRef::try_from(&rgb).map_err(|err| EncodeError::Quantization(err.to_string()))?;
        Pipeline::new()
            .quantize_method(method)
            .ditherer(
                FloydSteinberg::with_error_diffusion(FLOYD_STEINBERG_ERROR_DIFFUSION).ok_or_else(
                    || {
                        EncodeError::Quantization(
                            "Floyd-Steinberg diffusion factor is out of range".to_string(),
                        )
                    },
                )?,
            )
            .dedup(false)
            .parallel(true)
            .input_image(image_ref)
            .output_srgb8_indexed_image()
            .into_parts()
            .1
    } else {
        vec![0; image.width() as usize * image.height() as usize]
    };

    if has_transparency {
        for (index, pixel) in indices.iter_mut().zip(image.pixels()) {
            if pixel.0[3] == 0 {
                *index = 0;
            } else {
                *index = index.checked_add(1).ok_or_else(|| {
                    EncodeError::Quantization("palette index overflow".to_string())
                })?;
            }
        }
    }
    Ok(indices)
}

fn open_archive(path: &Path) -> Result<ZipArchive<File>, EncodeError> {
    Ok(ZipArchive::new(File::open(path)?)?)
}

fn decode_frame(
    archive: &mut ZipArchive<File>,
    frame: &UgoiraFrame,
) -> Result<RgbaImage, EncodeError> {
    let mut zip_frame = archive.by_name(&frame.file)?;
    let mut compressed = Vec::new();
    zip_frame.read_to_end(&mut compressed)?;
    Ok(ImageReader::new(Cursor::new(compressed))
        .with_guessed_format()?
        .decode()?
        .into_rgba8())
}

fn validate_frame_size(
    frame: &UgoiraFrame,
    dimensions: (u32, u32),
    expected: Option<(u32, u32)>,
) -> Result<(), EncodeError> {
    if let Some((expected_width, expected_height)) = expected {
        if dimensions != (expected_width, expected_height) {
            return Err(EncodeError::FrameSizeMismatch {
                file: frame.file.clone(),
                width: dimensions.0,
                height: dimensions.1,
                expected_width,
                expected_height,
            });
        }
    }
    Ok(())
}

fn resize_frame(image: RgbaImage, output_size: (u32, u32)) -> RgbaImage {
    if image.dimensions() == output_size {
        image
    } else {
        // imageops::resize 在计算期间必须同时读取源图并写入目标图；传入值由本函数独占，
        // 返回目标图前源图即被释放，因此原尺寸图不会延续到 bbox 或 PNG 压缩阶段。
        image::imageops::resize(&image, output_size.0, output_size.1, FilterType::Lanczos3)
    }
}

fn decode_and_resize_frame(
    archive: &mut ZipArchive<File>,
    frame: &UgoiraFrame,
    original_size: (u32, u32),
    output_size: (u32, u32),
) -> Result<RgbaImage, EncodeError> {
    let decoded = decode_frame(archive, frame)?;
    validate_frame_size(frame, decoded.dimensions(), Some(original_size))?;
    // decoded 被 move 进 resize_frame；helper 返回时只剩输出尺寸的 RGBA image。
    Ok(resize_frame(decoded, output_size))
}

fn changed_region(previous: &RgbaImage, current: &RgbaImage) -> FrameRegion {
    debug_assert_eq!(previous.dimensions(), current.dimensions());
    let (canvas_width, canvas_height) = current.dimensions();
    let mut min_x = canvas_width;
    let mut min_y = canvas_height;
    let mut max_x = 0;
    let mut max_y = 0;
    let mut changed = false;

    for y in 0..canvas_height {
        for x in 0..canvas_width {
            if previous.get_pixel(x, y) != current.get_pixel(x, y) {
                changed = true;
                min_x = min_x.min(x);
                min_y = min_y.min(y);
                max_x = max_x.max(x);
                max_y = max_y.max(y);
            }
        }
    }

    if !changed {
        return FrameRegion {
            x: 0,
            y: 0,
            width: 1,
            height: 1,
        };
    }

    FrameRegion {
        x: min_x,
        y: min_y,
        width: max_x - min_x + 1,
        height: max_y - min_y + 1,
    }
}

fn frame_region_rows(image: &RgbaImage, region: FrameRegion) -> impl Iterator<Item = &[u8]> {
    debug_assert!(region.width > 0 && region.height > 0);
    debug_assert!(region.x + region.width <= image.width());
    debug_assert!(region.y + region.height <= image.height());
    let canvas_width = image.width() as usize;
    let row_bytes = region.width as usize * 4;
    let raw = image.as_raw();
    (region.y..region.y + region.height).map(move |y| {
        let start = (y as usize * canvas_width + region.x as usize) * 4;
        &raw[start..start + row_bytes]
    })
}

fn write_apng_header<W: Write>(
    output: &mut W,
    canvas_size: (u32, u32),
    frame_count: u32,
) -> Result<(), EncodeError> {
    output.write_all(PNG_SIGNATURE)?;
    let mut ihdr = [0u8; 13];
    ihdr[0..4].copy_from_slice(&canvas_size.0.to_be_bytes());
    ihdr[4..8].copy_from_slice(&canvas_size.1.to_be_bytes());
    ihdr[8] = 8; // RGBA8 bit depth
    ihdr[9] = 6; // PNG truecolor with alpha
                 // compression/filter/interlace 均为 PNG 规范中的 method 0；APNG 不使用隔行扫描。
    write_png_chunk(output, *b"IHDR", &ihdr)?;

    let mut animation = [0u8; 8];
    animation[0..4].copy_from_slice(&frame_count.to_be_bytes());
    animation[4..8].copy_from_slice(&0u32.to_be_bytes()); // 0 表示无限循环。
    write_png_chunk(output, *b"acTL", &animation)?;
    Ok(())
}

#[allow(clippy::too_many_arguments)]
fn write_apng_frame<W: Write>(
    output: &mut W,
    image: &RgbaImage,
    region: FrameRegion,
    delay: (u16, u16),
    first_frame: bool,
    sequence: &mut u64,
    cancellation: &CancellationToken,
) -> Result<(), EncodeError> {
    cancellation.check()?;
    let mut control = [0u8; 26];
    control[0..4].copy_from_slice(&next_apng_sequence(sequence)?.to_be_bytes());
    control[4..8].copy_from_slice(&region.width.to_be_bytes());
    control[8..12].copy_from_slice(&region.height.to_be_bytes());
    control[12..16].copy_from_slice(&region.x.to_be_bytes());
    control[16..20].copy_from_slice(&region.y.to_be_bytes());
    control[20..22].copy_from_slice(&delay.0.to_be_bytes());
    control[22..24].copy_from_slice(&delay.1.to_be_bytes());
    control[24] = 0; // DisposeOp::None
    control[25] = 0; // BlendOp::Source
    write_png_chunk(output, *b"fcTL", &control)?;

    // png 0.18.1 的单个 APNG StreamWriter 在动态 set_frame_dimension 后不会按 bbox
    // 重建 prev/curr/filtered 行缓冲。这里为每个 bbox 创建普通 PNG stream writer，令其
    // 行缓冲从一开始就是 bbox width；relay 只转发 IDAT 压缩字节，不保存 payload。
    let mut relay =
        PngFrameDataRelay::new(output, sequence, first_frame, (region.width, region.height));
    {
        let mut encoder = PngEncoder::new(&mut relay, region.width, region.height);
        encoder.set_color(ColorType::Rgba);
        encoder.set_depth(BitDepth::Eight);
        encoder.set_compression(Compression::Balanced);
        encoder.set_filter(Filter::Adaptive);
        let mut writer = encoder.write_header()?;
        {
            let mut stream = writer.stream_writer()?;
            for row in frame_region_rows(image, region) {
                cancellation.check()?;
                stream.write_all(row)?;
            }
            cancellation.check()?;
            stream.finish()?;
        }
        writer.finish()?;
    }
    relay.finish()?;
    cancellation.check()?;
    Ok(())
}

fn next_apng_sequence(sequence: &mut u64) -> Result<u32, EncodeError> {
    let current = u32::try_from(*sequence).map_err(|_| EncodeError::ApngSequenceOverflow)?;
    *sequence = sequence
        .checked_add(1)
        .ok_or(EncodeError::ApngSequenceOverflow)?;
    Ok(current)
}

fn next_apng_sequence_io(sequence: &mut u64) -> io::Result<u32> {
    next_apng_sequence(sequence).map_err(|err| io::Error::new(io::ErrorKind::InvalidData, err))
}

fn write_png_chunk<W: Write>(output: &mut W, chunk_type: [u8; 4], data: &[u8]) -> io::Result<()> {
    let length = u32::try_from(data.len())
        .map_err(|_| io::Error::new(io::ErrorKind::InvalidInput, "PNG chunk exceeds u32 length"))?;
    output.write_all(&length.to_be_bytes())?;
    output.write_all(&chunk_type)?;
    output.write_all(data)?;
    let mut crc = Crc32::new();
    crc.update(&chunk_type);
    crc.update(data);
    output.write_all(&crc.finalize().to_be_bytes())?;
    Ok(())
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum RelayPhase {
    Signature,
    Length,
    Type,
    Data,
    Crc,
    Done,
}

struct PngFrameDataRelay<'a, W: Write> {
    output: &'a mut W,
    sequence: &'a mut u64,
    first_frame: bool,
    expected_size: (u32, u32),
    phase: RelayPhase,
    scratch: [u8; 8],
    scratch_len: usize,
    source_chunk_len: u32,
    source_chunk_type: [u8; 4],
    remaining: usize,
    source_crc: Option<Crc32>,
    target_crc: Option<Crc32>,
    ihdr_data: [u8; 13],
    ihdr_data_len: usize,
    chunks_seen: u64,
    saw_ihdr: bool,
    saw_idat: bool,
    idat_ended: bool,
    saw_iend: bool,
}

impl<'a, W: Write> PngFrameDataRelay<'a, W> {
    fn new(
        output: &'a mut W,
        sequence: &'a mut u64,
        first_frame: bool,
        expected_size: (u32, u32),
    ) -> Self {
        Self {
            output,
            sequence,
            first_frame,
            expected_size,
            phase: RelayPhase::Signature,
            scratch: [0; 8],
            scratch_len: 0,
            source_chunk_len: 0,
            source_chunk_type: [0; 4],
            remaining: 0,
            source_crc: None,
            target_crc: None,
            ihdr_data: [0; 13],
            ihdr_data_len: 0,
            chunks_seen: 0,
            saw_ihdr: false,
            saw_idat: false,
            idat_ended: false,
            saw_iend: false,
        }
    }

    fn finish(&mut self) -> io::Result<()> {
        if self.phase != RelayPhase::Done || self.scratch_len != 0 {
            return Err(io::Error::new(
                io::ErrorKind::UnexpectedEof,
                format!(
                    "temporary PNG stream ended before validated IEND (phase {:?})",
                    self.phase
                ),
            ));
        }
        if !self.saw_ihdr || !self.saw_idat || !self.saw_iend {
            return Err(io::Error::new(
                io::ErrorKind::InvalidData,
                "temporary PNG stream is missing IHDR, IDAT, or IEND",
            ));
        }
        Ok(())
    }

    fn begin_source_chunk(&mut self) -> io::Result<()> {
        validate_png_chunk_type(self.source_chunk_type)?;
        if self.chunks_seen == 0 && self.source_chunk_type != *b"IHDR" {
            return Err(invalid_png("IHDR must be the first PNG chunk"));
        }
        match self.source_chunk_type {
            chunk if chunk == *b"IHDR" => {
                if self.saw_ihdr || self.chunks_seen != 0 {
                    return Err(invalid_png("IHDR must appear exactly once and first"));
                }
                if self.source_chunk_len != 13 {
                    return Err(invalid_png("IHDR length must be exactly 13"));
                }
                self.ihdr_data_len = 0;
            }
            chunk if chunk == *b"IDAT" => {
                if !self.saw_ihdr {
                    return Err(invalid_png("IDAT appeared before IHDR"));
                }
                if self.idat_ended {
                    return Err(invalid_png("IDAT chunks must be consecutive"));
                }
            }
            chunk if chunk == *b"IEND" => {
                if !self.saw_ihdr || !self.saw_idat {
                    return Err(invalid_png("IEND requires preceding IHDR and IDAT"));
                }
                if self.saw_iend {
                    return Err(invalid_png("IEND must appear exactly once"));
                }
                if self.source_chunk_len != 0 {
                    return Err(invalid_png("IEND length must be zero"));
                }
                self.idat_ended = true;
            }
            chunk if is_png_critical(chunk) => {
                return Err(invalid_png("unexpected critical PNG chunk"));
            }
            _ => {
                if !self.saw_ihdr {
                    return Err(invalid_png("ancillary chunk appeared before IHDR"));
                }
                // PNG 允许 ancillary chunks 位于 IDAT 前后；一旦出现在 IDAT 后，数据区结束，
                // 后续恢复 IDAT 必须拒绝。配置中的 png Encoder 正常不会产生 ancillary chunks。
                if self.saw_idat {
                    self.idat_ended = true;
                }
            }
        }

        self.remaining = self.source_chunk_len as usize;
        let mut source_crc = Crc32::new();
        source_crc.update(&self.source_chunk_type);
        self.source_crc = Some(source_crc);
        if self.source_chunk_type == *b"IDAT" {
            self.begin_target_data_chunk()?;
        }
        if self.remaining == 0 {
            self.finish_source_chunk_data()?;
            self.phase = RelayPhase::Crc;
        } else {
            self.phase = RelayPhase::Data;
        }
        self.scratch_len = 0;
        Ok(())
    }

    fn consume_source_data(&mut self, data: &[u8]) -> io::Result<()> {
        self.source_crc
            .as_mut()
            .ok_or_else(|| invalid_png("missing source CRC state"))?
            .update(data);
        if self.source_chunk_type == *b"IHDR" {
            let end = self.ihdr_data_len + data.len();
            if end > self.ihdr_data.len() {
                return Err(invalid_png("IHDR payload exceeds 13 bytes"));
            }
            self.ihdr_data[self.ihdr_data_len..end].copy_from_slice(data);
            self.ihdr_data_len = end;
        }
        if self.source_chunk_type == *b"IDAT" {
            self.relay_target_data(data)?;
        }
        Ok(())
    }

    fn finish_source_chunk_data(&mut self) -> io::Result<()> {
        if self.source_chunk_type == *b"IHDR" {
            self.validate_ihdr()?;
        }
        self.finish_target_data_chunk()
    }

    fn validate_ihdr(&self) -> io::Result<()> {
        if self.ihdr_data_len != 13 {
            return Err(invalid_png("IHDR payload is incomplete"));
        }
        let width = u32::from_be_bytes(self.ihdr_data[0..4].try_into().unwrap());
        let height = u32::from_be_bytes(self.ihdr_data[4..8].try_into().unwrap());
        if (width, height) != self.expected_size {
            return Err(invalid_png("IHDR dimensions do not match APNG frame bbox"));
        }
        if self.ihdr_data[8..13] != [8, 6, 0, 0, 0] {
            return Err(invalid_png(
                "IHDR must be RGBA8 with standard compression/filter and no interlace",
            ));
        }
        Ok(())
    }

    fn validate_and_commit_source_crc(&mut self) -> io::Result<()> {
        let expected = u32::from_be_bytes(self.scratch[..4].try_into().unwrap());
        let actual = self
            .source_crc
            .take()
            .ok_or_else(|| invalid_png("missing source CRC state"))?
            .finalize();
        if actual != expected {
            return Err(invalid_png("temporary PNG source CRC mismatch"));
        }

        match self.source_chunk_type {
            chunk if chunk == *b"IHDR" => self.saw_ihdr = true,
            chunk if chunk == *b"IDAT" => self.saw_idat = true,
            chunk if chunk == *b"IEND" => self.saw_iend = true,
            _ => {}
        }
        self.chunks_seen = self
            .chunks_seen
            .checked_add(1)
            .ok_or_else(|| invalid_png("PNG chunk count overflow"))?;
        self.scratch_len = 0;
        self.phase = if self.source_chunk_type == *b"IEND" {
            RelayPhase::Done
        } else {
            RelayPhase::Length
        };
        Ok(())
    }

    fn begin_target_data_chunk(&mut self) -> io::Result<()> {
        let (target_type, target_length) =
            target_data_chunk_header(self.source_chunk_len, self.first_frame)?;
        self.output.write_all(&target_length.to_be_bytes())?;
        self.output.write_all(&target_type)?;
        let mut crc = Crc32::new();
        crc.update(&target_type);
        if !self.first_frame {
            let sequence = next_apng_sequence_io(self.sequence)?.to_be_bytes();
            self.output.write_all(&sequence)?;
            crc.update(&sequence);
        }
        self.target_crc = Some(crc);
        Ok(())
    }

    fn relay_target_data(&mut self, data: &[u8]) -> io::Result<()> {
        if let Some(crc) = &mut self.target_crc {
            self.output.write_all(data)?;
            crc.update(data);
        }
        Ok(())
    }

    fn finish_target_data_chunk(&mut self) -> io::Result<()> {
        if let Some(crc) = self.target_crc.take() {
            self.output.write_all(&crc.finalize().to_be_bytes())?;
        }
        Ok(())
    }

    fn copy_into_scratch(&mut self, input: &mut &[u8], target_len: usize) {
        let count = (target_len - self.scratch_len).min(input.len());
        self.scratch[self.scratch_len..self.scratch_len + count].copy_from_slice(&input[..count]);
        self.scratch_len += count;
        *input = &input[count..];
    }
}

impl<W: Write> Write for PngFrameDataRelay<'_, W> {
    fn write(&mut self, mut input: &[u8]) -> io::Result<usize> {
        let input_len = input.len();
        while !input.is_empty() {
            match self.phase {
                RelayPhase::Signature => {
                    self.copy_into_scratch(&mut input, PNG_SIGNATURE.len());
                    if self.scratch_len == PNG_SIGNATURE.len() {
                        if &self.scratch != PNG_SIGNATURE {
                            return Err(io::Error::new(
                                io::ErrorKind::InvalidData,
                                "temporary PNG has invalid signature",
                            ));
                        }
                        self.scratch_len = 0;
                        self.phase = RelayPhase::Length;
                    }
                }
                RelayPhase::Length => {
                    self.copy_into_scratch(&mut input, 4);
                    if self.scratch_len == 4 {
                        self.source_chunk_len = u32::from_be_bytes(
                            self.scratch[..4]
                                .try_into()
                                .expect("fixed four-byte length"),
                        );
                        self.scratch_len = 0;
                        self.phase = RelayPhase::Type;
                    }
                }
                RelayPhase::Type => {
                    self.copy_into_scratch(&mut input, 4);
                    if self.scratch_len == 4 {
                        self.source_chunk_type.copy_from_slice(&self.scratch[..4]);
                        self.begin_source_chunk()?;
                    }
                }
                RelayPhase::Data => {
                    let count = self.remaining.min(input.len());
                    let data = &input[..count];
                    self.consume_source_data(data)?;
                    self.remaining -= count;
                    input = &input[count..];
                    if self.remaining == 0 {
                        self.finish_source_chunk_data()?;
                        self.phase = RelayPhase::Crc;
                        self.scratch_len = 0;
                    }
                }
                RelayPhase::Crc => {
                    self.copy_into_scratch(&mut input, 4);
                    if self.scratch_len == 4 {
                        self.validate_and_commit_source_crc()?;
                    }
                }
                RelayPhase::Done => {
                    return Err(invalid_png("bytes found after IEND"));
                }
            }
        }
        Ok(input_len)
    }

    fn flush(&mut self) -> io::Result<()> {
        self.output.flush()
    }
}

fn target_data_chunk_header(source_length: u32, first_frame: bool) -> io::Result<([u8; 4], u32)> {
    if first_frame {
        Ok((*b"IDAT", source_length))
    } else {
        Ok((
            *b"fdAT",
            source_length
                .checked_add(4)
                .ok_or_else(|| invalid_png("fdAT chunk length overflow"))?,
        ))
    }
}

fn validate_png_chunk_type(chunk_type: [u8; 4]) -> io::Result<()> {
    if !chunk_type.iter().all(u8::is_ascii_alphabetic) {
        return Err(invalid_png("PNG chunk type must contain ASCII letters"));
    }
    if chunk_type[2].is_ascii_lowercase() {
        return Err(invalid_png("PNG chunk reserved bit must be uppercase"));
    }
    Ok(())
}

fn is_png_critical(chunk_type: [u8; 4]) -> bool {
    chunk_type[0].is_ascii_uppercase()
}

fn invalid_png(message: impl Into<String>) -> io::Error {
    io::Error::new(io::ErrorKind::InvalidData, message.into())
}

fn gif_palette_bytes(palette: &[Srgb<u8>], has_transparency: bool) -> Vec<u8> {
    let mut bytes = Vec::with_capacity((palette.len() + usize::from(has_transparency)) * 3);
    if has_transparency {
        bytes.extend_from_slice(&[0, 0, 0]);
    }
    for color in palette {
        bytes.extend_from_slice(&[color.red, color.green, color.blue]);
    }
    bytes
}

fn gif_delay_ticks(delay_ms: i64, file: &str) -> Result<u16, EncodeError> {
    if delay_ms <= 0 {
        return Err(EncodeError::NonPositiveDelay {
            file: file.to_string(),
            delay_ms,
        });
    }
    // GIF Graphic Control Extension 的 delay 字段以 1/100 秒为单位，gif crate 暴露为 u16。
    // 这里仅按格式字段上限拒绝无法表达的值；正常 Pixiv 毫秒值不做额外截断。
    let ticks = ((delay_ms - 1) / 10) + 1;
    if ticks > u16::MAX as i64 {
        return Err(EncodeError::DelayTooLarge {
            file: file.to_string(),
            delay_ms,
        });
    }
    Ok(ticks as u16)
}

fn apng_delay_fraction(delay_ms: i64, file: &str) -> Result<(u16, u16), EncodeError> {
    if delay_ms <= 0 {
        return Err(EncodeError::NonPositiveDelay {
            file: file.to_string(),
            delay_ms,
        });
    }
    let divisor = greatest_common_divisor(delay_ms, 1_000);
    let numerator = delay_ms / divisor;
    let denominator = 1_000 / divisor;
    if numerator > i64::from(u16::MAX) || denominator > i64::from(u16::MAX) {
        return Err(EncodeError::ApngDelayTooLarge {
            file: file.to_string(),
            delay_ms,
        });
    }
    Ok((numerator as u16, denominator as u16))
}

fn greatest_common_divisor(mut left: i64, mut right: i64) -> i64 {
    while right != 0 {
        let remainder = left % right;
        left = right;
        right = remainder;
    }
    left
}

unsafe fn owned_c_string(ptr: *const c_char, name: &'static str) -> Result<String, EncodeError> {
    if ptr.is_null() {
        return Err(EncodeError::NullPointer(name));
    }
    // Safety: 调用方须满足对应 extern 函数的 C 字符串契约；借用仅存在于本语句，随后立即复制。
    let bytes = unsafe { CStr::from_ptr(ptr) }.to_bytes().to_owned();
    String::from_utf8(bytes).map_err(|_| EncodeError::InvalidUtf8(name))
}

unsafe fn owned_c_path(ptr: *const c_char, name: &'static str) -> Result<PathBuf, EncodeError> {
    // Safety: 与 owned_c_string 相同，返回值不再借用调用方内存。
    Ok(PathBuf::from(unsafe { owned_c_string(ptr, name) }?))
}

fn ffi_result(
    function_name: &'static str,
    operation: impl FnOnce() -> Result<(), EncodeError>,
) -> *mut c_char {
    // catch_unwind 只阻止 Rust panic 穿越 C ABI；它无法捕获或修复无效 raw pointer 导致的 UB。
    match catch_unwind(AssertUnwindSafe(operation)) {
        Ok(Ok(())) => null_mut(),
        Ok(Err(err)) => c_error_string(err.to_string()),
        Err(payload) => c_error_string(format!(
            "panic in {function_name}: {}",
            panic_payload_to_string(payload.as_ref())
        )),
    }
}

#[no_mangle]
pub extern "C" fn ugoira_cancel_token_new() -> *mut CancellationToken {
    catch_unwind(|| Box::into_raw(Box::new(CancellationToken::default()))).unwrap_or(null_mut())
}

#[no_mangle]
/// 将取消状态原子地设为 true。
///
/// # Safety
///
/// `token` 必须由 [`ugoira_cancel_token_new`] 创建、尚未释放，并在本次调用期间保持有效。
/// 本函数可与使用同一 token 的 [`ugoira_encode`] 或其他 cancel 调用并发，但不得与
/// [`ugoira_cancel_token_free`] 并发。
pub unsafe extern "C" fn ugoira_cancel_token_cancel(
    token: *const CancellationToken,
) -> *mut c_char {
    ffi_result("ugoira_cancel_token_cancel", || {
        if token.is_null() {
            return Err(EncodeError::NullPointer("cancellation_token"));
        }
        // Safety: 由本函数的调用契约保证 token 有效；AtomicBool 支持约定内的并发 cancel/encode。
        unsafe { &*token }.cancel();
        Ok(())
    })
}

#[no_mangle]
/// 释放由 [`ugoira_cancel_token_new`] 创建的取消令牌。
///
/// # Safety
///
/// `token` 必须来自 [`ugoira_cancel_token_new`]、尚未释放，且此时没有编码或取消调用继续
/// 访问它。调用返回后该指针立即失效，不得再次使用或释放。
pub unsafe extern "C" fn ugoira_cancel_token_free(token: *mut CancellationToken) -> *mut c_char {
    ffi_result("ugoira_cancel_token_free", || {
        if token.is_null() {
            return Err(EncodeError::NullPointer("cancellation_token"));
        }
        // 指针仅能由 ugoira_cancel_token_new 产生，并且调用方保证只释放一次。
        unsafe {
            drop(Box::from_raw(token));
        }
        Ok(())
    })
}

#[no_mangle]
/// 将 Pixiv ugoira ZIP 编码为指定动画容器。
///
/// # Safety
///
/// `zip_path`、`frames_json`、`output_path` 必须分别指向在本次调用期间可读且以 NUL 结尾的
/// C 字符串；字符串内容必须是 UTF-8。`format` 只能为 0（GIF）或 1（APNG）。`cancellation` 必须由
/// [`ugoira_cancel_token_new`] 创建、尚未释放，并在本次调用返回前保持有效。其他线程可按
/// [`ugoira_cancel_token_cancel`] 的契约并发取消，但不得并发释放 token。输出路径必须允许
/// 当前进程创建或截断文件。
pub unsafe extern "C" fn ugoira_encode(
    zip_path: *const c_char,
    frames_json: *const c_char,
    output_path: *const c_char,
    cancellation: *const CancellationToken,
    format: u32,
    max_edge: u32,
) -> *mut c_char {
    ffi_result("ugoira_encode", || {
        // Safety: 本 extern 函数的 Safety contract 覆盖所有传入 raw pointer。
        unsafe {
            encode_ffi(
                zip_path,
                frames_json,
                output_path,
                cancellation,
                format,
                max_edge,
            )
        }
    })
}

unsafe fn encode_ffi(
    zip_path: *const c_char,
    frames_json: *const c_char,
    output_path: *const c_char,
    cancellation: *const CancellationToken,
    format: u32,
    max_edge: u32,
) -> Result<(), EncodeError> {
    // Safety: encode_ffi 只由 ugoira_encode 在其 Safety contract 下调用。
    let zip_path = unsafe { owned_c_path(zip_path, "zip_path") }?;
    // Safety: 同上，复制后不再借用调用方字符串。
    let frames_json = unsafe { owned_c_string(frames_json, "frames_json") }?;
    // Safety: 同上，复制后不再借用调用方字符串。
    let output_path = unsafe { owned_c_path(output_path, "output_path") }?;
    if cancellation.is_null() {
        return Err(EncodeError::NullPointer("cancellation_token"));
    }
    // Safety: ugoira_encode 的契约保证 token 在同步编码返回前有效且不会并发释放。
    let cancellation = unsafe { &*cancellation };
    let frames = parse_frames_json(&frames_json)?;
    match AnimationFormat::try_from(format)? {
        AnimationFormat::Gif => {
            encode_gif(&zip_path, &frames, &output_path, max_edge, cancellation)
        }
        AnimationFormat::Apng => {
            encode_apng(&zip_path, &frames, &output_path, max_edge, cancellation)
        }
    }
}

fn c_error_string(message: String) -> *mut c_char {
    let mut bytes = message.into_bytes();
    for byte in &mut bytes {
        if *byte == 0 {
            *byte = b' ';
        }
    }
    bytes.push(0);
    // bytes 总是由当前函数补上 NUL 结尾，且上方已清理内部 NUL。
    unsafe { CString::from_vec_with_nul_unchecked(bytes) }.into_raw()
}

fn panic_payload_to_string(payload: &(dyn std::any::Any + Send)) -> String {
    if let Some(message) = payload.downcast_ref::<&str>() {
        return (*message).to_string();
    }
    if let Some(message) = payload.downcast_ref::<String>() {
        return message.clone();
    }
    "unknown panic payload".to_string()
}

#[no_mangle]
/// 释放本 crate 其他 FFI 函数返回的错误字符串。
///
/// # Safety
///
/// 非空 `err` 必须是本 crate FFI 返回且尚未释放的错误字符串指针；调用后该指针失效。
pub unsafe extern "C" fn ugoira_free_error(err: *mut c_char) {
    let _ = catch_unwind(AssertUnwindSafe(|| {
        if !err.is_null() {
            // 错误字符串由 Rust FFI 分配；调用方读完后必须交回同一分配器释放。
            unsafe {
                drop(CString::from_raw(err));
            }
        }
    }));
}

#[cfg(test)]
mod tests {
    use super::*;

    use image::codecs::jpeg::JpegEncoder;
    use image::{ImageBuffer, Rgba};
    use png::{BlendOp, DisposeOp};
    use std::io::{BufReader, Write};
    use tempfile::tempdir;
    use zip::write::SimpleFileOptions;

    type TestRgbaImage = ImageBuffer<Rgba<u8>, Vec<u8>>;

    #[test]
    fn parses_frames_json() {
        let frames = parse_frames_json(r#"[{"file":"000000.jpg","delay":80}]"#).unwrap();
        assert_eq!(frames.len(), 1);
        assert_eq!(frames[0].file, "000000.jpg");
        assert_eq!(frames[0].delay, 80);
    }

    #[test]
    fn parses_delay_larger_than_u16_milliseconds() {
        let frames = parse_frames_json(r#"[{"file":"000000.jpg","delay":70000}]"#).unwrap();
        assert_eq!(frames[0].delay, 70_000);
        assert_eq!(
            gif_delay_ticks(frames[0].delay, &frames[0].file).unwrap(),
            7_000
        );
    }

    #[test]
    fn rejects_delay_that_cannot_fit_gif_delay_field() {
        let err = gif_delay_ticks(655_351, "000000.jpg").unwrap_err();
        assert!(err.to_string().contains("exceeds GIF delay field limit"));
    }

    #[test]
    fn formats_panic_payload_for_ffi_error() {
        let payload = std::panic::catch_unwind(|| panic!("ffi boom")).unwrap_err();
        assert_eq!(panic_payload_to_string(payload.as_ref()), "ffi boom");
    }

    #[test]
    fn ffi_c_string_is_copied_into_owned_memory() {
        let owned = {
            let input = CString::new("ugoira.zip").unwrap();
            unsafe { owned_c_string(input.as_ptr(), "zip_path") }.unwrap()
        };
        assert_eq!(owned, "ugoira.zip");
    }

    #[test]
    fn ffi_rejects_null_cancellation_token() {
        let dir = tempdir().unwrap();
        let zip_path =
            CString::new(dir.path().join("ugoira.zip").to_string_lossy().as_bytes()).unwrap();
        let frames = CString::new("[]").unwrap();
        let output = CString::new(dir.path().join("out.gif").to_string_lossy().as_bytes()).unwrap();

        let error = unsafe {
            ugoira_encode(
                zip_path.as_ptr(),
                frames.as_ptr(),
                output.as_ptr(),
                std::ptr::null(),
                0,
                0,
            )
        };
        assert!(!error.is_null());
        let message = unsafe { CStr::from_ptr(error) }
            .to_string_lossy()
            .into_owned();
        unsafe { ugoira_free_error(error) };
        assert!(message.contains("cancellation_token") && message.contains("null"));
    }

    #[test]
    fn ffi_cancellation_token_cancels_encoding_and_can_be_freed() {
        let dir = tempdir().unwrap();
        let zip_file = dir.path().join("ugoira.zip");
        let output_file = dir.path().join("out.gif");
        write_rgba_zip(&zip_file, "000000.png", 2, 2, Rgba([255, 0, 0, 255]));
        let zip_path = CString::new(zip_file.to_string_lossy().as_bytes()).unwrap();
        let frames = CString::new(r#"[{"file":"000000.png","delay":80}]"#).unwrap();
        let output = CString::new(output_file.to_string_lossy().as_bytes()).unwrap();
        let token = ugoira_cancel_token_new();
        assert!(!token.is_null());
        assert!(unsafe { ugoira_cancel_token_cancel(token) }.is_null());

        let error = unsafe {
            ugoira_encode(
                zip_path.as_ptr(),
                frames.as_ptr(),
                output.as_ptr(),
                token,
                0,
                0,
            )
        };
        assert!(!error.is_null());
        let message = unsafe { CStr::from_ptr(error) }
            .to_string_lossy()
            .into_owned();
        unsafe { ugoira_free_error(error) };
        assert!(message.contains("canceled"));
        assert!(!output_file.exists());
        assert!(unsafe { ugoira_cancel_token_free(token) }.is_null());
    }

    #[test]
    fn ffi_cancel_and_free_reject_null_tokens() {
        let cancel_error = unsafe { ugoira_cancel_token_cancel(std::ptr::null()) };
        assert!(!cancel_error.is_null());
        let cancel_message = unsafe { CStr::from_ptr(cancel_error) }
            .to_string_lossy()
            .into_owned();
        assert!(cancel_message.contains("cancellation_token pointer is null"));
        unsafe { ugoira_free_error(cancel_error) };

        let free_error = unsafe { ugoira_cancel_token_free(std::ptr::null_mut()) };
        assert!(!free_error.is_null());
        let free_message = unsafe { CStr::from_ptr(free_error) }
            .to_string_lossy()
            .into_owned();
        assert!(free_message.contains("cancellation_token pointer is null"));
        unsafe { ugoira_free_error(free_error) };
    }

    #[test]
    fn global_color_statistics_have_a_fixed_sample_cap() {
        let mut statistics = GlobalColorStatistics::new();
        statistics.samples = vec![Srgb::new(0, 0, 0); MAX_GLOBAL_COLOR_SAMPLES];
        statistics.opaque_pixels = MAX_GLOBAL_COLOR_SAMPLES as u64;
        let next = ImageBuffer::from_pixel(1, 1, Rgba([255, 128, 64, 255]));

        statistics.add_image(&next).unwrap();

        assert_eq!(statistics.samples.len(), MAX_GLOBAL_COLOR_SAMPLES);
        assert_eq!(
            statistics.weighted_samples().len(),
            MAX_GLOBAL_COLOR_SAMPLES
        );
        assert_eq!(
            statistics.opaque_pixels,
            MAX_GLOBAL_COLOR_SAMPLES as u64 + 1
        );
    }

    #[test]
    fn rejects_invalid_frames_json() {
        let err = parse_frames_json("not-json").unwrap_err();
        assert!(err.to_string().contains("frames json is invalid"));
    }

    #[test]
    fn encodes_zip_frames_to_gif() {
        let dir = tempdir().unwrap();
        let zip_path = dir.path().join("ugoira.zip");
        let out_path = dir.path().join("out.gif");
        write_zip(
            &zip_path,
            &[("000000.jpg", [255, 0, 0]), ("000001.jpg", [0, 255, 0])],
        );

        let frames = vec![
            UgoiraFrame {
                file: "000000.jpg".to_string(),
                delay: 80,
            },
            UgoiraFrame {
                file: "000001.jpg".to_string(),
                delay: 60,
            },
        ];
        encode_gif(
            &zip_path,
            &frames,
            &out_path,
            0,
            &CancellationToken::default(),
        )
        .unwrap();

        let body = std::fs::read(out_path).unwrap();
        assert!(body.starts_with(b"GIF"));
    }

    #[test]
    fn encodes_zip_frames_to_apng_with_infinite_loop() {
        let dir = tempdir().unwrap();
        let zip_path = dir.path().join("ugoira.zip");
        let out_path = dir.path().join("out.apng");
        write_images_zip(
            &zip_path,
            &[
                (
                    "000000.png",
                    ImageBuffer::from_pixel(2, 2, Rgba([255, 0, 0, 255])),
                ),
                (
                    "000001.png",
                    ImageBuffer::from_pixel(2, 2, Rgba([0, 255, 0, 255])),
                ),
            ],
        );
        let frames = vec![
            UgoiraFrame {
                file: "000000.png".to_string(),
                delay: 80,
            },
            UgoiraFrame {
                file: "000001.png".to_string(),
                delay: 60,
            },
        ];

        encode_apng(
            &zip_path,
            &frames,
            &out_path,
            0,
            &CancellationToken::default(),
        )
        .unwrap();

        let body = std::fs::read(out_path).unwrap();
        assert!(body.starts_with(b"\x89PNG\r\n\x1a\n"));
        let animation = png_chunk_data(&body, b"acTL").unwrap();
        assert_eq!(u32::from_be_bytes(animation[0..4].try_into().unwrap()), 2);
        assert_eq!(u32::from_be_bytes(animation[4..8].try_into().unwrap()), 0);
        let controls = png_chunks_data(&body, b"fcTL");
        assert_eq!(
            u16::from_be_bytes(controls[0][20..22].try_into().unwrap()),
            2
        );
        assert_eq!(
            u16::from_be_bytes(controls[0][22..24].try_into().unwrap()),
            25
        );
        assert_eq!(
            u16::from_be_bytes(controls[1][20..22].try_into().unwrap()),
            3
        );
        assert_eq!(
            u16::from_be_bytes(controls[1][22..24].try_into().unwrap()),
            50
        );

        let records = png_chunk_records(&body);
        let chunk_types: Vec<_> = records.iter().map(|record| record.0).collect();
        assert_eq!(&chunk_types[..3], &[*b"IHDR", *b"acTL", *b"fcTL"]);
        assert_eq!(chunk_types.last(), Some(b"IEND"));
        let second_control = chunk_types
            .iter()
            .enumerate()
            .filter(|(_, chunk_type)| **chunk_type == *b"fcTL")
            .nth(1)
            .map(|(index, _)| index)
            .unwrap();
        assert!(chunk_types[3..second_control]
            .iter()
            .all(|chunk_type| *chunk_type == *b"IDAT"));
        assert!(chunk_types[second_control + 1..chunk_types.len() - 1]
            .iter()
            .all(|chunk_type| *chunk_type == *b"fdAT"));
        let sequences: Vec<_> = records
            .iter()
            .filter(|record| record.0 == *b"fcTL" || record.0 == *b"fdAT")
            .map(|record| u32::from_be_bytes(record.1[..4].try_into().unwrap()))
            .collect();
        assert_eq!(sequences, (0..sequences.len() as u32).collect::<Vec<_>>());
        assert_png_chunk_crcs(&body);
    }

    #[test]
    fn apng_writes_smallest_changed_rectangle() {
        let dir = tempdir().unwrap();
        let zip_path = dir.path().join("ugoira.zip");
        let out_path = dir.path().join("out.apng");
        let first = ImageBuffer::from_pixel(4, 3, Rgba([255, 0, 0, 255]));
        let mut second = first.clone();
        second.put_pixel(2, 1, Rgba([0, 255, 0, 255]));
        write_images_zip(&zip_path, &[("000000.png", first), ("000001.png", second)]);
        let frames = vec![
            UgoiraFrame {
                file: "000000.png".to_string(),
                delay: 80,
            },
            UgoiraFrame {
                file: "000001.png".to_string(),
                delay: 125,
            },
        ];

        encode_apng(
            &zip_path,
            &frames,
            &out_path,
            0,
            &CancellationToken::default(),
        )
        .unwrap();

        let body = std::fs::read(out_path).unwrap();
        let controls = png_chunks_data(&body, b"fcTL");
        assert_eq!(controls.len(), 2);
        let second = controls[1];
        assert_eq!(u32::from_be_bytes(second[4..8].try_into().unwrap()), 1);
        assert_eq!(u32::from_be_bytes(second[8..12].try_into().unwrap()), 1);
        assert_eq!(u32::from_be_bytes(second[12..16].try_into().unwrap()), 2);
        assert_eq!(u32::from_be_bytes(second[16..20].try_into().unwrap()), 1);
        assert_eq!(u16::from_be_bytes(second[20..22].try_into().unwrap()), 1);
        assert_eq!(u16::from_be_bytes(second[22..24].try_into().unwrap()), 8);
        assert_eq!((second[24], second[25]), (0, 0));
    }

    #[test]
    fn full_changed_region_borrows_current_frame_pixels() {
        let previous = ImageBuffer::from_pixel(4, 3, Rgba([0, 0, 0, 255]));
        let current = ImageBuffer::from_pixel(4, 3, Rgba([255, 255, 255, 255]));

        let region = changed_region(&previous, &current);
        let raw = current.as_raw();
        for (row_index, row) in frame_region_rows(&current, region).enumerate() {
            assert_eq!(row.as_ptr(), raw[row_index * 4 * 4..].as_ptr());
            assert_eq!(row.len(), 4 * 4);
        }
    }

    #[test]
    fn partial_multiline_region_uses_borrowed_rows() {
        let previous = ImageBuffer::from_pixel(4, 4, Rgba([0, 0, 0, 255]));
        let mut current = previous.clone();
        current.put_pixel(1, 1, Rgba([255, 0, 0, 255]));
        current.put_pixel(2, 2, Rgba([0, 255, 0, 255]));

        let region = changed_region(&previous, &current);
        assert_eq!(
            (region.x, region.y, region.width, region.height),
            (1, 1, 2, 2)
        );
        let raw = current.as_raw();
        for (row_index, row) in frame_region_rows(&current, region).enumerate() {
            let expected_start = ((row_index + 1) * 4 + 1) * 4;
            assert_eq!(row.as_ptr(), raw[expected_start..].as_ptr());
            assert_eq!(row.len(), 2 * 4);
        }
    }

    #[test]
    fn near_full_region_iterates_borrowed_rows_without_area_buffer() {
        let previous = ImageBuffer::from_pixel(64, 64, Rgba([0, 0, 0, 255]));
        let mut current = previous.clone();
        for y in 0..64 {
            for x in 0..63 {
                current.put_pixel(x, y, Rgba([255, 255, 255, 255]));
            }
        }

        let region = changed_region(&previous, &current);
        assert_eq!(
            (region.x, region.y, region.width, region.height),
            (0, 0, 63, 64)
        );
        let raw = current.as_raw();
        let mut row_count = 0;
        for (row_index, row) in frame_region_rows(&current, region).enumerate() {
            let expected_start = row_index * current.width() as usize * 4;
            assert_eq!(row.as_ptr(), raw[expected_start..].as_ptr());
            assert_eq!(row.len(), 63 * 4);
            row_count += 1;
        }
        assert_eq!(row_count, 64);
    }

    #[test]
    fn idat_relay_handles_fragmented_writes_multiple_chunks_crc_and_sequence() {
        let temporary_png = temporary_png_with_idats(1, 1, &[b"first-payload", b"second-payload"]);

        let mut first_output = PNG_SIGNATURE.to_vec();
        let mut first_sequence = 1u64; // 首帧 fcTL 已消费 sequence 0。
        {
            let mut relay =
                PngFrameDataRelay::new(&mut first_output, &mut first_sequence, true, (1, 1));
            write_fragmented(&mut relay, &temporary_png);
            relay.finish().unwrap();
        }
        let first_chunks = png_chunk_records(&first_output);
        assert_eq!(first_chunks.len(), 2);
        assert_eq!(first_chunks[0].0, *b"IDAT");
        assert_eq!(first_chunks[0].1, b"first-payload");
        assert_eq!(first_chunks[1].0, *b"IDAT");
        assert_eq!(first_chunks[1].1, b"second-payload");
        assert_eq!(first_sequence, 1);
        assert_png_chunk_crcs(&first_output);

        let mut later_output = PNG_SIGNATURE.to_vec();
        let mut later_sequence = 2u64; // 第二帧 fcTL 已消费 sequence 1。
        {
            let mut relay =
                PngFrameDataRelay::new(&mut later_output, &mut later_sequence, false, (1, 1));
            write_fragmented(&mut relay, &temporary_png);
            relay.finish().unwrap();
        }
        let later_chunks = png_chunk_records(&later_output);
        assert_eq!(later_chunks.len(), 2);
        assert_eq!(later_chunks[0].0, *b"fdAT");
        assert_eq!(&later_chunks[0].1[..4], &2u32.to_be_bytes());
        assert_eq!(&later_chunks[0].1[4..], b"first-payload");
        assert_eq!(later_chunks[1].0, *b"fdAT");
        assert_eq!(&later_chunks[1].1[..4], &3u32.to_be_bytes());
        assert_eq!(&later_chunks[1].1[4..], b"second-payload");
        assert_eq!(later_sequence, 4);
        assert_png_chunk_crcs(&later_output);
    }

    #[test]
    fn idat_relay_rejects_bad_source_crc() {
        let mut temporary_png = temporary_png_with_idats(2, 1, &[b"payload"]);
        let idat_type = temporary_png
            .windows(4)
            .position(|window| window == b"IDAT")
            .unwrap();
        let payload_len =
            u32::from_be_bytes(temporary_png[idat_type - 4..idat_type].try_into().unwrap())
                as usize;
        temporary_png[idat_type + 4 + payload_len] ^= 0xff;

        let mut output = Vec::new();
        let mut sequence = 1;
        let mut relay = PngFrameDataRelay::new(&mut output, &mut sequence, true, (2, 1));
        let mut failure = None;
        for byte in temporary_png.chunks(1) {
            if let Err(err) = relay.write_all(byte) {
                failure = Some(err);
                break;
            }
        }
        let error = failure.or_else(|| relay.finish().err()).unwrap();
        assert!(error.to_string().contains("CRC"), "{error}");
    }

    #[test]
    fn idat_relay_rejects_invalid_png_structure() {
        let ihdr = temporary_ihdr(2, 1);
        let valid = temporary_png_with_idats(2, 1, &[b"payload"]);
        let missing_iend = valid[..valid.len() - 12].to_vec();
        let missing_ihdr = temporary_png_from_chunks(&[(*b"IDAT", b"payload"), (*b"IEND", b"")]);
        let duplicate_ihdr = temporary_png_from_chunks(&[
            (*b"IHDR", &ihdr),
            (*b"IHDR", &ihdr),
            (*b"IDAT", b"payload"),
            (*b"IEND", b""),
        ]);
        let non_contiguous_idat = temporary_png_from_chunks(&[
            (*b"IHDR", &ihdr),
            (*b"IDAT", b"first"),
            (*b"tEXt", b"allowed ancillary"),
            (*b"IDAT", b"second"),
            (*b"IEND", b""),
        ]);
        let missing_idat = temporary_png_from_chunks(&[(*b"IHDR", &ihdr), (*b"IEND", b"")]);
        let unexpected_critical = temporary_png_from_chunks(&[
            (*b"IHDR", &ihdr),
            (*b"PLTE", b""),
            (*b"IDAT", b"payload"),
            (*b"IEND", b""),
        ]);
        let non_empty_iend = temporary_png_from_chunks(&[
            (*b"IHDR", &ihdr),
            (*b"IDAT", b"payload"),
            (*b"IEND", b"x"),
        ]);
        let mut bytes_after_iend = valid.clone();
        bytes_after_iend.push(0);
        let mut duplicate_iend = valid;
        write_png_chunk(&mut duplicate_iend, *b"IEND", &[]).unwrap();

        for (name, body, expected) in [
            ("missing IEND", missing_iend, "validated IEND"),
            ("IDAT before IHDR", missing_ihdr, "IHDR must be the first"),
            ("duplicate IHDR", duplicate_ihdr, "exactly once"),
            (
                "non-contiguous IDAT",
                non_contiguous_idat,
                "must be consecutive",
            ),
            (
                "missing IDAT",
                missing_idat,
                "requires preceding IHDR and IDAT",
            ),
            (
                "unexpected critical",
                unexpected_critical,
                "unexpected critical",
            ),
            ("non-empty IEND", non_empty_iend, "length must be zero"),
            ("bytes after IEND", bytes_after_iend, "after IEND"),
            ("duplicate IEND", duplicate_iend, "after IEND"),
        ] {
            let error = relay_error(&body, (2, 1));
            assert!(
                error.to_string().contains(expected),
                "{name}: {error}; expected {expected:?}"
            );
        }
    }

    #[test]
    fn idat_relay_validates_ihdr_length_dimensions_and_rgba8_parameters() {
        let valid = temporary_ihdr(2, 1);
        let mut wrong_dimensions = valid;
        wrong_dimensions[0..4].copy_from_slice(&3u32.to_be_bytes());
        let mut wrong_depth = valid;
        wrong_depth[8] = 16;
        let mut wrong_color = valid;
        wrong_color[9] = 2;
        let mut wrong_compression = valid;
        wrong_compression[10] = 1;
        let mut wrong_filter = valid;
        wrong_filter[11] = 1;
        let mut interlaced = valid;
        interlaced[12] = 1;

        for (name, ihdr, expected) in [
            ("short", valid[..12].to_vec(), "length must be exactly 13"),
            (
                "dimensions",
                wrong_dimensions.to_vec(),
                "dimensions do not match",
            ),
            ("depth", wrong_depth.to_vec(), "must be RGBA8"),
            ("color", wrong_color.to_vec(), "must be RGBA8"),
            ("compression", wrong_compression.to_vec(), "must be RGBA8"),
            ("filter", wrong_filter.to_vec(), "must be RGBA8"),
            ("interlace", interlaced.to_vec(), "must be RGBA8"),
        ] {
            let body = temporary_png_from_chunks(&[
                (*b"IHDR", ihdr.as_slice()),
                (*b"IDAT", b"payload"),
                (*b"IEND", b""),
            ]);
            let error = relay_error(&body, (2, 1));
            assert!(
                error.to_string().contains(expected),
                "{name}: {error}; expected {expected:?}"
            );
        }
    }

    #[test]
    fn idat_relay_finish_rejects_every_truncated_parser_phase() {
        let body = temporary_png_with_idats(2, 1, &[b"payload"]);
        let ihdr_crc_start = PNG_SIGNATURE.len() + 4 + 4 + 13;
        for (name, truncated) in [
            ("signature", &body[..4]),
            ("chunk length", &body[..PNG_SIGNATURE.len() + 2]),
            ("chunk type", &body[..PNG_SIGNATURE.len() + 4 + 2]),
            ("chunk payload", &body[..PNG_SIGNATURE.len() + 4 + 4 + 5]),
            ("chunk CRC", &body[..ihdr_crc_start + 2]),
        ] {
            let error = relay_error(truncated, (2, 1));
            assert_eq!(
                error.kind(),
                io::ErrorKind::UnexpectedEof,
                "{name}: {error}"
            );
        }
    }

    #[test]
    fn idat_relay_rejects_invalid_signature() {
        let mut body = temporary_png_with_idats(2, 1, &[b"payload"]);
        body[0] = 0;
        let error = relay_error(&body, (2, 1));
        assert!(error.to_string().contains("invalid signature"), "{error}");
    }

    #[test]
    fn apng_sequence_reports_overflow_after_last_representable_value() {
        let mut sequence = u32::MAX as u64;

        assert_eq!(next_apng_sequence(&mut sequence).unwrap(), u32::MAX);
        assert_eq!(sequence, u32::MAX as u64 + 1);
        assert!(matches!(
            next_apng_sequence(&mut sequence),
            Err(EncodeError::ApngSequenceOverflow)
        ));
    }

    #[test]
    fn fdat_target_chunk_length_reports_overflow() {
        let error = target_data_chunk_header(u32::MAX, false).unwrap_err();

        assert_eq!(error.kind(), io::ErrorKind::InvalidData);
        assert!(error.to_string().contains("fdAT chunk length overflow"));
    }

    #[test]
    fn apng_writes_identical_frame_as_one_pixel_region() {
        let dir = tempdir().unwrap();
        let zip_path = dir.path().join("ugoira.zip");
        let out_path = dir.path().join("out.apng");
        let image = ImageBuffer::from_pixel(3, 2, Rgba([10, 20, 30, 40]));
        write_images_zip(
            &zip_path,
            &[("000000.png", image.clone()), ("000001.png", image)],
        );
        let frames = vec![
            UgoiraFrame {
                file: "000000.png".to_string(),
                delay: 80,
            },
            UgoiraFrame {
                file: "000001.png".to_string(),
                delay: 60,
            },
        ];

        encode_apng(
            &zip_path,
            &frames,
            &out_path,
            0,
            &CancellationToken::default(),
        )
        .unwrap();

        let body = std::fs::read(out_path).unwrap();
        let controls = png_chunks_data(&body, b"fcTL");
        assert_eq!(controls.len(), 2);
        assert_eq!(u32::from_be_bytes(controls[1][4..8].try_into().unwrap()), 1);
        assert_eq!(
            u32::from_be_bytes(controls[1][8..12].try_into().unwrap()),
            1
        );
        assert_eq!(
            u16::from_be_bytes(controls[1][20..22].try_into().unwrap()),
            3
        );
        assert_eq!(
            u16::from_be_bytes(controls[1][22..24].try_into().unwrap()),
            50
        );
    }

    #[test]
    fn apng_delta_frames_replay_to_original_rgba_images() {
        let dir = tempdir().unwrap();
        let zip_path = dir.path().join("ugoira.zip");
        let out_path = dir.path().join("out.apng");
        let first = ImageBuffer::from_fn(3, 2, |x, y| {
            Rgba([(x * 40) as u8, (y * 80) as u8, 120, (x * 70 + 40) as u8])
        });
        let mut second = first.clone();
        second.put_pixel(1, 0, Rgba([200, 10, 20, 0]));
        second.put_pixel(2, 1, Rgba([5, 220, 30, 255]));
        write_images_zip(
            &zip_path,
            &[
                ("000000.png", first.clone()),
                ("000001.png", second.clone()),
            ],
        );
        let frames = vec![
            UgoiraFrame {
                file: "000000.png".to_string(),
                delay: 80,
            },
            UgoiraFrame {
                file: "000001.png".to_string(),
                delay: 60,
            },
        ];

        encode_apng(
            &zip_path,
            &frames,
            &out_path,
            0,
            &CancellationToken::default(),
        )
        .unwrap();

        let decoded = decode_apng_rgba_frames(&out_path);
        assert_eq!(decoded, vec![first.into_raw(), second.into_raw()]);
    }

    #[test]
    fn apng_resize_preserves_aspect_ratio_and_never_upscales() {
        for (name, width, height, max_edge, expected) in [
            ("downscale", 8, 4, 4, (4, 2)),
            ("no-upscale", 3, 2, 540, (3, 2)),
        ] {
            let dir = tempdir().unwrap();
            let zip_path = dir.path().join(format!("{name}.zip"));
            let out_path = dir.path().join(format!("{name}.apng"));
            write_rgba_zip(
                &zip_path,
                "000000.png",
                width,
                height,
                Rgba([255, 0, 0, 255]),
            );
            let frames = vec![UgoiraFrame {
                file: "000000.png".to_string(),
                delay: 80,
            }];

            encode_apng(
                &zip_path,
                &frames,
                &out_path,
                max_edge,
                &CancellationToken::default(),
            )
            .unwrap();

            let decoder = png::Decoder::new(BufReader::new(File::open(out_path).unwrap()));
            let reader = decoder.read_info().unwrap();
            assert_eq!(
                (reader.info().width, reader.info().height),
                expected,
                "{name}"
            );
        }
    }

    #[test]
    fn apng_rejects_delays_that_do_not_fit_exact_u16_fraction() {
        let err = apng_delay_fraction(65_536_000, "000000.png").unwrap_err();
        assert!(err
            .to_string()
            .contains("cannot be represented exactly by APNG u16 delay fields"));
        assert_eq!(apng_delay_fraction(125, "000000.png").unwrap(), (1, 8));
    }

    #[test]
    fn apng_rejects_empty_frames_corrupt_zip_and_missing_entries() {
        let dir = tempdir().unwrap();
        let corrupt_zip = dir.path().join("corrupt.zip");
        let valid_zip = dir.path().join("valid.zip");
        let out_path = dir.path().join("out.apng");
        std::fs::write(&corrupt_zip, b"not a zip").unwrap();
        write_rgba_zip(&valid_zip, "000000.png", 2, 2, Rgba([255, 0, 0, 255]));
        let frame = UgoiraFrame {
            file: "000000.png".to_string(),
            delay: 80,
        };

        let empty =
            encode_apng(&valid_zip, &[], &out_path, 0, &CancellationToken::default()).unwrap_err();
        assert!(matches!(empty, EncodeError::EmptyFrames));

        let corrupt = encode_apng(
            &corrupt_zip,
            std::slice::from_ref(&frame),
            &out_path,
            0,
            &CancellationToken::default(),
        )
        .unwrap_err();
        assert!(corrupt.to_string().contains("zip error"));

        let missing = encode_apng(
            &valid_zip,
            &[UgoiraFrame {
                file: "missing.png".to_string(),
                delay: 80,
            }],
            &out_path,
            0,
            &CancellationToken::default(),
        )
        .unwrap_err();
        assert!(missing.to_string().contains("specified file not found"));
    }

    #[test]
    fn apng_rejects_mismatched_frame_dimensions() {
        let dir = tempdir().unwrap();
        let zip_path = dir.path().join("ugoira.zip");
        let out_path = dir.path().join("out.apng");
        write_images_zip(
            &zip_path,
            &[
                (
                    "000000.png",
                    ImageBuffer::from_pixel(4, 2, Rgba([255, 0, 0, 255])),
                ),
                (
                    "000001.png",
                    ImageBuffer::from_pixel(3, 2, Rgba([0, 255, 0, 255])),
                ),
            ],
        );
        let frames = vec![
            UgoiraFrame {
                file: "000000.png".to_string(),
                delay: 80,
            },
            UgoiraFrame {
                file: "000001.png".to_string(),
                delay: 80,
            },
        ];

        let err = encode_apng(
            &zip_path,
            &frames,
            &out_path,
            0,
            &CancellationToken::default(),
        )
        .unwrap_err();
        assert!(err.to_string().contains("expected 4x2"));
    }

    #[test]
    fn canceled_apng_stops_before_creating_output() {
        let dir = tempdir().unwrap();
        let zip_path = dir.path().join("ugoira.zip");
        let out_path = dir.path().join("out.apng");
        write_rgba_zip(&zip_path, "000000.png", 2, 2, Rgba([255, 0, 0, 255]));
        let cancellation = CancellationToken::default();
        cancellation.cancel();

        let err = encode_apng(
            &zip_path,
            &[UgoiraFrame {
                file: "000000.png".to_string(),
                delay: 80,
            }],
            &out_path,
            0,
            &cancellation,
        )
        .unwrap_err();
        assert!(matches!(err, EncodeError::Canceled));
        assert!(!out_path.exists());
    }

    #[test]
    fn scales_to_max_edge_while_preserving_aspect_ratio() {
        let dir = tempdir().unwrap();
        let zip_path = dir.path().join("ugoira.zip");
        let out_path = dir.path().join("out.gif");
        write_rgba_zip(&zip_path, "000000.png", 8, 4, Rgba([255, 0, 0, 255]));
        let frames = vec![UgoiraFrame {
            file: "000000.png".to_string(),
            delay: 80,
        }];

        encode_gif(
            &zip_path,
            &frames,
            &out_path,
            4,
            &CancellationToken::default(),
        )
        .unwrap();

        let decoder = gif::DecodeOptions::new()
            .read_info(File::open(out_path).unwrap())
            .unwrap();
        assert_eq!((decoder.width(), decoder.height()), (4, 2));
    }

    #[test]
    fn does_not_upscale_frames_below_max_edge() {
        let dir = tempdir().unwrap();
        let zip_path = dir.path().join("ugoira.zip");
        let out_path = dir.path().join("out.gif");
        write_rgba_zip(&zip_path, "000000.png", 3, 2, Rgba([255, 0, 0, 255]));
        let frames = vec![UgoiraFrame {
            file: "000000.png".to_string(),
            delay: 80,
        }];

        encode_gif(
            &zip_path,
            &frames,
            &out_path,
            540,
            &CancellationToken::default(),
        )
        .unwrap();

        let decoder = gif::DecodeOptions::new()
            .read_info(File::open(out_path).unwrap())
            .unwrap();
        assert_eq!((decoder.width(), decoder.height()), (3, 2));
    }

    #[test]
    fn writes_one_global_palette_without_local_frame_palettes() {
        let dir = tempdir().unwrap();
        let zip_path = dir.path().join("ugoira.zip");
        let out_path = dir.path().join("out.gif");
        write_zip(
            &zip_path,
            &[("000000.jpg", [255, 0, 0]), ("000001.jpg", [0, 255, 0])],
        );
        let frames = vec![
            UgoiraFrame {
                file: "000000.jpg".to_string(),
                delay: 80,
            },
            UgoiraFrame {
                file: "000001.jpg".to_string(),
                delay: 60,
            },
        ];

        encode_gif(
            &zip_path,
            &frames,
            &out_path,
            0,
            &CancellationToken::default(),
        )
        .unwrap();

        let mut decoder = gif::DecodeOptions::new()
            .read_info(File::open(out_path).unwrap())
            .unwrap();
        assert!(decoder.global_palette().is_some());
        while let Some(frame) = decoder.read_next_frame().unwrap() {
            assert!(
                frame.palette.is_none(),
                "frame unexpectedly has a local palette"
            );
        }
    }

    #[test]
    fn preserves_frame_order_delays_and_infinite_loop() {
        let dir = tempdir().unwrap();
        let zip_path = dir.path().join("ugoira.zip");
        let out_path = dir.path().join("out.gif");
        write_zip(
            &zip_path,
            &[("000000.jpg", [255, 0, 0]), ("000001.jpg", [0, 255, 0])],
        );
        let frames = vec![
            UgoiraFrame {
                file: "000000.jpg".to_string(),
                delay: 71,
            },
            UgoiraFrame {
                file: "000001.jpg".to_string(),
                delay: 60,
            },
        ];
        encode_gif(
            &zip_path,
            &frames,
            &out_path,
            0,
            &CancellationToken::default(),
        )
        .unwrap();

        let mut options = gif::DecodeOptions::new();
        options.set_color_output(gif::ColorOutput::RGBA);
        let mut decoder = options.read_info(File::open(out_path).unwrap()).unwrap();
        assert_eq!(decoder.repeat(), Repeat::Infinite);
        let first = decoder.read_next_frame().unwrap().unwrap().clone();
        let second = decoder.read_next_frame().unwrap().unwrap().clone();
        assert_eq!((first.delay, second.delay), (8, 6));
        assert!(first.buffer[0] > first.buffer[1]);
        assert!(second.buffer[1] > second.buffer[0]);
        assert!(decoder.read_next_frame().unwrap().is_none());
    }

    #[test]
    fn reserves_transparency_only_for_fully_transparent_pixels() {
        let dir = tempdir().unwrap();
        let zip_path = dir.path().join("ugoira.zip");
        let out_path = dir.path().join("out.gif");
        let image = ImageBuffer::from_fn(2, 1, |x, _| {
            if x == 0 {
                Rgba([10, 20, 30, 0])
            } else {
                Rgba([200, 100, 50, 1])
            }
        });
        write_image_zip(&zip_path, "000000.png", image);
        let frames = vec![UgoiraFrame {
            file: "000000.png".to_string(),
            delay: 80,
        }];
        encode_gif(
            &zip_path,
            &frames,
            &out_path,
            0,
            &CancellationToken::default(),
        )
        .unwrap();

        let mut options = gif::DecodeOptions::new();
        options.set_color_output(gif::ColorOutput::RGBA);
        let mut decoder = options.read_info(File::open(out_path).unwrap()).unwrap();
        let frame = decoder.read_next_frame().unwrap().unwrap();
        assert_eq!(frame.buffer[3], 0);
        assert_eq!(frame.buffer[7], 255);
    }

    #[test]
    fn rejects_frames_with_mismatched_original_dimensions() {
        let dir = tempdir().unwrap();
        let zip_path = dir.path().join("ugoira.zip");
        let out_path = dir.path().join("out.gif");
        write_images_zip(
            &zip_path,
            &[
                (
                    "000000.png",
                    ImageBuffer::from_pixel(4, 2, Rgba([255, 0, 0, 255])),
                ),
                (
                    "000001.png",
                    ImageBuffer::from_pixel(3, 2, Rgba([0, 255, 0, 255])),
                ),
            ],
        );
        let frames = vec![
            UgoiraFrame {
                file: "000000.png".to_string(),
                delay: 80,
            },
            UgoiraFrame {
                file: "000001.png".to_string(),
                delay: 80,
            },
        ];

        let err = encode_gif(
            &zip_path,
            &frames,
            &out_path,
            0,
            &CancellationToken::default(),
        )
        .unwrap_err();
        assert!(err.to_string().contains("expected 4x2"));
    }

    #[test]
    fn rejects_frame_metadata_that_names_a_missing_zip_entry() {
        let dir = tempdir().unwrap();
        let zip_path = dir.path().join("ugoira.zip");
        let out_path = dir.path().join("out.gif");
        write_rgba_zip(&zip_path, "000000.png", 2, 2, Rgba([255, 0, 0, 255]));
        let frames = vec![UgoiraFrame {
            file: "missing.png".to_string(),
            delay: 80,
        }];

        let err = encode_gif(
            &zip_path,
            &frames,
            &out_path,
            0,
            &CancellationToken::default(),
        )
        .unwrap_err();
        assert!(err.to_string().contains("specified file not found"));
    }

    #[test]
    fn rejects_corrupt_zip_input() {
        let dir = tempdir().unwrap();
        let zip_path = dir.path().join("ugoira.zip");
        let out_path = dir.path().join("out.gif");
        std::fs::write(&zip_path, b"not a zip").unwrap();
        let frames = vec![UgoiraFrame {
            file: "000000.png".to_string(),
            delay: 80,
        }];

        let err = encode_gif(
            &zip_path,
            &frames,
            &out_path,
            0,
            &CancellationToken::default(),
        )
        .unwrap_err();
        assert!(err.to_string().contains("zip error"));
    }

    #[test]
    fn canceled_encoding_stops_before_creating_output() {
        let dir = tempdir().unwrap();
        let zip_path = dir.path().join("ugoira.zip");
        let out_path = dir.path().join("out.gif");
        write_rgba_zip(&zip_path, "000000.png", 2, 2, Rgba([255, 0, 0, 255]));
        let frames = vec![UgoiraFrame {
            file: "000000.png".to_string(),
            delay: 80,
        }];
        let cancellation = CancellationToken::default();
        cancellation.cancel();

        let err = encode_gif(&zip_path, &frames, &out_path, 0, &cancellation).unwrap_err();
        assert!(matches!(err, EncodeError::Canceled));
        assert!(!out_path.exists());
    }

    fn write_zip(path: &Path, frames: &[(&str, [u8; 3])]) {
        let file = File::create(path).unwrap();
        let mut zip = zip::ZipWriter::new(file);
        for (name, rgb) in frames {
            zip.start_file(*name, SimpleFileOptions::default()).unwrap();
            zip.write_all(&jpeg_bytes(*rgb)).unwrap();
        }
        zip.finish().unwrap();
    }

    fn png_chunk_data<'a>(body: &'a [u8], wanted: &[u8; 4]) -> Option<&'a [u8]> {
        png_chunks_data(body, wanted).into_iter().next()
    }

    fn png_chunks_data<'a>(body: &'a [u8], wanted: &[u8; 4]) -> Vec<&'a [u8]> {
        let mut found = Vec::new();
        let mut offset = 8usize;
        while offset.checked_add(12).is_some_and(|end| end <= body.len()) {
            let Ok(length_bytes) = body[offset..offset + 4].try_into() else {
                break;
            };
            let length = u32::from_be_bytes(length_bytes) as usize;
            let Some(data_start) = offset.checked_add(8) else {
                break;
            };
            let Some(data_end) = data_start.checked_add(length) else {
                break;
            };
            let Some(chunk_end) = data_end.checked_add(4) else {
                break;
            };
            if chunk_end > body.len() {
                break;
            }
            if &body[offset + 4..offset + 8] == wanted {
                found.push(&body[data_start..data_end]);
            }
            offset = chunk_end;
        }
        found
    }

    fn write_fragmented(writer: &mut impl Write, body: &[u8]) {
        let fragment_sizes = [1usize, 2, 3, 5, 8, 13];
        let mut offset = 0;
        let mut index = 0;
        while offset < body.len() {
            let end = (offset + fragment_sizes[index % fragment_sizes.len()]).min(body.len());
            writer.write_all(&body[offset..end]).unwrap();
            offset = end;
            index += 1;
        }
    }

    fn temporary_png_with_idats(width: u32, height: u32, idats: &[&[u8]]) -> Vec<u8> {
        let ihdr = temporary_ihdr(width, height);
        let mut chunks = Vec::with_capacity(idats.len() + 2);
        chunks.push((*b"IHDR", ihdr.as_slice()));
        for idat in idats {
            chunks.push((*b"IDAT", *idat));
        }
        chunks.push((*b"IEND", &[]));
        temporary_png_from_chunks(&chunks)
    }

    fn temporary_ihdr(width: u32, height: u32) -> [u8; 13] {
        let mut ihdr = [0u8; 13];
        ihdr[0..4].copy_from_slice(&width.to_be_bytes());
        ihdr[4..8].copy_from_slice(&height.to_be_bytes());
        ihdr[8] = 8;
        ihdr[9] = 6;
        ihdr
    }

    fn temporary_png_from_chunks(chunks: &[([u8; 4], &[u8])]) -> Vec<u8> {
        let mut body = PNG_SIGNATURE.to_vec();
        for (chunk_type, data) in chunks {
            write_png_chunk(&mut body, *chunk_type, data).unwrap();
        }
        body
    }

    fn relay_error(body: &[u8], expected_size: (u32, u32)) -> io::Error {
        let mut output = Vec::new();
        let mut sequence = 1;
        let mut relay = PngFrameDataRelay::new(&mut output, &mut sequence, true, expected_size);
        let fragment_sizes = [1usize, 2, 3, 5, 8];
        let mut offset = 0;
        let mut fragment = 0;
        while offset < body.len() {
            let end = (offset + fragment_sizes[fragment % fragment_sizes.len()]).min(body.len());
            if let Err(err) = relay.write_all(&body[offset..end]) {
                return err;
            }
            offset = end;
            fragment += 1;
        }
        relay.finish().unwrap_err()
    }

    fn png_chunk_records(body: &[u8]) -> Vec<([u8; 4], &[u8], u32)> {
        assert!(body.starts_with(PNG_SIGNATURE));
        let mut records = Vec::new();
        let mut offset = PNG_SIGNATURE.len();
        while offset < body.len() {
            assert!(offset + 12 <= body.len());
            let length = u32::from_be_bytes(body[offset..offset + 4].try_into().unwrap()) as usize;
            let chunk_type = body[offset + 4..offset + 8].try_into().unwrap();
            let data_start = offset + 8;
            let data_end = data_start + length;
            let chunk_end = data_end + 4;
            assert!(chunk_end <= body.len());
            let crc = u32::from_be_bytes(body[data_end..chunk_end].try_into().unwrap());
            records.push((chunk_type, &body[data_start..data_end], crc));
            offset = chunk_end;
        }
        records
    }

    fn assert_png_chunk_crcs(body: &[u8]) {
        for (chunk_type, data, actual) in png_chunk_records(body) {
            let mut crc = Crc32::new();
            crc.update(&chunk_type);
            crc.update(data);
            assert_eq!(actual, crc.finalize(), "chunk {:?}", chunk_type);
        }
    }

    fn decode_apng_rgba_frames(path: &Path) -> Vec<Vec<u8>> {
        let decoder = png::Decoder::new(BufReader::new(File::open(path).unwrap()));
        let mut reader = decoder.read_info().unwrap();
        let animation = reader.info().animation_control.unwrap();
        let canvas_width = reader.info().width;
        let canvas_height = reader.info().height;
        let mut canvas = vec![0; canvas_width as usize * canvas_height as usize * 4];
        let mut buffer = vec![0; reader.output_buffer_size().unwrap()];
        let mut frames = Vec::with_capacity(animation.num_frames as usize);

        for _ in 0..animation.num_frames {
            let output = reader.next_frame(&mut buffer).unwrap();
            assert_eq!(
                (output.color_type, output.bit_depth),
                (ColorType::Rgba, BitDepth::Eight)
            );
            let control = reader.info().frame_control.unwrap();
            assert_eq!(
                (control.blend_op, control.dispose_op),
                (BlendOp::Source, DisposeOp::None)
            );
            let row_bytes = output.width as usize * 4;
            for row in 0..output.height as usize {
                let source_start = row * row_bytes;
                let target_start = ((control.y_offset as usize + row) * canvas_width as usize
                    + control.x_offset as usize)
                    * 4;
                canvas[target_start..target_start + row_bytes]
                    .copy_from_slice(&buffer[source_start..source_start + row_bytes]);
            }
            frames.push(canvas.clone());
        }
        frames
    }

    fn jpeg_bytes(rgb: [u8; 3]) -> Vec<u8> {
        let image = ImageBuffer::from_pixel(2, 2, Rgba([rgb[0], rgb[1], rgb[2], 255]));
        let mut bytes = Vec::new();
        let mut encoder = JpegEncoder::new(&mut bytes);
        encoder.encode_image(&image).unwrap();
        bytes
    }

    fn write_rgba_zip(path: &Path, name: &str, width: u32, height: u32, color: Rgba<u8>) {
        write_image_zip(path, name, ImageBuffer::from_pixel(width, height, color));
    }

    fn write_image_zip(path: &Path, name: &str, image: TestRgbaImage) {
        write_images_zip(path, &[(name, image)]);
    }

    fn write_images_zip(path: &Path, images: &[(&str, TestRgbaImage)]) {
        let file = File::create(path).unwrap();
        let mut zip = zip::ZipWriter::new(file);
        for (name, image) in images {
            zip.start_file(*name, SimpleFileOptions::default()).unwrap();
            let mut bytes = Cursor::new(Vec::new());
            image::DynamicImage::ImageRgba8(image.clone())
                .write_to(&mut bytes, image::ImageFormat::Png)
                .unwrap();
            zip.write_all(bytes.get_ref()).unwrap();
        }
        zip.finish().unwrap();
    }
}
