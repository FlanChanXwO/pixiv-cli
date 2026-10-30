// Package target 按命令声明的契约，把文本值或结构化记录解析成 Pixiv 目标。
//
// 包的组成：[Resolve] 应用一份 [Contract] 并返回 [Target]。
// 只有契约显式提供 [BareIDPolicy] 时才会为裸 ID 发起探测请求，其余情况不做网络 I/O。
package target

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// Namespace 指定输入身份或操作内容的命名空间；URL 专属身份由 ReferenceKind 保留。
type Namespace string

const (
	// NamespaceArtwork 表示作品。
	NamespaceArtwork Namespace = "artwork"
	// NamespaceNovel 表示小说。
	NamespaceNovel Namespace = "novel"
	// NamespaceUser 表示用户。
	NamespaceUser Namespace = "user"
	// NamespaceSeries 表示系列。
	NamespaceSeries Namespace = "series"
	// NamespaceComment 表示评论。
	NamespaceComment Namespace = "comment"
	// NamespaceTag 表示标签。
	NamespaceTag Namespace = "tag"
)

// Source 记录解析结果的本地输入来源，便于诊断时区分 ID、URL 和探测。
type Source string

const (
	// SourceID 表示输入是一个 ID 文本。
	SourceID Source = "id"
	// SourceURL 表示输入是 Pixiv URL。
	SourceURL Source = "url"
	// SourceRecord 表示输入是结构化记录。
	SourceRecord Source = "record"
	// SourceBareIDProbe 表示裸 ID 经契约允许的命名空间探测确定类型。
	SourceBareIDProbe Source = "bare_id_probe"
)

// URLTypeRelation 控制 URL 声明的命名空间是否必须与命令选定的结果类型一致。
type URLTypeRelation string

const (
	// URLTypeMustMatchResult 要求 URL 类型与结果类型一致；detail、series 和作品相关写操作使用它。
	URLTypeMustMatchResult URLTypeRelation = "must_match_result"
	// URLTypeIndependent 允许属主 URL 与结果类型不同，例如用户 URL 配合 bookmark --type novel。
	URLTypeIndependent URLTypeRelation = "independent"
)

// Input 承载命令解析器接受的输入。Value 和 Record 必须恰好提供一个；Type 是命令已解析的
// 类型选择器，为空时使用 Contract.DefaultType 或从 URL 推断。
type Input struct {
	Value  string
	Record *record.Record
	Type   string
}

// TypeSpec 描述命令契约接受的一个类型取值。BareReferenceKind 是同一类型以数字 ID
// 提供时使用的命名空间；例如收藏列表把作品、小说结果选择器映射到用户裸 ID 命名空间。
type TypeSpec struct {
	Name              string
	ResultKind        Namespace
	BareReferenceKind pixiv.ReferenceKind
	// BareTargetKind 承载没有可引用 Pixiv 页面的输入身份，例如评论 ID。设置时若同时
	// 提供 BareReferenceKind，两者推导出的目标类型必须一致。
	BareTargetKind Namespace
	// All 表示该类型是“全部”聚合选择器，不是具体子类型。
	All bool
}

// BareIDPolicy 只为显式固定候选命名空间的契约启用受控探测。探测必须报告每个候选是
// 找到、不存在、禁止访问还是暂时不可用；解析器不会从未分类的错误猜测类型。
type BareIDPolicy struct {
	Candidates []string
	Probe      func(context.Context, int64, TypeSpec) ProbeOutcome
}

// ProbeStatus 表示裸 ID 探测返回的结果类别。
type ProbeStatus string

const (
	// ProbeFound 表示候选命名空间中找到了该 ID。
	ProbeFound ProbeStatus = "found"
	// ProbeNotFound 表示候选命名空间中明确不存在该 ID。
	ProbeNotFound ProbeStatus = "not_found"
	// ProbeForbidden 表示账号无权访问该候选命名空间。
	ProbeForbidden ProbeStatus = "forbidden"
	// ProbeNetworkError 表示探测请求因网络失败而无法得出结论。
	ProbeNetworkError ProbeStatus = "network_error"
	// ProbeIndeterminate 表示探测结果无法明确分类，解析器据此停止尝试其他候选。
	ProbeIndeterminate ProbeStatus = "indeterminate"
)

