package pixiv

import (
	"errors"
	"testing"
)

func TestOperationPolicyIsCompleteAndUnknownIsUnsupported(t *testing.T) {
	tests := []struct {
		operation     Operation
		authenticated contentRoute
		anonymousWeb  bool
	}{
		{OperationIllustDetail, routeAppThenWeb, true}, {OperationIllustPages, routeWeb, true},
		{OperationIllustRelated, routeApp, false}, {OperationTrendingTagsIllust, routeApp, false},
		{OperationUgoiraMetadata, routeAppThenWeb, true}, {OperationSearchIllust, routeApp, true},
		{OperationIllustRanking, routeApp, true}, {OperationIllustRecommended, routeApp, false},
		{OperationMangaRecommended, routeApp, false}, {OperationNovelRecommended, routeApp, false}, {OperationUserRecommended, routeApp, false},
		{OperationFollowingIllusts, routeApp, false}, {OperationSearchUser, routeApp, true},
		{OperationUserDetail, routeApp, false}, {OperationUserArtworks, routeApp, false},
		{OperationUserBookmarks, routeApp, false}, {OperationUserFollowing, routeApp, false},
	}
	for _, tt := range tests {
		policy, ok := policyFor(tt.operation)
		if !ok || policy.authenticated != tt.authenticated || policy.anonymousWeb != tt.anonymousWeb {
			t.Fatalf("policy %s = %+v, %v", tt.operation, policy, ok)
		}
		auth := &Client{authenticated: true, webFallbackEnabled: true}
		if route, err := auth.selectRoute(tt.operation, 7, 8); err != nil || route != tt.authenticated {
			t.Fatalf("auth %s route=%v err=%v", tt.operation, route, err)
		}
		anonEnabled := &Client{webFallbackEnabled: true}
		route, err := anonEnabled.selectRoute(tt.operation, 7, 8)
		if tt.anonymousWeb {
			if err != nil || route != routeWeb {
				t.Fatalf("anonymous enabled %s route=%v err=%v", tt.operation, route, err)
			}
		} else {
			assertLocalRouteCode(t, err, CodeUnauthorized, tt.operation)
		}
		anonDisabled := &Client{}
		_, err = anonDisabled.selectRoute(tt.operation, 7, 8)
		assertLocalRouteCode(t, err, CodeUnauthorized, tt.operation)
	}
	client := &Client{authenticated: true}
	_, err := client.selectRoute(Operation("unknown"), 1, 2)
	typed, ok := err.(*Error)
	if !ok || typed.Code != CodeUnsupported || typed.Backend != "" || typed.IllustID != 1 || typed.UserID != 2 {
		t.Fatalf("err=%#v", err)
	}
	err = client.requireRoute(OperationIllustDetail, routeApp, 7, 8)
	assertLocalRouteCode(t, err, CodeUnsupported, OperationIllustDetail)
}

func assertLocalRouteCode(t *testing.T, err error, code ErrorCode, operation Operation) {
	t.Helper()
	var typed *Error
	if !errors.As(err, &typed) || typed.Code != code || typed.Operation != operation || typed.Backend != "" || typed.IllustID != 7 || typed.UserID != 8 {
		t.Fatalf("err=%#v", err)
	}
}
