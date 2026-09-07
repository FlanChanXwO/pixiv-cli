package protocol_test

import (
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/pixiv/protocol"
)

// TestCommentAndStampPaths 固定 comments/stamps endpoint 的候选协议路径。
func TestCommentAndStampPaths(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		got  string
		want string
	}{
		"artwork comments read": {
			got:  protocol.AppIllustComments,
			want: "/v3/illust/comments",
		},
		"artwork comment add": {
			got:  protocol.AppIllustCommentAdd,
			want: "/v1/illust/comment/add",
		},
		"artwork comment delete": {
			got:  protocol.AppIllustCommentDelete,
			want: "/v1/illust/comment/delete",
		},
		"novel comments read": {
			got:  protocol.AppNovelComments,
			want: "/v2/novel/comments",
		},
		"novel comment add": {
			got:  protocol.AppNovelCommentAdd,
			want: "/v1/novel/comment/add",
		},
		"novel comment delete": {
			got:  protocol.AppNovelCommentDelete,
			want: "/v1/novel/comment/delete",
		},
		"stamps read": {
			got:  protocol.AppStamps,
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
