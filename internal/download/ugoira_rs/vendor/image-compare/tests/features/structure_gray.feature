Feature: Gray image comparison using MSSIM and RMS

  Scenario Outline: Comparing a modified image to the original using RMS algorithm
    Given the images 'tests/data/pad_gaprao.png' and '<compare_image>' are loaded
    When comparing the images using RMS as grayscale
    Then the similarity score is <result>

    Examples:
      | compare_image                           | result              |
      | tests/data/pad_gaprao.png               | 1.0                 |
      | tests/data/pad_gaprao_lighter.png       | 0.9201704590012584  |
      | tests/data/pad_gaprao_noise.png         | 0.7512383697679271  |
      | tests/data/pad_gaprao_gray_inverted.png | 0.497502556580533   |
      | tests/data/pad_gaprao_color_filters.png | 0.9745525957218039  |

  Scenario Outline: Comparing a modified image to the original using MSSIM algorithm
    Given the images 'tests/data/pad_gaprao.png' and '<compare_image>' are loaded
    When comparing the images using MSSIM as grayscale
    Then the similarity score is <result>

    Examples:
      | compare_image                           | result              |
      | tests/data/pad_gaprao.png               | 1.0                 |
      | tests/data/pad_gaprao_lighter.png       | 0.9465500206208791  |
      | tests/data/pad_gaprao_noise.png         | 0.1260665609278695  |
      | tests/data/pad_gaprao_gray_inverted.png | -0.6559340036804088 |
      | tests/data/pad_gaprao_color_filters.png | 0.9885149408030369  |

    Scenario: Comparing an image to the original with RMS and checking the difference image
      Given the images 'tests/data/pad_gaprao.png' and 'tests/data/pad_gaprao_broken.png' are loaded
      When comparing the images using RMS as grayscale
      Then the similarity image matches 'tests/data/pad_graparo_broken_rms_compare.png'

    Scenario: Comparing an image to the original with MSSIM and checking the difference image
      Given the images 'tests/data/pad_gaprao.png' and 'tests/data/pad_gaprao_broken.png' are loaded
      When comparing the images using MSSIM as grayscale
      Then the similarity image matches 'tests/data/pad_graparo_broken_ssim_compare.png'
