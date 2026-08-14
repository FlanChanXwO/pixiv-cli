package pixiv_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	pixivmcpserver "github.com/FlanChanXwO/pixiv-cli/internal/mcpserver/pixiv"
	downloader "github.com/FlanChanXwO/pixiv-cli/internal/media/downloader"
	"github.com/FlanChanXwO/pixiv-cli/sdk"
	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// download_random_from_recommendation tool 的 owner 契约：count 语义、默认值、上限与错误形状。
func TestDownloadRandomFromRecommendationUsesSDKAndPreservesCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recommended.jpg")
	if err := os.WriteFile(path, []byte("recommended"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloads := &fakeDownloads{artworks: []downloader.DownloadedArtwork{{IllustID: 77, Files: []downloader.DownloadedFile{{Path: path}}}}}
	var requests []pixiv.RecommendedArtworksRequest
	sdkClient := &fakeSDKClient{recommendedArtworks: func(_ context.Context, request pixiv.RecommendedArtworksRequest, _ int) (sdk.Page[pixiv.Artwork], error) {
		requests = append(requests, request)
		return sdk.Page[pixiv.Artwork]{Items: []pixiv.Artwork{testSDKIllust(77, "recommended", 1)}}, nil
	}}
	server := pixivmcpserver.NewWithSDK(&fakeAPI{}, downloads, testSDKPorts(t, sdkClient), pixivmcpserver.Account{})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": 1})
	if result.IsError || len(result.Content) != 1 || len(requests) != 1 || requests[0] != (pixiv.RecommendedArtworksRequest{}) || !slices.Equal(downloads.downloadIDs, []int64{77}) {
		t.Fatalf("result=%+v requests=%+v ids=%v", result, requests, downloads.downloadIDs)
	}
}

func TestDownloadRandomSDKOpenErrorPreservesBusinessErrorShape(t *testing.T) {
	var openCalls, managerFactoryCalls int
	downloads := &fakeDownloads{}
	server := pixivmcpserver.NewWithSDKDownloadFactory(downloads, func(*pixiv.Client) pixivmcpserver.DownloadManager {
		managerFactoryCalls++
		return downloads
	}, pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
			openCalls++
			return nil, errors.New("open sentinel")
		},
		Pooled: func(context.Context, pixivmcpserver.Account, func(context.Context, *pixiv.Client) (bool, error)) error {
			return nil
		},
	}, pixivmcpserver.Account{})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download_random_from_recommendation",
		Arguments: map[string]any{
			"count":    1,
			"delivery": "local_path",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "Could not retrieve recommendations: open sentinel"
	assertEmptyDownloadResult(t, result, "local_path", wantText)
	if openCalls != 1 || managerFactoryCalls != 0 || len(downloads.downloadIDs) != 0 {
		t.Fatalf("downstream calls: open=%d manager_factory=%d download_ids=%v", openCalls, managerFactoryCalls, downloads.downloadIDs)
	}
}

func TestDownloadRandomRecommendationErrorPreservesBusinessErrorShape(t *testing.T) {
	var openCalls, recommendationCalls, managerFactoryCalls int
	downloads := &fakeDownloads{}
	sdkClient := &fakeSDKClient{recommendedArtworks: func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error) {
		recommendationCalls++
		return sdk.Page[pixiv.Artwork]{}, errors.New("recommendation sentinel")
	}}
	wireClient := openWireClient(t, sdkClient)
	ports := pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
			openCalls++
			return wireClient, nil
		},
		Pooled: func(ctx context.Context, account pixivmcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, wireClient)
			return err
		},
	}
	server := pixivmcpserver.NewWithSDKDownloadFactory(downloads, func(*pixiv.Client) pixivmcpserver.DownloadManager {
		managerFactoryCalls++
		return downloads
	}, ports, pixivmcpserver.Account{})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download_random_from_recommendation",
		Arguments: map[string]any{
			"count":    1,
			"delivery": "local_path",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "Could not retrieve recommendations: pixiv:RecommendedArtworks: upstream_error"
	assertEmptyDownloadResult(t, result, "local_path", wantText)
	if openCalls != 1 || recommendationCalls != 1 || managerFactoryCalls != 0 || len(downloads.downloadIDs) != 0 {
		t.Fatalf("downstream calls: open=%d recommendation=%d manager_factory=%d download_ids=%v", openCalls, recommendationCalls, managerFactoryCalls, downloads.downloadIDs)
	}
}

