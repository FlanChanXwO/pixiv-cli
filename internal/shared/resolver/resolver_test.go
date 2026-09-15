package resolver_test

import (
	"context"
	"errors"
	"testing"

	recordpkg "github.com/FlanChanXwO/pixiv-cli/internal/shared/record"
	"github.com/FlanChanXwO/pixiv-cli/internal/shared/resolver"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDetailURLAndExplicitIDKeepTargetAndResultKindsSeparate(t *testing.T) {
	contract := detailContract()

	got, err := resolver.Resolve(context.Background(), resolver.Input{
		Value: "https://www.pixiv.net/en/novel/show.php?id=42&ref=tracking",
		Type:  "novel",
	}, contract)
	require.NoError(t, err)
	assert.Equal(t, int64(42), got.ID)
	assert.Equal(t, resolver.TargetKindNovel, got.TargetKind)
	assert.Equal(t, resolver.ResultKindNovel, got.ResultKind)
	assert.Equal(t, pixiv.ReferenceKindNovel, got.ReferenceKind)
	assert.Equal(t, resolver.SourceURL, got.Source)

	got, err = resolver.Resolve(context.Background(), resolver.Input{Value: "43", Type: "user"}, contract)
	require.NoError(t, err)
	assert.Equal(t, int64(43), got.ID)
	assert.Equal(t, resolver.TargetKindUser, got.TargetKind)
	assert.Equal(t, resolver.ResultKindUser, got.ResultKind)
	assert.Equal(t, pixiv.ReferenceKindUser, got.ReferenceKind)
	assert.Equal(t, resolver.SourceID, got.Source)
}

func TestResolveTypedCommentIDKeepsCommentTargetKindWithoutURLReference(t *testing.T) {
	contract := resolver.Contract{
		Operation: "comment delete",
		Types: []resolver.TypeSpec{
			{Name: "comment", ResultKind: resolver.ResultKindComment, BareTargetKind: resolver.TargetKindComment},
		},
	}

	got, err := resolver.Resolve(context.Background(), resolver.Input{Value: "44", Type: "comment"}, contract)
	require.NoError(t, err)
	assert.Equal(t, int64(44), got.ID)
	assert.Equal(t, resolver.TargetKindComment, got.TargetKind)
	assert.Equal(t, resolver.ResultKindComment, got.ResultKind)
	assert.Empty(t, got.ReferenceKind)
	assert.Equal(t, resolver.SourceID, got.Source)
}

func TestResolveRejectsURLTypeConflictBeforeAnyProbe(t *testing.T) {
	contract := detailContract()
	called := false
	contract.BareID = &resolver.BareIDPolicy{
		Candidates: []string{"artwork", "novel"},
		Probe: func(context.Context, int64, resolver.TypeSpec) resolver.ProbeOutcome {
			called = true
			return resolver.ProbeOutcome{Status: resolver.ProbeFound}
		},
	}

	_, err := resolver.Resolve(context.Background(), resolver.Input{
		Value: "https://www.pixiv.net/artworks/42",
		Type:  "novel",
	}, contract)
	require.Error(t, err)
	assert.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	assert.False(t, called, "a URL conflict must not fall through to bare-ID probing")
}

func TestResolveBookmarkOwnerURLAllowsIndependentNovelSelectionButBookmarkURLDoesNot(t *testing.T) {
	contract := bookmarkListContract()

	got, err := resolver.Resolve(context.Background(), resolver.Input{
		Value: "https://www.pixiv.net/users/123",
		Type:  "novel",
	}, contract)
	require.NoError(t, err)
	assert.Equal(t, int64(123), got.ID)
	assert.Equal(t, resolver.TargetKindUser, got.TargetKind)
	assert.Equal(t, resolver.ResultKindNovel, got.ResultKind)
	assert.Equal(t, pixiv.ReferenceKindUser, got.ReferenceKind)

	_, err = resolver.Resolve(context.Background(), resolver.Input{
		Value: "https://www.pixiv.net/users/123/bookmarks/artworks",
		Type:  "novel",
	}, contract)
	require.Error(t, err)
	assert.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))

	_, err = resolver.Resolve(context.Background(), resolver.Input{
		Value: "https://www.pixiv.net/users/123/bookmarks/artworks",
		Type:  "all",
	}, contract)
	require.Error(t, err)
	assert.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))

	got, err = resolver.Resolve(context.Background(), resolver.Input{
		Record: parseRecord(t, `{"id":"123","type":"user","url":"https://www.pixiv.net/users/123"}`),
		Type:   "novel",
	}, contract)
	require.NoError(t, err)
	assert.Equal(t, int64(123), got.ID)
	assert.Equal(t, resolver.TargetKindUser, got.TargetKind)
	assert.Equal(t, resolver.ResultKindNovel, got.ResultKind)
	assert.Equal(t, pixiv.ReferenceKindUser, got.ReferenceKind)
	assert.Equal(t, resolver.SourceRecord, got.Source)

	got, err = resolver.Resolve(context.Background(), resolver.Input{
		Value: "https://www.pixiv.net/users/123",
		Type:  "all",
	}, contract)
	require.NoError(t, err)
	assert.True(t, got.All)
	assert.Empty(t, got.ResultKind)
}

