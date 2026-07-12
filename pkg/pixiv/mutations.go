package pixiv

import (
	"context"
	"errors"
	"strings"
)

// AddBookmark 收藏作品。restrict 省略时使用 public；标签按调用方顺序原样提交。
func (c *Client) AddBookmark(ctx context.Context, request AddBookmarkRequest) error {
	if err := validateBookmarkRequest(request); err != nil {
		return err
	}
	if scoped, err := c.operationClient(ctx, OperationAddBookmark); err != nil {
		return err
	} else if scoped != c {
		return scoped.AddBookmark(ctx, request)
	}
	if err := c.requireRoute(OperationAddBookmark, routeApp, request.IllustID, 0); err != nil {
		return err
	}
	restrict := defaultRestrict(request.Restrict)
	if err := c.app.AddBookmark(ctx, request.IllustID, string(restrict), request.Tags); err != nil {
		return mapAppError(err, OperationAddBookmark, request.IllustID)
	}
	return nil
}

// RemoveBookmark 取消收藏作品；它不读取当前收藏状态。
func (c *Client) RemoveBookmark(ctx context.Context, request RemoveBookmarkRequest) error {
	if request.IllustID <= 0 {
		return newError(CodeInvalidArgument, OperationRemoveBookmark, "", false, 0, request.IllustID, errors.New("illust id must be positive"))
	}
	if scoped, err := c.operationClient(ctx, OperationRemoveBookmark); err != nil {
		return err
	} else if scoped != c {
		return scoped.RemoveBookmark(ctx, request)
	}
	if err := c.requireRoute(OperationRemoveBookmark, routeApp, request.IllustID, 0); err != nil {
		return err
	}
	if err := c.app.RemoveBookmark(ctx, request.IllustID); err != nil {
		return mapAppError(err, OperationRemoveBookmark, request.IllustID)
	}
	return nil
}

// FollowUser 关注用户。restrict 省略时使用 public；它不读取当前关注状态。
func (c *Client) FollowUser(ctx context.Context, request FollowUserRequest) error {
	if err := validateFollowRequest(request); err != nil {
		return err
	}
	if scoped, err := c.operationClient(ctx, OperationFollowUser); err != nil {
		return err
	} else if scoped != c {
		return scoped.FollowUser(ctx, request)
	}
	if err := c.requireRoute(OperationFollowUser, routeApp, 0, request.UserID); err != nil {
		return err
	}
	restrict := defaultRestrict(request.Restrict)
	if err := c.app.FollowUser(ctx, request.UserID, string(restrict)); err != nil {
		return mapAppOperationError(err, OperationFollowUser, request.UserID)
	}
	return nil
}

// UnfollowUser 取消关注用户；它不读取当前关注状态。
func (c *Client) UnfollowUser(ctx context.Context, request UnfollowUserRequest) error {
	if request.UserID <= 0 {
		return invalidArgument(OperationUnfollowUser, request.UserID, errors.New("user id must be positive"))
	}
	if scoped, err := c.operationClient(ctx, OperationUnfollowUser); err != nil {
		return err
	} else if scoped != c {
		return scoped.UnfollowUser(ctx, request)
	}
	if err := c.requireRoute(OperationUnfollowUser, routeApp, 0, request.UserID); err != nil {
		return err
	}
	if err := c.app.UnfollowUser(ctx, request.UserID); err != nil {
		return mapAppOperationError(err, OperationUnfollowUser, request.UserID)
	}
	return nil
}

func validateBookmarkRequest(request AddBookmarkRequest) error {
	if request.IllustID <= 0 {
		return newError(CodeInvalidArgument, OperationAddBookmark, "", false, 0, request.IllustID, errors.New("illust id must be positive"))
	}
	if !validMutationRestrict(request.Restrict) {
		return newError(CodeInvalidArgument, OperationAddBookmark, "", false, 0, request.IllustID, errors.New("restrict must be public or private"))
	}
	for _, tag := range request.Tags {
		if strings.TrimSpace(tag) == "" {
			return newError(CodeInvalidArgument, OperationAddBookmark, "", false, 0, request.IllustID, errors.New("bookmark tag must not be empty"))
		}
	}
	return nil
}

func validateFollowRequest(request FollowUserRequest) error {
	if request.UserID <= 0 {
		return invalidArgument(OperationFollowUser, request.UserID, errors.New("user id must be positive"))
	}
	if !validMutationRestrict(request.Restrict) {
		return invalidArgument(OperationFollowUser, request.UserID, errors.New("restrict must be public or private"))
	}
	return nil
}

func validMutationRestrict(restrict Restrict) bool {
	return restrict == "" || restrict == RestrictPublic || restrict == RestrictPrivate
}

func defaultRestrict(restrict Restrict) Restrict {
	if restrict == "" {
		return RestrictPublic
	}
	return restrict
}
