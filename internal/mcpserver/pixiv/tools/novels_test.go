package tools

import (
	"context"
	"testing"
)

func TestHandleNovelDetailRejectsNonPositiveIDBeforeOpeningSDK(t *testing.T) {
	result, out, err := pixivTools{}.novelDetailHandler(context.Background(), novelDetailIn{NovelID: 0})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !result.IsError || len(out.Records) != 0 {
		t.Fatalf("expected structured validation error, got result=%#v out=%#v", result, out)
	}
}