func TestResolveRecordUsesURLIdentityAndMapsLegacyArtworkSubtype(t *testing.T) {
	contract := detailContract()
	record := parseRecord(t, `{
		"id":"42",
		"type":"ugoira",
		"url":"https://www.pixiv.net/artworks/42",
		"title":"kept as opaque record data"
	}`)

	got, err := resolver.Resolve(context.Background(), resolver.Input{Record: record, Type: "artwork"}, contract)
	require.NoError(t, err)
	assert.Equal(t, int64(42), got.ID)
	assert.Equal(t, resolver.TargetKindArtwork, got.TargetKind)
	assert.Equal(t, resolver.ResultKindArtwork, got.ResultKind)
	assert.Equal(t, resolver.SubtypeUgoira, got.Subtype)
	assert.Equal(t, pixiv.ReferenceKindArtwork, got.ReferenceKind)
	assert.Equal(t, resolver.SourceRecord, got.Source)
}

func TestResolveRecordRejectsIdentityAndNamespaceConflicts(t *testing.T) {
	contract := detailContract()

	for name, input := range map[string]resolver.Input{
		"id differs from URL":     {Record: parseRecord(t, `{"id":"43","type":"artwork","url":"https://www.pixiv.net/artworks/42"}`)},
		"type differs from URL":   {Record: parseRecord(t, `{"id":"42","type":"novel","url":"https://www.pixiv.net/artworks/42"}`)},
		"user URL is not artwork": {Record: parseRecord(t, `{"id":"42","type":"user","url":"https://www.pixiv.net/users/42"}`), Type: "artwork"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := resolver.Resolve(context.Background(), input, contract)
			require.Error(t, err)
			assert.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
		})
	}
}

func TestResolveRejectsAllWhenContractDoesNotDeclareIt(t *testing.T) {
	_, err := resolver.Resolve(context.Background(), resolver.Input{Value: "42", Type: "all"}, detailContract())
	require.Error(t, err)
	assert.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}

func TestResolveBareIDRequiresADeclaredTypeOrControlledProbe(t *testing.T) {
	contract := detailContract()
	contract.DefaultType = ""

	_, err := resolver.Resolve(context.Background(), resolver.Input{Value: "42"}, contract)
	require.Error(t, err)
	assert.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))

	got, err := resolver.Resolve(context.Background(), resolver.Input{Value: "42", Type: "novel"}, contract)
	require.NoError(t, err)
	assert.Equal(t, resolver.ResultKindNovel, got.ResultKind)

	var calls []string
	contract.BareID = &resolver.BareIDPolicy{
		Candidates: []string{"artwork", "novel"},
		Probe: func(_ context.Context, id int64, candidate resolver.TypeSpec) resolver.ProbeOutcome {
			assert.Equal(t, int64(42), id)
			calls = append(calls, candidate.Name)
			if candidate.Name == "novel" {
				return resolver.ProbeOutcome{Status: resolver.ProbeFound}
			}
			return resolver.ProbeOutcome{Status: resolver.ProbeNotFound}
		},
	}
	got, err = resolver.Resolve(context.Background(), resolver.Input{Value: "42"}, contract)
	require.NoError(t, err)
	assert.Equal(t, resolver.ResultKindNovel, got.ResultKind)
	assert.Equal(t, resolver.SourceBareIDProbe, got.Source)
	assert.Equal(t, []string{"artwork", "novel"}, calls)
}

func TestResolveBareIDProbeDoesNotFallbackAfterForbiddenOrNetworkFailure(t *testing.T) {
	for _, test := range []struct {
		name   string
		status resolver.ProbeStatus
		reason sdk.Reason
	}{
		{name: "forbidden", status: resolver.ProbeForbidden, reason: sdk.Forbidden},
		{name: "network", status: resolver.ProbeNetworkError, reason: sdk.UpstreamUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			contract := probeContract()
			calls := 0
			contract.BareID.Probe = func(context.Context, int64, resolver.TypeSpec) resolver.ProbeOutcome {
				calls++
				return resolver.ProbeOutcome{
					Status: test.status,
					Err:    sdk.NewError("pixiv", "probe", test.reason),
				}
			}

			_, err := resolver.Resolve(context.Background(), resolver.Input{Value: "42"}, contract)
			require.Error(t, err)
			assert.Equal(t, test.reason, sdk.ReasonOf(err))
			assert.Equal(t, 1, calls, "an unsafe probe result must stop before trying another namespace")
		})
	}
}

