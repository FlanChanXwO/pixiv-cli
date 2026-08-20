Feature: RGB image comparison using RMS and MSSIM

  Scenario Outline: Comparing a modified image to the original using RMS algorithm
    Given the images 'tests/data/pad_gaprao.png' and '<compare_image>' are loaded
    When comparing the images using RMS as rgb
    Then the similarity score is <result>

    Examples:
      | compare_image                           | result              |
      | tests/data/pad_gaprao.png               | 1.0                 |
      | tests/data/pad_gaprao_lighter.png       | 0.9179765266232506  |
      | tests/data/pad_gaprao_noise.png         | 0.7440815983539952  |
      | tests/data/pad_gaprao_gray_inverted.png | 0.49263422652304256 |
      | tests/data/pad_gaprao_color_filters.png | 0.9268171842610413  |

  Scenario Outline: Comparing a modified image to the original using MSSIM algorithm
    Given the images 'tests/data/pad_gaprao.png' and '<compare_image>' are loaded
    When comparing the images using MSSIM as rgb
    Then the similarity score is <result>

    Examples:
      | compare_image                           | result              |
      | tests/data/pad_gaprao.png               | 1.0                 |
      | tests/data/pad_gaprao_lighter.png       | 0.9081331834121142  |
      | tests/data/pad_gaprao_noise.png         | 0.1240573695772849  |
      | tests/data/pad_gaprao_gray_inverted.png | -0.6520514023141206 |
      | tests/data/pad_gaprao_color_filters.png | 0.9481089021470664  |

  Scenario: Comparing an image to the original with RMS and checking the difference image
    Given the images 'tests/data/pad_gaprao.png' and 'tests/data/pad_gaprao_broken.png' are loaded
    When comparing the images using RMS as rgb
    Then the rgb similarity image matches 'tests/data/pad_graparo_broken_rms_compare_rgb.png'

  Scenario: Comparing an image to the original with RMS and checking the difference image
    Given the images 'tests/data/pad_gaprao.png' and 'tests/data/pad_gaprao_color_filters.png' are loaded
    When comparing the images using RMS as rgb
    Then the rgb similarity image matches 'tests/data/pad_gaprao_color_filters_rms_compare_rgb.png'

  Scenario: Comparing an image to the original with MSSIM and checking the difference image
    Given the images 'tests/data/pad_gaprao.png' and 'tests/data/pad_gaprao_broken.png' are loaded
    When comparing the images using MSSIM as rgb
    Then the rgb similarity image matches 'tests/data/pad_graparo_broken_ssim_compare_rgb.png'

  Scenario: Comparing an image to the original with MSSIM and checking the difference image
    Given the images 'tests/data/pad_gaprao.png' and 'tests/data/pad_gaprao_color_filters.png' are loaded
    When comparing the images using MSSIM as rgb
    Then the rgb similarity image matches 'tests/data/pad_gaprao_color_filters_ssim_compare_rgb.png'

