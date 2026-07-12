package pixiv

import "testing"

func TestOperationPolicyIsCompleteAndUnknownIsUnsupported(t *testing.T) {
	for _, operation := range []Operation{
		OperationIllustDetail, OperationIllustPages, OperationIllustRelated, OperationTrendingTagsIllust,
		OperationUgoiraMetadata, OperationSearchIllust, OperationIllustRanking, OperationIllustRecommended,
		OperationFollowingIllusts, OperationSearchUser, OperationUserDetail, OperationUserArtworks,
		OperationUserBookmarks, OperationUserFollowing,
	} {
		if _, ok := policyFor(operation); !ok {
			t.Fatalf("missing policy for %s", operation)
		}
	}
	client := &Client{authenticated: true}
	_, err := client.selectRoute(Operation("unknown"), 1, 2)
	typed, ok := err.(*Error)
	if !ok || typed.Code != CodeUnsupported || typed.Backend != "" || typed.IllustID != 1 || typed.UserID != 2 {
		t.Fatalf("err=%#v", err)
	}
}
