// Package resolver owns the command-scoped semantic resolution of Pixiv
// targets. It does not open a client or perform network I/O except through an
// explicitly supplied bare-ID probe.
package resolver

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	recordpkg "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// TargetKind identifies the input namespace selected by a command contract.
// URL-specific identities such as user_bookmarks and artwork_series remain in
// Target.ReferenceKind rather than being collapsed into a new global kind.
type TargetKind string

const (
	TargetKindArtwork TargetKind = "artwork"
	TargetKindNovel   TargetKind = "novel"
	TargetKindUser    TargetKind = "user"
	TargetKindSeries  TargetKind = "series"
	TargetKindComment TargetKind = "comment"
)

// ResultKind identifies the content returned or mutated by a command.
type ResultKind string

const (
	ResultKindArtwork ResultKind = "artwork"
	ResultKindNovel   ResultKind = "novel"
	ResultKindUser    ResultKind = "user"
	ResultKindSeries  ResultKind = "series"
	ResultKindComment ResultKind = "comment"
	ResultKindTag     ResultKind = "tag"
)

// Subtype is an endpoint-specific content subtype. It is deliberately
// separate from both the input target namespace and the result kind.
type Subtype string

const (
	SubtypeIllust Subtype = "illust"
	SubtypeManga  Subtype = "manga"
	SubtypeUgoira Subtype = "ugoira"
)

// Source records which local input path produced a resolved target.
type Source string

const (
	SourceID          Source = "id"
	SourceURL         Source = "url"
	SourceRecord      Source = "record"
	SourceBareIDProbe Source = "bare_id_probe"
)

// URLTypeRelation controls whether a parsed URL's declared namespace must
// agree with the command's selected result type.
type URLTypeRelation string

const (
	// URLTypeMustMatchResult is used by detail, series, and work mutations.
	URLTypeMustMatchResult URLTypeRelation = "must_match_result"
	// URLTypeIndependent allows an owner URL to select a different result kind,
	// such as a user URL with bookmark --type novel.
	URLTypeIndependent URLTypeRelation = "independent"
)

// Input is the value accepted by a command resolver. Exactly one of Value and
// Record must be supplied. Type is the command's already-parsed type selector;
// an empty Type uses Contract.DefaultType or URL inference.
type Input struct {
	Value  string
	Record *recordpkg.Record
	Type   string
}

// TypeSpec describes one type value that a particular command contract
// accepts. BareReferenceKind is the namespace used when the same type is
// supplied with a numeric ID. Bookmark list, for example, maps artwork and
// novel result selectors to a user bare-ID namespace.
type TypeSpec struct {
	Name              string
	ResultKind        ResultKind
	Subtype           Subtype
	BareReferenceKind pixiv.ReferenceKind
	// BareTargetKind represents an input identity that has no supported Pixiv
	// page reference, such as a comment ID. When set, it must agree with the
	// target kind derived from BareReferenceKind, if one is also provided.
	BareTargetKind TargetKind
	All            bool
}

// BareIDPolicy enables a controlled probe only for a contract that explicitly
// freezes its candidate namespaces. A probe must report whether each candidate
// was found, absent, forbidden, or unavailable; the resolver never guesses a
// type from an unclassified error.
type BareIDPolicy struct {
	Candidates []string
	Probe      func(context.Context, int64, TypeSpec) ProbeOutcome
}

// ProbeStatus is the outcome class returned by a controlled bare-ID probe.
type ProbeStatus string

const (
	ProbeFound         ProbeStatus = "found"
	ProbeNotFound      ProbeStatus = "not_found"
	ProbeForbidden     ProbeStatus = "forbidden"
	ProbeNetworkError  ProbeStatus = "network_error"
	ProbeIndeterminate ProbeStatus = "indeterminate"
)

// ProbeOutcome is returned by BareIDPolicy.Probe. Err should be a classified
// SDK error for forbidden or network outcomes; the status remains authoritative
// and prevents the resolver from silently trying another namespace.
type ProbeOutcome struct {
	Status ProbeStatus
	Err    error
}

// Contract contains the command-specific input and conflict rules. It is a
// policy value rather than a universal entity union: every command declares
// only the type values, URL kinds, and optional probe candidates it supports.
type Contract struct {
	Operation   string
	DefaultType string
	Types       []TypeSpec
	URLKinds    map[pixiv.ReferenceKind]URLTypeRelation
	BareID      *BareIDPolicy
}

// Target is the resolved identity consumed by a command owner. TargetKind is
// the input namespace, ResultKind is the selected operation content, and
// ReferenceKind preserves URL-specific distinctions such as user bookmarks or
// artwork series. All is a selector and is never represented as a Subtype.
type Target struct {
	ID            int64
	TargetKind    TargetKind
	ResultKind    ResultKind
	Subtype       Subtype
	ReferenceKind pixiv.ReferenceKind
	OwnerUserID   int64
	All           bool
	Source        Source
}

