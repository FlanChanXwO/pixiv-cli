package novel_detail

import (
	"context"
	"testing"
)

func TestHandleNovelDetailRejectsNonPositiveIDBeforeOpeningSDK(t *testing.T) {
	result, out, err := handleNovelDetail(context.Background(), nil, In{NovelID: 0})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !result.IsError || len(out.Records) != 0 {
		t.Fatalf("expected structured validation error, got result=%#v out=%#v", result, out)
	}
}