// ProbeOutcome 由 BareIDPolicy.Probe 返回。禁止访问或网络失败时 Err 应是分类过的
// SDK 错误；Status 始终优先，解析器不会因此静默改试另一个命名空间。
type ProbeOutcome struct {
	Status ProbeStatus
	Err    error
}

// Contract 保存单条命令的输入与冲突规则。每份契约只声明自己支持的类型取值、URL 类型和
// 可选探测候选，因此这里不是全仓库通用的实体联合。
type Contract struct {
	Operation   string
	DefaultType string
	Types       []TypeSpec
	URLKinds    map[pixiv.ReferenceKind]URLTypeRelation
	BareID      *BareIDPolicy
}

// Target 保存 command owner 使用的解析结果。TargetKind 是输入命名空间，ResultKind 是选定的
// 操作内容；两者共用 Namespace，ReferenceKind 保留 URL 专属区别。
type Target struct {
	ID            int64
	TargetKind    Namespace
	ResultKind    Namespace
	ReferenceKind pixiv.ReferenceKind
	OwnerUserID   int64
	All           bool
	Source        Source
}

// Resolve 把一份命令契约应用到文本值或标准记录。契约不提供裸 ID 探测时不做网络 I/O；
// 只有未带类型的数字输入才可能在契约允许时触发一次探测。
func Resolve(ctx context.Context, input Input, contract Contract) (Target, error) {
	if input.Record != nil && strings.TrimSpace(input.Value) != "" {
		return Target{}, invalid(contract, "exactly one input value is allowed")
	}
	if input.Record != nil {
		return resolveRecord(*input.Record, input.Type, contract)
	}
	value := strings.TrimSpace(input.Value)
	if value == "" {
		return Target{}, invalid(contract, "input value is required")
	}
	return resolveValue(ctx, value, input.Type, contract)
}

func resolveValue(ctx context.Context, value, typeName string, contract Contract) (Target, error) {
	if id, err := strconv.ParseInt(value, 10, 64); err == nil {
		if id <= 0 {
			return Target{}, invalid(contract, "id must be a positive integer")
		}
		return resolveID(ctx, id, typeName, contract, SourceID)
	}

	ref, err := pixiv.ParseURL(value)
	if err != nil {
		return Target{}, invalid(contract, "input must be a positive ID or a supported Pixiv URL")
	}
	relation, ok := contract.URLKinds[ref.Kind]
	if !ok {
		return Target{}, invalid(contract, "URL kind is not allowed for this command")
	}

	selection, err := selectType(typeName, ref, contract)
	if err != nil {
		return Target{}, err
	}
	if err := validateURLRelation(ref, selection, relation, contract); err != nil {
		return Target{}, err
	}
	return targetFromReference(ref, selection, SourceURL, contract)
}

func resolveRecord(source record.Record, typeName string, contract Contract) (Target, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(source.ID()), 10, 64)
	if err != nil || id <= 0 {
		return Target{}, invalid(contract, "record id must be a positive integer")
	}
	ref, err := pixiv.ParseURL(source.URL())
	if err != nil {
		return Target{}, invalid(contract, "record url must be a supported Pixiv URL")
	}
	if ref.ID != id {
		return Target{}, invalid(contract, "record id does not match record url")
	}
	if err := validateRecordNamespace(source.Type(), ref.Kind, contract); err != nil {
		return Target{}, err
	}

	selection, err := selectType(typeName, ref, contract)
	if err != nil {
		return Target{}, err
	}
	if err := validateURLRelation(ref, selection, contract.URLKinds[ref.Kind], contract); err != nil {
		return Target{}, err
	}
	target, err := targetFromReference(ref, selection, SourceRecord, contract)
	if err != nil {
		return Target{}, err
	}
	target.ID = id
	return target, nil
}