func TestDownloadRandomEmptyRecommendationPreservesBusinessErrorShape(t *testing.T) {
	session, closeSession, probe := newDownloadRandomProbeSession(t, nil)
	defer closeSession()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download_random_from_recommendation",
		Arguments: map[string]any{
			"count":    1,
			"delivery": "local_path",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "Could not retrieve recommendations: the list is empty."
	assertEmptyDownloadResult(t, result, "local_path", wantText)
	if probe.openCalls != 1 || probe.recommendationCalls != 1 || probe.managerFactoryCalls != 0 || len(probe.downloads.downloadIDs) != 0 {
		t.Fatalf("downstream calls: open=%d recommendation=%d manager_factory=%d download_ids=%v", probe.openCalls, probe.recommendationCalls, probe.managerFactoryCalls, probe.downloads.downloadIDs)
	}
}

func TestDownloadRandomManagerErrorPreservesBusinessErrorShape(t *testing.T) {
	session, closeSession, probe := newDownloadRandomProbeSession(t, []int64{77})
	defer closeSession()
	probe.downloads.err = errors.New("download sentinel")

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download_random_from_recommendation",
		Arguments: map[string]any{
			"count":    1,
			"delivery": "local_path",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	const wantText = "Download failed: download sentinel"
	assertEmptyDownloadResult(t, result, "local_path", wantText)
	if probe.openCalls != 1 || probe.recommendationCalls != 1 || probe.managerFactoryCalls != 1 || probe.downloads.downloadCalls != 1 || !slices.Equal(probe.downloads.downloadIDs, []int64{77}) {
		t.Fatalf("downstream calls: open=%d recommendation=%d manager_factory=%d downloads=%d download_ids=%v", probe.openCalls, probe.recommendationCalls, probe.managerFactoryCalls, probe.downloads.downloadCalls, probe.downloads.downloadIDs)
	}
}

func TestDownloadRandomBuildErrorPreservesBusinessErrorShape(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.png")
	_, statErr := os.Stat(missing)
	if statErr == nil {
		t.Fatal("missing test file unexpectedly exists")
	}
	session, closeSession, probe := newDownloadRandomProbeSession(t, []int64{77})
	defer closeSession()
	probe.downloads.artworks = []downloader.DownloadedArtwork{{
		IllustID: 77,
		Files:    []downloader.DownloadedFile{{Path: missing}},
	}}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "download_random_from_recommendation",
		Arguments: map[string]any{
			"count":    1,
			"delivery": "local_path",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	wantText := "Could not build the download result: " + statErr.Error()
	assertEmptyDownloadResult(t, result, "local_path", wantText)
	if probe.openCalls != 1 || probe.recommendationCalls != 1 || probe.managerFactoryCalls != 1 || probe.downloads.downloadCalls != 1 || !slices.Equal(probe.downloads.downloadIDs, []int64{77}) {
		t.Fatalf("downstream calls: open=%d recommendation=%d manager_factory=%d downloads=%d download_ids=%v", probe.openCalls, probe.recommendationCalls, probe.managerFactoryCalls, probe.downloads.downloadCalls, probe.downloads.downloadIDs)
	}
}

func TestDownloadRandomRejectsExplicitZeroBeforeOpeningSDK(t *testing.T) {
	session, closeSession, probe := newDownloadRandomProbeSession(t, []int64{1, 2, 3, 4, 5})
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": 0})
	assertDownloadRandomCountError(t, result)
	assertNoDownloadRandomDownstream(t, probe)
}

func TestDownloadRandomCountErrorPreservesLocalPathDelivery(t *testing.T) {
	session, closeSession, probe := newDownloadRandomProbeSession(t, []int64{1, 2, 3, 4, 5})
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{
		"count":    0,
		"delivery": "local_path",
	})
	const wantText = "Error: count must be an integer from 1 to 20"
	assertEmptyDownloadResult(t, result, "local_path", wantText)
	assertNoDownloadRandomDownstream(t, probe)
}

func TestDownloadRandomInvalidDeliveryPrecedesCountValidation(t *testing.T) {
	session, closeSession, probe := newDownloadRandomProbeSession(t, []int64{1, 2, 3, 4, 5})
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{
		"count":    0,
		"delivery": "invalid-delivery",
	})
	const wantText = `Error: delivery supports only "local_path".`
	assertEmptyDownloadResult(t, result, "local_path", wantText)
	assertNoDownloadRandomDownstream(t, probe)
}

func TestDownloadRandomRejectsExplicitNegativeCountBeforeOpeningSDK(t *testing.T) {
	session, closeSession, probe := newDownloadRandomProbeSession(t, []int64{1, 2, 3, 4, 5})
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": -1})
	assertDownloadRandomCountError(t, result)
	assertNoDownloadRandomDownstream(t, probe)
}

func TestDownloadRandomRejectsCountAboveMaximumBeforeOpeningSDK(t *testing.T) {
	ids := make([]int64, 21)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	session, closeSession, probe := newDownloadRandomProbeSession(t, ids)
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": 21})
	assertDownloadRandomCountError(t, result)
	assertNoDownloadRandomDownstream(t, probe)
}

func TestDownloadRandomOmittedCountDefaultsToFive(t *testing.T) {
	recommendationIDs := []int64{1, 2, 3, 4, 5, 6}
	session, closeSession, probe := newDownloadRandomProbeSession(t, recommendationIDs)
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{})
	if result.IsError || probe.openCalls != 1 || probe.recommendationCalls != 1 || len(probe.downloads.downloadIDs) != 5 {
		t.Fatalf("result=%+v open=%d recommendation=%d download_ids=%v", result, probe.openCalls, probe.recommendationCalls, probe.downloads.downloadIDs)
	}
	seen := make(map[int64]struct{}, len(probe.downloads.downloadIDs))
	for _, id := range probe.downloads.downloadIDs {
		if !slices.Contains(recommendationIDs, id) {
			t.Fatalf("download ID %d is not from recommendations %v", id, recommendationIDs)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != 5 {
		t.Fatalf("download IDs are not unique: %v", probe.downloads.downloadIDs)
	}
}

func TestDownloadRandomNullCountDefaultsToFive(t *testing.T) {
	recommendationIDs := []int64{1, 2, 3, 4, 5, 6}
	session, closeSession, probe := newDownloadRandomProbeSession(t, recommendationIDs)
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": nil})
	if result.IsError || probe.openCalls != 1 || probe.recommendationCalls != 1 || len(probe.downloads.downloadIDs) != 5 {
		t.Fatalf("result=%+v open=%d recommendation=%d download_ids=%v", result, probe.openCalls, probe.recommendationCalls, probe.downloads.downloadIDs)
	}
}

func TestDownloadRandomToolSchemaDocumentsOptionalCountContract(t *testing.T) {
	session, closeSession, _ := newDownloadRandomProbeSession(t, nil)
	defer closeSession()

	var inputSchema any
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		if tool.Name == "download_random_from_recommendation" {
			inputSchema = tool.InputSchema
			break
		}
	}
	if inputSchema == nil {
		t.Fatal("download_random_from_recommendation tool not found")
	}
	raw, err := json.Marshal(inputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(schema.Required, "count") {
		t.Fatalf("count must remain optional: schema=%s", raw)
	}
	description := schema.Properties["count"].Description
	if !strings.Contains(description, "defaults to 5") || !strings.Contains(description, "1 to 20") {
		t.Fatalf("count schema does not document default/range: %s", raw)
	}
}

func TestDownloadRandomAcceptsMaximumCount(t *testing.T) {
	recommendationIDs := make([]int64, 21)
	for i := range recommendationIDs {
		recommendationIDs[i] = int64(i + 1)
	}
	session, closeSession, probe := newDownloadRandomProbeSession(t, recommendationIDs)
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": 20})
	if result.IsError || probe.openCalls != 1 || probe.recommendationCalls != 1 || len(probe.downloads.downloadIDs) != 20 {
		t.Fatalf("result=%+v open=%d recommendation=%d download_ids=%v", result, probe.openCalls, probe.recommendationCalls, probe.downloads.downloadIDs)
	}
	seen := make(map[int64]struct{}, len(probe.downloads.downloadIDs))
	for _, id := range probe.downloads.downloadIDs {
		if !slices.Contains(recommendationIDs, id) {
			t.Fatalf("download ID %d is not from recommendations %v", id, recommendationIDs)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != 20 {
		t.Fatalf("download IDs are not unique: %v", probe.downloads.downloadIDs)
	}
}

func TestDownloadRandomUsesAvailableRecommendationsWhenListIsShorter(t *testing.T) {
	recommendationIDs := []int64{11, 12, 13}
	session, closeSession, probe := newDownloadRandomProbeSession(t, recommendationIDs)
	defer closeSession()

	result := callTool(t, session, "download_random_from_recommendation", map[string]any{"count": 5})
	if result.IsError || probe.openCalls != 1 || probe.recommendationCalls != 1 || len(probe.downloads.downloadIDs) != len(recommendationIDs) {
		t.Fatalf("result=%+v open=%d recommendation=%d download_ids=%v", result, probe.openCalls, probe.recommendationCalls, probe.downloads.downloadIDs)
	}
	got := append([]int64(nil), probe.downloads.downloadIDs...)
	slices.Sort(got)
	if !slices.Equal(got, recommendationIDs) {
		t.Fatalf("download IDs=%v want available recommendations %v", got, recommendationIDs)
	}
}

func assertDownloadRandomCountError(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	const wantText = "Error: count must be an integer from 1 to 20"
	assertEmptyDownloadResult(t, result, "local_path", wantText)
}

func assertNoDownloadRandomDownstream(t *testing.T, probe *downloadRandomProbe) {
	t.Helper()
	if probe.openCalls != 0 || probe.recommendationCalls != 0 || probe.managerFactoryCalls != 0 || probe.downloads.downloadCalls != 0 || len(probe.downloads.downloadIDs) != 0 {
		t.Fatalf("downstream calls: open=%d recommendation=%d manager_factory=%d downloads=%d download_ids=%v", probe.openCalls, probe.recommendationCalls, probe.managerFactoryCalls, probe.downloads.downloadCalls, probe.downloads.downloadIDs)
	}
}

type downloadRandomProbe struct {
	openCalls           int
	recommendationCalls int
	managerFactoryCalls int
	downloads           *fakeDownloads
}

func newDownloadRandomProbeSession(t *testing.T, recommendationIDs []int64) (*mcp.ClientSession, func(), *downloadRandomProbe) {
	t.Helper()
	probe := &downloadRandomProbe{downloads: &fakeDownloads{}}
	sdkClient := &fakeSDKClient{recommendedArtworks: func(context.Context, pixiv.RecommendedArtworksRequest, int) (sdk.Page[pixiv.Artwork], error) {
		probe.recommendationCalls++
		illusts := make([]pixiv.Artwork, len(recommendationIDs))
		for i, id := range recommendationIDs {
			illusts[i] = testSDKIllust(id, "recommended", 1)
		}
		return sdk.Page[pixiv.Artwork]{Items: illusts}, nil
	}}
	wireClient := openWireClient(t, sdkClient)
	ports := pixivmcpserver.SDKPorts{
		Open: func(pixivmcpserver.Account) (*pixiv.Client, error) {
			probe.openCalls++
			return wireClient, nil
		},
		Pooled: func(ctx context.Context, account pixivmcpserver.Account, attempt func(context.Context, *pixiv.Client) (bool, error)) error {
			_, err := attempt(ctx, wireClient)
			return err
		},
	}
	server := pixivmcpserver.NewWithSDKDownloadFactory(probe.downloads, func(*pixiv.Client) pixivmcpserver.DownloadManager {
		probe.managerFactoryCalls++
		return probe.downloads
	}, ports, pixivmcpserver.Account{})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = server.Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	return session, func() {
		_ = session.Close()
		cancel()
	}, probe
}
