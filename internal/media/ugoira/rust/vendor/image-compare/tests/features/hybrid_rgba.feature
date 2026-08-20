Feature: RGBA image comparison using hybrid mode - MSSIM for for Y, Alpha channels, RMS for U and V

  Scenario: Comparing an image to the original with hybrid mode and checking the difference image
    Given the images 'tests/data/colored_primitives_swapped_alpha.png' and 'tests/data/colored_primitives_alpha.png' are loaded
    When comparing the images using the hybrid mode as rgba
    Then the rgba similarity image matches 'tests/data/colored_primitives_hybrid_compare_rgba.png'

  Scenario: Comparing two images where one is transparent and one is not
    Given the images 'tests/data/100/hand_white.png' and 'tests/data/100/typed_alpha.png' are loaded
    When comparing the images using the hybrid mode as rgba
    Then the rgba similarity image matches 'tests/data/100/diff_100_hand_alpha.png'
    And the similarity score is 0.006358610363005692

  Scenario: Comparing two images where one is transparent in front of black background
    Given the images 'tests/data/100/hand_white.png' and 'tests/data/100/typed_alpha.png' are loaded
    When comparing the images using the blended hybrid mode with 'black' background
    Then the similarity score is 0.007447309291571148

  Scenario: Comparing two images where one is transparent in front of white background
    Given the images 'tests/data/100/hand_white.png' and 'tests/data/100/typed_alpha.png' are loaded
    When comparing the images using the blended hybrid mode with 'white' background
    Then the similarity score is 0.6303176177525529




  Scenario: Comparing two images where both are transparent and similar
    Given the images 'tests/data/100/typed_alpha.png' and 'tests/data/100/typed_color_changed.png' are loaded
    When comparing the images using the hybrid mode as rgba
    Then the rgba similarity image matches 'tests/data/100/diff_100_typed_colors.png'
    And the similarity score is 0.9748343863750435

  Scenario Outline: Comparing a modified image to the original using hybrid mode algorithm
    Given the images 'tests/data/pad_gaprao.png' and '<compare_image>' are loaded
    When comparing the images using the hybrid mode as rgba
    Then the similarity score is <result>

    Examples:
      | compare_image                           | result                |
      | tests/data/pad_gaprao.png               | 1.0                   |
      | tests/data/pad_gaprao_lighter.png       | 0.9514066504143178    |
      | tests/data/pad_gaprao_noise.png         | 0.13009783371684705   |
      | tests/data/pad_gaprao_gray_inverted.png | 3.2566565189821024e-5 |
      | tests/data/pad_gaprao_color_filters.png | 0.9876923700357477    |
      | tests/data/pad_gaprao_alpha.png         | 0.9540830736098154    |