func resolveID(ctx context.Context, id int64, typeName string, contract Contract, source Source) (Target, error) {
	if strings.TrimSpace(typeName) != "" || strings.TrimSpace(contract.DefaultType) != "" {
		selection, err := selectType(typeName, pixiv.Reference{}, contract)
		if err != nil {
			return Target{}, err
		}
		return targetFromID(id, selection, source, contract)
	}
	if contract.BareID == nil {
		return Target{}, invalid(contract, "bare ID requires an explicit type")
	}
	return resolveBareID(ctx, id, contract)
}

func resolveBareID(ctx context.Context, id int64, contract Contract) (Target, error) {
	policy := contract.BareID
	if policy == nil || len(policy.Candidates) == 0 || policy.Probe == nil {
		return Target{}, invalid(contract, "bare-ID probe is not configured for this command")
	}
	types, err := contractTypes(contract)
	if err != nil {
		return Target{}, err
	}
	var found *TypeSpec
	for _, candidateName := range policy.Candidates {
		candidate, ok := types[candidateName]
		if !ok {
			return Target{}, invalid(contract, "bare-ID probe candidate is not declared by this command")
		}
		if _, err := targetKindForType(candidate, contract); err != nil {
			return Target{}, err
		}
		outcome := policy.Probe(ctx, id, candidate)
		status, probeErr := normalizeProbeOutcome(outcome, contract)
		switch status {
		case ProbeNotFound:
			continue
		case ProbeFound:
			if found != nil {
				return Target{}, invalid(contract, "bare ID matches multiple resource types")
			}
			selected := candidate
			found = &selected
		case ProbeForbidden, ProbeNetworkError:
			if probeErr != nil {
				return Target{}, probeErr
			}
			return Target{}, probeError(status, contract)
		default:
			if probeErr != nil {
				return Target{}, invalid(contract, "bare-ID probe could not determine a resource type")
			}
			return Target{}, invalid(contract, "bare-ID probe returned an invalid outcome")
		}
	}
	if found == nil {
		return Target{}, sdk.NewError("pixiv", operation(contract), sdk.NotFound)
	}
	return targetFromID(id, *found, SourceBareIDProbe, contract)
}

func selectType(typeName string, ref pixiv.Reference, contract Contract) (TypeSpec, error) {
	types, err := contractTypes(contract)
	if err != nil {
		return TypeSpec{}, err
	}
	name := strings.TrimSpace(typeName)
	if name == "" {
		name = strings.TrimSpace(contract.DefaultType)
	}
	if name != "" {
		selection, ok := types[name]
		if !ok {
			return TypeSpec{}, invalid(contract, fmt.Sprintf("type %q is not supported by this command", name))
		}
		return selection, nil
	}
	if ref.Kind == "" {
		return TypeSpec{}, invalid(contract, "an explicit type is required for this ID")
	}
	want, ok := resultKindForReference(ref.Kind)
	if !ok {
		return TypeSpec{}, invalid(contract, "URL kind cannot infer a result type")
	}
	var selected *TypeSpec
	for _, candidate := range types {
		if candidate.All || candidate.ResultKind != want {
			continue
		}
		copy := candidate
		if selected != nil {
			return TypeSpec{}, invalid(contract, "URL does not identify one result type")
		}
		selected = &copy
	}
	if selected == nil {
		return TypeSpec{}, invalid(contract, "an explicit type is required for this URL")
	}
	return *selected, nil
}

func validateURLRelation(ref pixiv.Reference, selection TypeSpec, relation URLTypeRelation, contract Contract) error {
	if relation != URLTypeMustMatchResult && relation != URLTypeIndependent {
		return invalid(contract, "URL type relation is not supported")
	}
	if relation == URLTypeIndependent {
		return nil
	}
	want, ok := resultKindForReference(ref.Kind)
	if !ok || selection.All || selection.ResultKind != want {
		return invalid(contract, "URL namespace conflicts with the selected type")
	}
	return nil
}

