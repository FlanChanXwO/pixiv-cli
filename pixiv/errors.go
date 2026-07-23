package pixiv

import (
	"fmt"
	"strings"
)

// ErrorCode 是供调用方稳定分支处理的机器可读错误码。
type ErrorCode string

const (
	CodeInvalidArgument           ErrorCode = "invalid_argument"
	CodeArtworkUnavailable        ErrorCode = "artwork_unavailable"
	CodeUnauthorized              ErrorCode = "unauthorized"
	CodeForbidden                 ErrorCode = "forbidden"
	CodeUnsupported               ErrorCode = "unsupported"
	CodeRateLimited               ErrorCode = "rate_limited"
	CodeUpstreamError             ErrorCode = "upstream_error"
	CodeUpstreamUnavailable       ErrorCode = "upstream_unavailable"
	CodeMalformedUpstreamResponse ErrorCode = "malformed_upstream_response"
)

// Backend 标识失败所在的协议边界。
type Backend string

const (
	BackendAppAPI   Backend = "app_api"
	BackendWebAPI   Backend = "web_api"
	BackendOAuth    Backend = "oauth"
	BackendResource Backend = "resource"
)

// Operation 标识稳定的公开 SDK 操作。
type Operation string

const (
	OperationIllustDetail        Operation = "illust_detail"
	OperationIllustPages         Operation = "illust_pages"
	OperationIllustRelated       Operation = "illust_related"
	OperationTrendingTagsIllust  Operation = "trending_tags_illust"
	OperationUgoiraMetadata      Operation = "ugoira_metadata"
	OperationSearchIllust        Operation = "search_illust"
	OperationSearchNovel         Operation = "search_novel"
	OperationSearchIllustOptions Operation = "search_illust_options"
	OperationIllustRanking       Operation = "illust_ranking"
	OperationIllustRecommended   Operation = "illust_recommended"
	OperationMangaRecommended    Operation = "manga_recommended"
	OperationNovelRecommended    Operation = "novel_recommended"
	OperationUserRecommended     Operation = "user_recommended"
	OperationFollowingIllusts    Operation = "following_illusts"
	OperationSearchUser          Operation = "search_user"
	OperationUserDetail          Operation = "user_detail"
	OperationUserArtworks        Operation = "user_artworks"
	OperationUserBookmarks       Operation = "user_bookmarks"
	OperationUserFollowing       Operation = "user_following"
	OperationAddBookmark         Operation = "add_bookmark"
	OperationRemoveBookmark      Operation = "remove_bookmark"
	OperationFollowUser          Operation = "follow_user"
	OperationUnfollowUser        Operation = "unfollow_user"
	OperationParseResourceRef    Operation = "parse_resource_ref"
	OperationOpenResource        Operation = "open_resource"
	OperationDownload            Operation = "download"
	OperationRefresh             Operation = "refresh"
	OperationImportAccount       Operation = "import_account"
	OperationListAccounts        Operation = "list_accounts"
	OperationSelectAccount       Operation = "select_account"
	OperationRemoveAccount       Operation = "remove_account"
	OperationCheckAccount        Operation = "check_account"
	OperationCheckRefreshToken   Operation = "check_refresh_token"
	OperationConfigGet           Operation = "config_get"
	OperationConfigSet           Operation = "config_set"
	OperationConfigUnset         Operation = "config_unset"
	OperationStartLogin          Operation = "start_login"
	OperationCompleteLogin       Operation = "complete_login"
	OperationCurrentUserID       Operation = "current_user_id"
	OperationSnapshot            Operation = "snapshot"
	OperationExportAuthBundle    Operation = "export_auth_bundle"
	OperationEncodeAuthBundle    Operation = "encode_auth_bundle"
	OperationDecodeAuthBundle    Operation = "decode_auth_bundle"
	OperationRestoreAuthBundle   Operation = "restore_auth_bundle"
)

const OperationExportAccountRefreshToken Operation = "export_account_refresh_token"

// TransportKind 标识不携带目标地址、证书或凭据的稳定传输失败子类。
type TransportKind string

const (
	TransportKindDNS               TransportKind = "dns"
	TransportKindTLS               TransportKind = "tls"
	TransportKindProxy             TransportKind = "proxy"
	TransportKindConnectionRefused TransportKind = "connection_refused"
	TransportKindConnectionReset   TransportKind = "connection_reset"
	TransportKindTimeout           TransportKind = "timeout"
	TransportKindUnknown           TransportKind = "unknown"
)

// LocalStateKind 标识不携带路径、文件内容或凭据的稳定本地状态失败子类。
type LocalStateKind string

const (
	LocalStateKindAuthMalformed    LocalStateKind = "auth_malformed"
	LocalStateKindConfigMalformed  LocalStateKind = "config_malformed"
	LocalStateKindPermissionDenied LocalStateKind = "permission_denied"
	LocalStateKindNotFound         LocalStateKind = "not_found"
	LocalStateKindInvalidProxy     LocalStateKind = "invalid_proxy"
	LocalStateKindAccountMismatch  LocalStateKind = "account_mismatch"
	LocalStateKindUnavailable      LocalStateKind = "unavailable"
	LocalStateKindUnknown          LocalStateKind = "unknown"
)

// LocalWriteCommitOutcome 标识本地原子写入失败时 replacement 的提交状态。
type LocalWriteCommitOutcome string

