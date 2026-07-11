# image-compare

[![Crates.io](https://img.shields.io/crates/d/image-compare?style=flat)](https://crates.io/crates/image-compare)
[![Documentation](https://docs.rs/image-compare/badge.svg)](https://docs.rs/image-compare)
![CI](https://github.com/ChrisRega/image-compare/actions/workflows/rust.yml/badge.svg?branch=main "CI")
[![Coverage Status](https://coveralls.io/repos/github/ChrisRega/image-compare/badge.svg?branch=main)](https://coveralls.io/github/ChrisRega/image-compare?branch=main)
[![License](https://img.shields.io/badge/license-MIT-blue?style=flat)](LICENSE)

Image comparison in rust based on the image crate

- Everything is implemented in plain CPU with rayon multithreading and seems to perform just fine on modern processors.
  Neither [memory optimizations](https://actix.vdop.org/view_post?post_num=10)
  nor [SIMD](https://actix.vdop.org/view_post?post_num=8) seemed to provide any remarkable improvement.

## Comparing grayscale images

### By structure

- By RMS - score is calculated by: $1-\sqrt{\frac{\sum_{x,y=0}^{x,y=w,h}\left(f(x,y)-g(x,y)\right)^2}{w*h}}$
- By MSSIM
    - SSIM is implemented as described on [wikipedia](https://en.wikipedia.org/wiki/Structural_similarity):
      $\mathrm{SSIM}(x,y)={\frac{(2\mu_{x}\mu_{y}+c_{1})(2\sigma_{xy}+c_{2})}{(\mu_{x}^{2}+\mu_{y}^{2}+c_{1})(\sigma_
      {x}^{2}+\sigma_{y}^{2}+c_{2})}}$
    - MSSIM is calculated by using 8x8 pixel windows for SSIM and averaging over the results

### By histogram

- Several distance metrics implemented
  see [OpenCV docs](https://docs.opencv.org/4.5.5/d8/dc8/tutorial_histogram_comparison.html):
- Correlation $d(H_1,H_2) = \frac{\sum_I (H_1(I) - \bar{H_1}) (H_2(I) - \bar{H_2})}{\sqrt{\sum_I(H_1(I) - \bar{H_1})^2
  \sum_I(H_2(I) - \bar{H_2})^2}}$
- Chi-Square $d(H_1,H_2) = \sum _I \frac{\left(H_1(I)-H_2(I)\right)^2}{H_1(I)}$
- Intersection $d(H_1,H_2) = \sum _I \min (H_1(I), H_2(I))$
- Hellinger distance $d(H_1,H_2) = \sqrt{1 - \frac{1}{\sqrt{\int{H_1} \int{H_2}}} \sum_I \sqrt{H_1(I) \cdot H_2(I)}}$

## Comparing RGB(A)

### By structure: RMS, SSIM

- RGB type images are split to R,G and B channels and processed separately.
- The worst of the color results is propagated as score but a float-typed RGB image provides access to all values.
- As you can see in the gherkin tests this result is not worth it currently, as it takes a lot more time
- It could be improved, by not just propagating the individual color-score results but using the worst for each pixel
- This approach is implemented in hybrid-mode, see below

### By structure: "Hybrid Comparison"

- Splitting the image to YUV colorspace according to T.871
- Processing the Y channel with MSSIM
- Comparing U and V channels via RMS
- Recombining the differences to a nice visualization image
- RGB Score is calculated as: $\mathrm{score}=\mathrm{avg}_{x,y}\left(
  \mathrm{min}\left[\Delta \mathrm{MSSIM}(Y,x,y),\sqrt{(\Delta RMS(U,x,y))^2 + (\Delta RMS(V,x,y))^2}\right]\right)$
- RGBA can either be premultiplied with a specifiable background color using `rgba_blended_hybrid_compare`
- Otherwise, for `rgba_hybrid_compare` the $\alpha$ channel is also compared using MSSIM and taken into account.
- The average alpha of each pixel $\bar{\alpha}(x,y) = 1/2 (\alpha_1(x,y) + \alpha_2(x,y))$ is then used as a linear
  weighting factor
- RGBA Score is calculated as: $\mathrm{score}=\mathrm{avg}_{x,y}\left(1/\bar{\alpha} \cdot
  \mathrm{min}\left[\Delta \mathrm{MSSIM}(Y,x,y),\sqrt{(\Delta RMS(U,x,y))^2 + (\Delta RMS(V,x,y))^2}, \Delta \mathrm{RMS}(\alpha,x,y)\right]
  \right)$
- Edge cases RGBA: $\mathrm{score} \in (0, 1)$ and $\mathrm{score} = 1.0$ if $\bar{\alpha} = 0.0$
- This allows for a good separation of color differences and structure differences for both RGB and RGBA
- Interpretation of the diff-images:
    - RGB: Red contains structure differences, Green and Blue the color differences, the more color, the higher the diff
    - RGBA: Same as RGB but alpha contains the inverse of the alpha-diffs. If something is heavily translucent, the
      alpha was so different, that differentiating between color and structure difference would be difficult. Also,
      minimum alpha is clamped at 0.1, so you can still see all changes.

Changelog:
0.5.0:
- Added support for custom image types via config

0.4.2:
- Updated dependencies 

0.4.1:
- Support for hybrid-compare with images larger than 4096x4096 (Thanks @nikarh)

0.4.0:

- Support for image-rs series 0.25

0.3.1:

- Remove additional dependencies from image-rs, making this library more lean (Thanks @ericseppanen)

0.3.0:

- An error was found in hybrid RGB compare in 0.2.x that over-weighted color differences. Numbers in tests were adjusted
    - Influence was very small for most images but noticeable for the color-filtered one which yields much higher
      similarity now
- Added two methods for RGBA comparison
- Added GitHub inline latex for equations instead of embedded images - fixes dark theme rendering
- Made API more intuitive