func targetFromReference(ref pixiv.Reference, selection TypeSpec, source Source, contract Contract) (Target, error) {
	targetKind, ok := targetKindForReference(ref.Kind)
	if !ok {
		return Target{}, invalid(contract, "URL kind is not a supported target")
	}
	return Target{
		ID:            ref.ID,
		TargetKind:    targetKind,
		ResultKind:    selection.ResultKind,
		ReferenceKind: ref.Kind,
		OwnerUserID:   ref.OwnerUserID,
		All:           selection.All,
		Source:        source,
	}, nil
}

func targetFromID(id int64, selection TypeSpec, source Source, contract Contract) (Target, error) {
	targetKind, err := targetKindForType(selection, contract)
	if err != nil {
		return Target{}, err
	}
	return Target{
		ID:            id,
		TargetKind:    targetKind,
		ResultKind:    selection.ResultKind,
		ReferenceKind: selection.BareReferenceKind,
		All:           selection.All,
		Source:        source,
	}, nil
}

func validateRecordNamespace(recordType string, refKind pixiv.ReferenceKind, contract Contract) error {
	result := Namespace(recordType)
	switch refKind {
	case pixiv.ReferenceKindArtwork:
		if result != NamespaceArtwork {
			return invalid(contract, "record type conflicts with artwork URL")
		}
	case pixiv.ReferenceKindNovel:
		if result != NamespaceNovel {
			return invalid(contract, "record type conflicts with novel URL")
		}
	case pixiv.ReferenceKindUser:
		if result != NamespaceUser {
			return invalid(contract, "record type conflicts with user URL")
		}
	case pixiv.ReferenceKindUserBookmarks:
		if recordType != string(pixiv.ReferenceKindUserBookmarks) && result != NamespaceArtwork {
			return invalid(contract, "record type conflicts with user bookmarks URL")
		}
	case pixiv.ReferenceKindArtworkSeries:
		if recordType != string(NamespaceSeries) && result != NamespaceArtwork {
			return invalid(contract, "record type conflicts with artwork series URL")
		}
	case pixiv.ReferenceKindNovelSeries:
		if recordType != string(NamespaceSeries) && result != NamespaceNovel {
			return invalid(contract, "record type conflicts with novel series URL")
		}
	default:
		return invalid(contract, "record URL kind is not supported")
	}
	return nil
}

func resultKindForReference(kind pixiv.ReferenceKind) (Namespace, bool) {
	switch kind {
	case pixiv.ReferenceKindArtwork, pixiv.ReferenceKindUserBookmarks, pixiv.ReferenceKindArtworkSeries:
		return NamespaceArtwork, true
	case pixiv.ReferenceKindNovel, pixiv.ReferenceKindNovelSeries:
		return NamespaceNovel, true
	case pixiv.ReferenceKindUser:
		return NamespaceUser, true
	default:
		return "", false
	}
}

func targetKindForReference(kind pixiv.ReferenceKind) (Namespace, bool) {
	switch kind {
	case pixiv.ReferenceKindArtwork:
		return NamespaceArtwork, true
	case pixiv.ReferenceKindNovel:
		return NamespaceNovel, true
	case pixiv.ReferenceKindUser, pixiv.ReferenceKindUserBookmarks:
		return NamespaceUser, true
	case pixiv.ReferenceKindArtworkSeries, pixiv.ReferenceKindNovelSeries:
		return NamespaceSeries, true
	default:
		return "", false
	}
}