const (
	LocalWriteCommitOutcomeUnknown      LocalWriteCommitOutcome = "unknown"
	LocalWriteCommitOutcomeNotCommitted LocalWriteCommitOutcome = "not_committed"
	LocalWriteCommitOutcomeCommitted    LocalWriteCommitOutcome = "committed"
)

// Error 是公开 SDK 的安全、可分类错误。cause 只保存已脱敏原因。
type Error struct {
	Code                    ErrorCode
	Operation               Operation
	Backend                 Backend
	Retryable               bool
	UpstreamStatus          int
	IllustID                int64
	UserID                  int64
	TransportKind           TransportKind
	LocalStateKind          LocalStateKind
	LocalWriteCommitOutcome LocalWriteCommitOutcome
	cause                   error
}

func newUserError(code ErrorCode, operation Operation, backend Backend, retryable bool, status int, userID int64, cause error) *Error {
	err := newError(code, operation, backend, retryable, status, 0, cause)
	err.UserID = userID
	return err
}

// Error 返回不含上游响应体、URL、header 或凭据的诊断文本。
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	parts := []string{"pixiv", string(e.Code)}
	if e.Operation != "" {
		parts = append(parts, "operation="+string(e.Operation))
	}
	if e.Backend != "" {
		parts = append(parts, "backend="+string(e.Backend))
	}
	if e.UpstreamStatus != 0 {
		parts = append(parts, fmt.Sprintf("status=%d", e.UpstreamStatus))
	}
	if e.IllustID > 0 {
		parts = append(parts, fmt.Sprintf("illust_id=%d", e.IllustID))
	}
	if e.UserID > 0 {
		parts = append(parts, fmt.Sprintf("user_id=%d", e.UserID))
	}
	if kind := safeTransportKind(e.TransportKind); kind != "" {
		parts = append(parts, "transport_kind="+string(kind))
	}
	if kind := safeLocalStateKind(e.LocalStateKind); kind != "" {
		parts = append(parts, "local_state_kind="+string(kind))
	}
	if outcome := safeLocalWriteCommitOutcome(e.LocalWriteCommitOutcome); outcome != "" {
		parts = append(parts, "local_write_commit_outcome="+string(outcome))
	}
	return strings.Join(parts, " ")
}

// safeTransportKind 确保公开可写字段不能把任意 URL、主机或凭据带入诊断。
func safeTransportKind(kind TransportKind) TransportKind {
	switch kind {
	case "",
		TransportKindDNS,
		TransportKindTLS,
		TransportKindProxy,
		TransportKindConnectionRefused,
		TransportKindConnectionReset,
		TransportKindTimeout,
		TransportKindUnknown:
		return kind
	default:
		return TransportKindUnknown
	}
}

// safeLocalStateKind 确保公开可写字段不能把路径、配置内容或凭据带入诊断。
func safeLocalStateKind(kind LocalStateKind) LocalStateKind {
	switch kind {
	case "",
		LocalStateKindAuthMalformed,
		LocalStateKindConfigMalformed,
		LocalStateKindPermissionDenied,
		LocalStateKindNotFound,
		LocalStateKindInvalidProxy,
		LocalStateKindAccountMismatch,
		LocalStateKindUnavailable,
		LocalStateKindUnknown:
		return kind
	default:
		return LocalStateKindUnknown
	}
}

// safeLocalWriteCommitOutcome 防止公开可写字段把任意内容带入诊断。
func safeLocalWriteCommitOutcome(outcome LocalWriteCommitOutcome) LocalWriteCommitOutcome {
	switch outcome {
	case "",
		LocalWriteCommitOutcomeUnknown,
		LocalWriteCommitOutcomeNotCommitted,
		LocalWriteCommitOutcomeCommitted:
		return outcome
	default:
		return LocalWriteCommitOutcomeUnknown
	}
}

// Unwrap 暴露已验证安全的 cause，并保留 context 取消等标准错误链。
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Is 让调用方通过稳定 code sentinel 使用 errors.Is。
func (e *Error) Is(target error) bool {
	if e == nil {
		return target == nil
	}
	switch other := target.(type) {
	case codeSentinel:
		return e.Code == ErrorCode(other)
	case *Error:
		return other != nil && other.Code != "" && e.Code == other.Code
	default:
		return false
	}
}

type codeSentinel ErrorCode

func (e codeSentinel) Error() string { return string(e) }

var (
	ErrInvalidArgument           error = codeSentinel(CodeInvalidArgument)
	ErrArtworkUnavailable        error = codeSentinel(CodeArtworkUnavailable)
	ErrUnauthorized              error = codeSentinel(CodeUnauthorized)
	ErrForbidden                 error = codeSentinel(CodeForbidden)
	ErrUnsupported               error = codeSentinel(CodeUnsupported)
	ErrRateLimited               error = codeSentinel(CodeRateLimited)
	ErrUpstreamError             error = codeSentinel(CodeUpstreamError)
	ErrUpstreamUnavailable       error = codeSentinel(CodeUpstreamUnavailable)
	ErrMalformedUpstreamResponse error = codeSentinel(CodeMalformedUpstreamResponse)
)

func newError(code ErrorCode, operation Operation, backend Backend, retryable bool, status int, illustID int64, cause error) *Error {
	return &Error{
		Code:           code,
		Operation:      operation,
		Backend:        backend,
		Retryable:      retryable,
		UpstreamStatus: status,
		IllustID:       illustID,
		cause:          cause,
	}
}
