Feature: Gray image comparison using histogram similarity with different metrics

  Scenario Outline: Comparing a modified image to the original using histogram correlation
    Given the images 'tests/data/pad_gaprao.png' and '<compare_image>' are loaded
    When comparing the images using histogram 'correlation' as grayscale
    Then the similarity score is <result>

    Examples:
      | compare_image                           | result              |
      | tests/data/pad_gaprao.png               | 1.0                 |
      | tests/data/pad_gaprao_lighter.png       | 0.4166225335699562  |
      | tests/data/pad_gaprao_noise.png         | -0.10678452274495025|
      | tests/data/pad_gaprao_gray_inverted.png | 0.2529181079771206  |


  Scenario Outline: Comparing a modified image to the original using histogram chisquare
    Given the images 'tests/data/pad_gaprao.png' and '<compare_image>' are loaded
    When comparing the images using histogram 'chisquare' as grayscale
    Then the similarity score is <result>

    Examples:
      | compare_image                           | result              |
      | tests/data/pad_gaprao.png               | 0.0                 |

  Scenario Outline: Comparing a modified image to the original using histogram intersection
    Given the images 'tests/data/pad_gaprao.png' and '<compare_image>' are loaded
    When comparing the images using histogram 'intersection' as grayscale
    Then the similarity score is <result>

    Examples:
      | compare_image                           | result   |
      | tests/data/pad_gaprao.png               | 480000.0 |
      | tests/data/pad_gaprao_lighter.png       | 391363.0 |
      | tests/data/pad_gaprao_noise.png         | 312255.0 |
      | tests/data/pad_gaprao_gray_inverted.png | 364095.0 |

  Scenario Outline: Comparing a modified image to the original using hellinger distance of histograms
    Given the images 'tests/data/pad_gaprao.png' and '<compare_image>' are loaded
    When comparing the images using histogram 'hellinger distance' as grayscale
    Then the similarity score is <result>

    Examples:
      | compare_image                           | result              |
      | tests/data/pad_gaprao.png               | 0.0                 |
      | tests/data/pad_gaprao_lighter.png       | 0.21725162902677742 |
      | tests/data/pad_gaprao_noise.png         | 0.41048980725794537 |
      | tests/data/pad_gaprao_gray_inverted.png | 0.22138675253275514 |