// Resolve applies one command contract to either a text value or a structured
// canonical record. It performs no network I/O unless the contract supplies a
// controlled bare-ID probe for an otherwise untyped numeric value.
func Resolve(ctx context.Context, input Input, contract Contract) (Target, error) {
	if ctx == nil {
		return Target{}, invalid(contract, "context is required")
	}
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

func resolveRecord(source recordpkg.Record, typeName string, contract Contract) (Target, error) {
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
	if err := mergeRecordSubtype(&selection, source.Type(), contract); err != nil {
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
		if candidate.All || candidate.ResultKind != want || candidate.Subtype != "" {
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
		Subtype:       selection.Subtype,
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
		Subtype:       selection.Subtype,
		ReferenceKind: selection.BareReferenceKind,
		All:           selection.All,
		Source:        source,
	}, nil
}

func validateRecordNamespace(recordType string, refKind pixiv.ReferenceKind, contract Contract) error {
	result, _, ok := recordTypeMeaning(recordType)
	if !ok {
		return invalid(contract, "record type is not a supported Pixiv namespace or subtype")
	}
	switch refKind {
	case pixiv.ReferenceKindArtwork:
		if result != ResultKindArtwork {
			return invalid(contract, "record type conflicts with artwork URL")
		}
	case pixiv.ReferenceKindNovel:
		if result != ResultKindNovel {
			return invalid(contract, "record type conflicts with novel URL")
		}
	case pixiv.ReferenceKindUser:
		if result != ResultKindUser {
			return invalid(contract, "record type conflicts with user URL")
		}
	case pixiv.ReferenceKindUserBookmarks:
		if recordType != "user_bookmarks" && result != ResultKindArtwork {
			return invalid(contract, "record type conflicts with user bookmarks URL")
		}
	case pixiv.ReferenceKindArtworkSeries:
		if recordType != "series" && result != ResultKindArtwork {
			return invalid(contract, "record type conflicts with artwork series URL")
		}
	case pixiv.ReferenceKindNovelSeries:
		if recordType != "series" && result != ResultKindNovel {
			return invalid(contract, "record type conflicts with novel series URL")
		}
	default:
		return invalid(contract, "record URL kind is not supported")
	}
	return nil
}

func mergeRecordSubtype(selection *TypeSpec, recordType string, contract Contract) error {
	_, recordSubtype, ok := recordTypeMeaning(recordType)
	if !ok || recordSubtype == "" {
		return nil
	}
	if selection.All || selection.ResultKind != ResultKindArtwork {
		return invalid(contract, "record artwork subtype conflicts with the selected type")
	}
	if selection.Subtype != "" && selection.Subtype != recordSubtype {
		return invalid(contract, "record artwork subtype conflicts with the selected type")
	}
	selection.Subtype = recordSubtype
	return nil
}

func recordTypeMeaning(value string) (ResultKind, Subtype, bool) {
	switch value {
	case "artwork":
		return ResultKindArtwork, "", true
	case "illustration", "illust":
		return ResultKindArtwork, SubtypeIllust, true
	case "manga":
		return ResultKindArtwork, SubtypeManga, true
	case "ugoira":
		return ResultKindArtwork, SubtypeUgoira, true
	case "novel":
		return ResultKindNovel, "", true
	case "user":
		return ResultKindUser, "", true
	case "series":
		return "", "", true
	case "user_bookmarks":
		return "", "", true
	default:
		return "", "", false
	}
}

func resultKindForReference(kind pixiv.ReferenceKind) (ResultKind, bool) {
	switch kind {
	case pixiv.ReferenceKindArtwork, pixiv.ReferenceKindUserBookmarks, pixiv.ReferenceKindArtworkSeries:
		return ResultKindArtwork, true
	case pixiv.ReferenceKindNovel, pixiv.ReferenceKindNovelSeries:
		return ResultKindNovel, true
	case pixiv.ReferenceKindUser:
		return ResultKindUser, true
	default:
		return "", false
	}
}

func targetKindForReference(kind pixiv.ReferenceKind) (TargetKind, bool) {
	switch kind {
	case pixiv.ReferenceKindArtwork:
		return TargetKindArtwork, true
	case pixiv.ReferenceKindNovel:
		return TargetKindNovel, true
	case pixiv.ReferenceKindUser, pixiv.ReferenceKindUserBookmarks:
		return TargetKindUser, true
	case pixiv.ReferenceKindArtworkSeries, pixiv.ReferenceKindNovelSeries:
		return TargetKindSeries, true
	default:
		return "", false
	}
}

func targetKindForType(selection TypeSpec, contract Contract) (TargetKind, error) {
	fromReference, referenceOK := targetKindForReference(selection.BareReferenceKind)
	if selection.BareReferenceKind != "" && !referenceOK {
		return "", invalid(contract, "type contains an unsupported bare reference kind")
	}
	if selection.BareTargetKind != "" {
		switch selection.BareTargetKind {
		case TargetKindArtwork, TargetKindNovel, TargetKindUser, TargetKindSeries, TargetKindComment:
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