func targetKindForType(selection TypeSpec, contract Contract) (Namespace, error) {
	fromReference, referenceOK := targetKindForReference(selection.BareReferenceKind)
	if selection.BareReferenceKind != "" && !referenceOK {
		return "", invalid(contract, "type contains an unsupported bare reference kind")
	}
	if selection.BareTargetKind != "" {
		switch selection.BareTargetKind {
		case NamespaceArtwork, NamespaceNovel, NamespaceUser, NamespaceSeries, NamespaceComment:
		default:
			return "", invalid(contract, "type contains an unsupported bare target kind")
		}
		if referenceOK && fromReference != selection.BareTargetKind {
			return "", invalid(contract, "type contains conflicting bare target identities")
		}
		return selection.BareTargetKind, nil
	}
	if referenceOK {
		return fromReference, nil
	}
	return "", invalid(contract, "type cannot resolve a bare ID in this command")
}

func contractTypes(contract Contract) (map[string]TypeSpec, error) {
	types := make(map[string]TypeSpec, len(contract.Types))
	for _, candidate := range contract.Types {
		if strings.TrimSpace(candidate.Name) == "" || candidate.ResultKind == "" && !candidate.All {
			return nil, invalid(contract, "type contract contains an incomplete selector")
		}
		if _, exists := types[candidate.Name]; exists {
			return nil, invalid(contract, "type contract contains a duplicate selector")
		}
		types[candidate.Name] = candidate
	}
	if contract.DefaultType != "" {
		if _, ok := types[contract.DefaultType]; !ok {
			return nil, invalid(contract, "default type is not declared by this command")
		}
	}
	if contract.BareID != nil {
		seen := make(map[string]struct{}, len(contract.BareID.Candidates))
		for _, name := range contract.BareID.Candidates {
			if _, duplicate := seen[name]; duplicate {
				return nil, invalid(contract, "bare-ID probe contains a duplicate candidate")
			}
			seen[name] = struct{}{}
		}
	}
	return types, nil
}

func normalizeProbeOutcome(outcome ProbeOutcome, contract Contract) (ProbeStatus, error) {
	if outcome.Status == "" && outcome.Err != nil {
		switch sdk.ReasonOf(outcome.Err) {
		case sdk.NotFound:
			outcome.Status = ProbeNotFound
		case sdk.Forbidden:
			outcome.Status = ProbeForbidden
		case sdk.UpstreamUnavailable:
			outcome.Status = ProbeNetworkError
		default:
			outcome.Status = ProbeIndeterminate
		}
	}
	switch outcome.Status {
	case ProbeFound, ProbeNotFound:
		if outcome.Err != nil {
			return ProbeIndeterminate, invalid(contract, "bare-ID probe returned an error with a terminal result")
		}
		return outcome.Status, nil
	case ProbeForbidden, ProbeNetworkError:
		return outcome.Status, classifyProbeError(outcome.Status, outcome.Err, contract)
	case ProbeIndeterminate:
		return ProbeIndeterminate, outcome.Err
	default:
		return ProbeIndeterminate, invalid(contract, "bare-ID probe returned an unknown status")
	}
}

func classifyProbeError(status ProbeStatus, err error, contract Contract) error {
	if err == nil {
		return probeError(status, contract)
	}
	if sdk.ReasonOf(err) != "" {
		return err
	}
	if errors.Is(err, context.Canceled) {
		return sdk.NewError("pixiv", operation(contract), sdk.UpstreamUnavailable, sdk.WithCause(context.Canceled))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return sdk.NewError("pixiv", operation(contract), sdk.UpstreamUnavailable, sdk.WithCause(context.DeadlineExceeded))
	}
	return probeError(status, contract)
}

func probeError(status ProbeStatus, contract Contract) error {
	if status == ProbeForbidden {
		return sdk.NewError("pixiv", operation(contract), sdk.Forbidden)
	}
	return sdk.NewError("pixiv", operation(contract), sdk.UpstreamUnavailable)
}

func operation(contract Contract) string {
	if strings.TrimSpace(contract.Operation) == "" {
		return "Resolve"
	}
	return contract.Operation
}

func invalid(contract Contract, detail string) error {
	return sdk.NewError("pixiv", operation(contract), sdk.InvalidArgument, sdk.WithDetail(detail))
}
