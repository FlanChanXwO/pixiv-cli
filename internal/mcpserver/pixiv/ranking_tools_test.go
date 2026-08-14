package pixiv_test

import (
	"context"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv/internal/outputs"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// illust_ranking 的 owner 契约：请求映射与全部 mode。
func TestIllustRankingPassesRequestAndReturnsRecord(t *testing.T) {
	var requests []pixiv.ArtworkRankingRequest
	client := &fakeSDKClient{illustRanking: func(_ context.Context, request pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error) {
		requests = append(requests, request)
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{
			testSDKIllust(11, "first", 1),
			testSDKIllust(12, "second", 1),
			testSDKIllust(13, "third", 1),
		}}, nil
	}}
	session, closeSession := newSDKTestSession(t, client)
	defer closeSession()

	result := callTool(t, session, "illust_ranking", map[string]any{
		"mode": "day_male", "date": "2025-02-03", "page": 3, "limit": 1,
	})
	var out outputs.Records
	decodeStructured(t, result, &out)
	if result.IsError {
		t.Fatalf("illust_ranking returned MCP error: %+v", result)
	}
	if len(requests) != 1 || requests[0].Mode != pixiv.RankingModeDayMale || requests[0].Date != "2025-02-03" || !requests[0].Cursor.IsZero() {
		t.Fatalf("ranking requests = %+v", requests)
	}
	if len(out.Records) != 1 || out.Records[0].ID() != "13" {
		t.Fatalf("illust_ranking records = %+v", out)
	}
}

func TestIllustRankingSupportsAllModes(t *testing.T) {
	tests := []struct {
		mode string
	}{
		{mode: "day"},
		{mode: "day_male"},
		{mode: "day_female"},
		{mode: "week"},
		{mode: "week_original"},
		{mode: "week_rookie"},
		{mode: "month"},
		{mode: "day_manga"},
		{mode: "week_manga"},
		{mode: "month_manga"},
		{mode: "week_rookie_manga"},
		{mode: "day_r18"},
		{mode: "day_male_r18"},
		{mode: "day_female_r18"},
		{mode: "week_r18"},
		{mode: "week_r18g"},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			client := &fakeSDKClient{illustRanking: func(_ context.Context, request pixiv.ArtworkRankingRequest) (sdk.Page[pixiv.Artwork], error) {
				if string(request.Mode) != test.mode {
					t.Fatalf("ranking mode = %q, want %q", request.Mode, test.mode)
				}
				return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(13, "ranked", 1)}}, nil
			}}
			session, closeSession := newSDKTestSession(t, client)
			defer closeSession()

			result := callTool(t, session, "illust_ranking", map[string]any{"mode": test.mode})
			var out outputs.Records
			decodeStructured(t, result, &out)
			if result.IsError || len(out.Records) != 1 || out.Records[0].ID() != "13" {
				t.Fatalf("illust_ranking(%q) output = %+v", test.mode, out)
			}
		})
	}
}
