package ids

import (
	"reflect"
	"testing"
)

func TestDeduplicatePositiveSortsAndDropsInvalidIDs(t *testing.T) {
	t.Parallel()

	if got := DeduplicatePositive([]int64{3, 1, 3, 0, -1, 2}); !reflect.DeepEqual(got, []int64{1, 2, 3}) {
		t.Fatalf("DeduplicatePositive() = %#v", got)
	}
}
