package pixiv

import "errors"

type contentRoute uint8

const (
	routeApp contentRoute = iota + 1
	routeWeb
	routeAppThenWeb
)

type operationPolicy struct {
	authenticated contentRoute
	anonymousWeb  bool
}

// policyFor 是公开 content operation 的唯一、不可变路由表。
func policyFor(operation Operation) (operationPolicy, bool) {
	switch operation {
	case OperationIllustDetail, OperationUgoiraMetadata:
		return operationPolicy{authenticated: routeAppThenWeb, anonymousWeb: true}, true
	case OperationIllustPages:
		return operationPolicy{authenticated: routeWeb, anonymousWeb: true}, true
	case OperationSearchIllust, OperationIllustRanking, OperationSearchUser:
		return operationPolicy{authenticated: routeApp, anonymousWeb: true}, true
	case OperationIllustRelated, OperationTrendingTagsIllust,
		OperationIllustRecommended, OperationFollowingIllusts,
		OperationUserDetail, OperationUserArtworks, OperationUserBookmarks, OperationUserFollowing:
		return operationPolicy{authenticated: routeApp}, true
	default:
		return operationPolicy{}, false
	}
}

// selectRoute 统一执行认证状态与匿名 Web 白名单判断；adapter 调用仍由各用例负责。
func (c *Client) selectRoute(operation Operation, illustID, userID int64) (contentRoute, error) {
	policy, ok := policyFor(operation)
	if !ok {
		return 0, localRouteError(CodeUnsupported, operation, illustID, userID, errors.New("operation is unsupported"))
	}
	if c.authenticated {
		return policy.authenticated, nil
	}
	if policy.anonymousWeb && c.webFallbackEnabled {
		return routeWeb, nil
	}
	return 0, localRouteError(CodeUnauthorized, operation, illustID, userID, errors.New("access token is required"))
}

func localRouteError(code ErrorCode, operation Operation, illustID, userID int64, cause error) error {
	err := newError(code, operation, "", false, 0, illustID, cause)
	err.UserID = userID
	return err
}
