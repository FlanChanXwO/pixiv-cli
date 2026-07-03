package parse

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPositiveInt64ParsesPositiveInteger(t *testing.T) {
	value, err := PositiveInt64("123", "illust_id")

	require.NoError(t, err)
	assert.Equal(t, int64(123), value)
}

func TestPositiveInt64RejectsInvalidOrNonPositiveValues(t *testing.T) {
	_, err := PositiveInt64("abc", "illust_id")
	require.ErrorContains(t, err, "illust_id must be a positive integer")

	_, err = PositiveInt64("0", "illust_id")
	require.ErrorContains(t, err, "illust_id must be a positive integer")
}