func TestResolveBareIDProbeRedactsUnclassifiedFailureByStatus(t *testing.T) {
	for _, test := range []struct {
		name   string
		status resolver.ProbeStatus
		reason sdk.Reason
	}{
		{name: "forbidden", status: resolver.ProbeForbidden, reason: sdk.Forbidden},
		{name: "network", status: resolver.ProbeNetworkError, reason: sdk.UpstreamUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			contract := probeContract()
			contract.BareID.Probe = func(context.Context, int64, resolver.TypeSpec) resolver.ProbeOutcome {
				return resolver.ProbeOutcome{
					Status: test.status,
					Err:    errors.New("private probe failure details"),
				}
			}

			_, err := resolver.Resolve(context.Background(), resolver.Input{Value: "42"}, contract)
			require.Error(t, err)
			assert.Equal(t, test.reason, sdk.ReasonOf(err))
			assert.NotContains(t, err.Error(), "private probe failure details")
		})
	}
}

func TestResolveBareIDProbeRejectsAmbiguousSuccess(t *testing.T) {
	contract := probeContract()
	calls := 0
	contract.BareID.Probe = func(context.Context, int64, resolver.TypeSpec) resolver.ProbeOutcome {
		calls++
		return resolver.ProbeOutcome{Status: resolver.ProbeFound}
	}

	_, err := resolver.Resolve(context.Background(), resolver.Input{Value: "42"}, contract)
	require.Error(t, err)
	assert.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	assert.Equal(t, 2, calls, "all frozen candidates must be checked before declaring ambiguity")
}

func TestResolveBareIDProbeReturnsNotFoundWhenEveryCandidateIsMissing(t *testing.T) {
	contract := probeContract()
	contract.BareID.Probe = func(context.Context, int64, resolver.TypeSpec) resolver.ProbeOutcome {
		return resolver.ProbeOutcome{Status: resolver.ProbeNotFound}
	}

	_, err := resolver.Resolve(context.Background(), resolver.Input{Value: "42"}, contract)
	require.Error(t, err)
	assert.Equal(t, sdk.NotFound, sdk.ReasonOf(err))
}

func TestResolveRejectsInvalidProbeOutcomeWithoutHidingItAsNotFound(t *testing.T) {
	contract := probeContract()
	contract.BareID.Probe = func(context.Context, int64, resolver.TypeSpec) resolver.ProbeOutcome {
		return resolver.ProbeOutcome{Status: resolver.ProbeIndeterminate, Err: errors.New("unclassified probe result")}
	}

	_, err := resolver.Resolve(context.Background(), resolver.Input{Value: "42"}, contract)
	require.Error(t, err)
	assert.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}

func detailContract() resolver.Contract {
	return resolver.Contract{
		Operation:   "detail",
		DefaultType: "artwork",
		Types: []resolver.TypeSpec{
			{Name: "artwork", ResultKind: resolver.ResultKindArtwork, BareReferenceKind: pixiv.ReferenceKindArtwork},
			{Name: "novel", ResultKind: resolver.ResultKindNovel, BareReferenceKind: pixiv.ReferenceKindNovel},
			{Name: "user", ResultKind: resolver.ResultKindUser, BareReferenceKind: pixiv.ReferenceKindUser},
		},
		URLKinds: map[pixiv.ReferenceKind]resolver.URLTypeRelation{
			pixiv.ReferenceKindArtwork: resolver.URLTypeMustMatchResult,
			pixiv.ReferenceKindNovel:   resolver.URLTypeMustMatchResult,
			pixiv.ReferenceKindUser:    resolver.URLTypeMustMatchResult,
		},
	}
}

func bookmarkListContract() resolver.Contract {
	return resolver.Contract{
		Operation:   "bookmark list",
		DefaultType: "artwork",
		Types: []resolver.TypeSpec{
			{Name: "artwork", ResultKind: resolver.ResultKindArtwork, BareReferenceKind: pixiv.ReferenceKindUser},
			{Name: "novel", ResultKind: resolver.ResultKindNovel, BareReferenceKind: pixiv.ReferenceKindUser},
			{Name: "all", All: true, BareReferenceKind: pixiv.ReferenceKindUser},
		},
		URLKinds: map[pixiv.ReferenceKind]resolver.URLTypeRelation{
			pixiv.ReferenceKindUser:          resolver.URLTypeIndependent,
			pixiv.ReferenceKindUserBookmarks: resolver.URLTypeMustMatchResult,
		},
	}
}

func probeContract() resolver.Contract {
	return resolver.Contract{
		Operation: "typed detail",
		Types: []resolver.TypeSpec{
			{Name: "artwork", ResultKind: resolver.ResultKindArtwork, BareReferenceKind: pixiv.ReferenceKindArtwork},
			{Name: "novel", ResultKind: resolver.ResultKindNovel, BareReferenceKind: pixiv.ReferenceKindNovel},
		},
		BareID: &resolver.BareIDPolicy{Candidates: []string{"artwork", "novel"}},
	}
}

func parseRecord(t *testing.T, raw string) *recordpkg.Record {
	t.Helper()
	parsed, err := recordpkg.ParseRecordJSON([]byte(raw))
	require.NoError(t, err)
	return &parsed
}
