package user_detail

import (
	"context"
	"testing"
)

func TestHandleUserDetailRejectsNonPositiveIDBeforeOpeningSDK(t *testing.T) {
	result, out, err := handleUserDetail(context.Background(), nil, In{UserID: -1})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !result.IsError || len(out.Records) != 0 {
		t.Fatalf("expected structured validation error, got result=%#v out=%#v", result, out)
	}
}
