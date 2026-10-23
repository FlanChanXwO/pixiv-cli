package tools

import (
	"context"
	"testing"
)

func TestHandleIllustDetailRejectsAmbiguousReferenceBeforeOpeningSDK(t *testing.T) {
	result, out, err := pixivTools{}.illustDetailHandler(context.Background(), illustReferenceIn{IllustID: 1, URL: "https://www.pixiv.net/artworks/1"})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !result.IsError || len(out.Records) != 0 {
		t.Fatalf("expected structured validation error, got result=%#v out=%#v", result, out)
	}
}
