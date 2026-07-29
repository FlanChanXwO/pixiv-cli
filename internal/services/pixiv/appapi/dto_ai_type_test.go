package appapi

import (
	"encoding/json"
	"testing"
)

// App API 现网可能返回 illust_ai_type；旧字段 ai_type 仍需兼容。
// 优先 illust_ai_type；缺失时回退 ai_type；两者都缺则为 0。
// 本地 AI 判定固定以 AIType==2 为 AI（见 public SDK searchAIModeAccepts）。
func TestIllustDTOUnmarshalAITypeFieldPriority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want int
	}{
		{
			name: "only illust_ai_type",
			raw:  `{"id":1,"title":"a","type":"illust","page_count":1,"illust_ai_type":2}`,
			want: 2,
		},
		{
			name: "only legacy ai_type",
			raw:  `{"id":2,"title":"b","type":"illust","page_count":1,"ai_type":2}`,
			want: 2,
		},
		{
			name: "both present prefers illust_ai_type",
			raw:  `{"id":3,"title":"c","type":"illust","page_count":1,"illust_ai_type":1,"ai_type":2}`,
			want: 1,
		},
		{
			name: "both present prefers illust_ai_type even when zero",
			raw:  `{"id":4,"title":"d","type":"illust","page_count":1,"illust_ai_type":0,"ai_type":2}`,
			want: 0,
		},
		{
			name: "non-AI value preserved",
			raw:  `{"id":5,"title":"e","type":"illust","page_count":1,"illust_ai_type":1}`,
			want: 1,
		},
		{
			name: "missing both defaults to zero",
			raw:  `{"id":6,"title":"f","type":"illust","page_count":1}`,
			want: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var dto illustDTO
			if err := json.Unmarshal([]byte(tt.raw), &dto); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if dto.AIType != tt.want {
				t.Fatalf("AIType = %d, want %d (raw=%s)", dto.AIType, tt.want, tt.raw)
			}
			// mapper 必须透传归一化后的 AIType，不得二次改写。
			if got := mapIllust(dto).AIType; got != tt.want {
				t.Fatalf("mapIllust().AIType = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIllustListDTOUnmarshalIllustAITypeThroughList(t *testing.T) {
	t.Parallel()
	raw := `{
		"illusts":[
			{"id":10,"title":"new","type":"illust","page_count":1,"illust_ai_type":2},
			{"id":11,"title":"old","type":"illust","page_count":1,"ai_type":1},
			{"id":12,"title":"prefer-new","type":"illust","page_count":1,"illust_ai_type":0,"ai_type":2}
		],
		"next_url":null
	}`
	var dto illustListDTO
	if err := json.Unmarshal([]byte(raw), &dto); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	list := mapIllustList(dto)
	if len(list.Illusts) != 3 {
		t.Fatalf("len(illusts)=%d, want 3", len(list.Illusts))
	}
	want := []int{2, 1, 0}
	for i, w := range want {
		if list.Illusts[i].AIType != w {
			t.Fatalf("illusts[%d].AIType=%d, want %d", i, list.Illusts[i].AIType, w)
		}
	}
}
