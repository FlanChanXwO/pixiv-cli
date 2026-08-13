package parse_test

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/utils/parse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPositiveInt64ParsesPositiveInteger(t *testing.T) {
	value, err := parse.PositiveInt64("123", "illust_id")

	require.NoError(t, err)
	assert.Equal(t, int64(123), value)
}

func TestPositiveInt64RejectsInvalidOrNonPositiveValues(t *testing.T) {
	_, err := parse.PositiveInt64("abc", "illust_id")
	require.ErrorContains(t, err, "illust_id must be a positive integer")

	_, err = parse.PositiveInt64("0", "illust_id")
	require.ErrorContains(t, err, "illust_id must be a positive integer")
}
