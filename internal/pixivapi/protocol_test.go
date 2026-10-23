package pixivapi_test

import (
	"github.com/FlanChanXwO/pixiv-cli/internal/pixivapi"
	"testing"
)

// TestCommentAndStampPaths 固定 comments/stamps endpoint 的候选协议路径。
func TestCommentAndStampPaths(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		got  string
		want string
	}{
		"artwork comments read": {
			got:  pixivapi.AppIllustComments,
			want: "/v3/illust/comments",
		},
		"artwork comment add": {
			got:  pixivapi.AppIllustCommentAdd,
			want: "/v1/illust/comment/add",
		},
		"artwork comment delete": {
			got:  pixivapi.AppIllustCommentDelete,
			want: "/v1/illust/comment/delete",
		},
		"novel comments read": {
			got:  pixivapi.AppNovelComments,
			want: "/v2/novel/comments",
		},
		"novel comment add": {
			got:  pixivapi.AppNovelCommentAdd,
			want: "/v1/novel/comment/add",
		},
		"novel comment delete": {
			got:  pixivapi.AppNovelCommentDelete,
			want: "/v1/novel/comment/delete",
		},
		"stamps read": {
			got:  pixivapi.AppStamps,
			want: "/v1/stamps",
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if test.got != test.want {
				t.Errorf("path = %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestNovelRankingPath(t *testing.T) {
	if got, want := pixivapi.AppNovelRanking, "/v1/novel/ranking"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}
