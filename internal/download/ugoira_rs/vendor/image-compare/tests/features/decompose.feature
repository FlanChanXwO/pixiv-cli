Feature: Decompose trait test

Scenario Outline: Compare newly split yuv channels to expected values
  Given the image '<full_image>' is loaded
  When comparing yuv channels
  Then check split success

Examples:
| full_image                                           |
| tests/data/pad_gaprao.png                            |
| tests/data/pad_gaprao_lighter.png                    |
| tests/data/pad_gaprao_noise.png                      |
| tests/data/pad_gaprao_gray_inverted.png              |
| tests/data/pad_gaprao_color_filters.png              |
| tests/data/colored_primitives_swapped.png            |
| tests/data/colored_primitives_hybrid_compare_rgb.png |
| tests/data/colored_primitives.png                    |



Scenario Outline: Compare newly split yuv channels to expected values
  Given the image '<full_image>' is loaded
  When comparing rgb channels
  Then check split success

Examples:
| full_image                                           |
| tests/data/pad_gaprao.png                            |
| tests/data/pad_gaprao_lighter.png                    |
| tests/data/pad_gaprao_noise.png                      |
| tests/data/pad_gaprao_gray_inverted.png              |
| tests/data/pad_gaprao_color_filters.png              |
| tests/data/colored_primitives_swapped.png            |
| tests/data/colored_primitives_hybrid_compare_rgb.png |
| tests/data/colored_primitives.png                    |