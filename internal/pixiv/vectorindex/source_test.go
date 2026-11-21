package vectorindex_test

import (
	"encoding/binary"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/internal/pixiv/vectorindex"
	"github.com/FlanChanXwO/pixiv-cli/internal/vectormodel"
	"github.com/stretchr/testify/require"
)

func TestQueryImageDetectionAndContentValidation(t *testing.T) {
	t.Chdir(t.TempDir())
	picture, err := vectormodel.ProbeImage()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile("cat", []byte("ordinary document"), 0600))
	require.NoError(t, os.WriteFile("picture", picture, 0600))
	for _, tc := range []struct {
		input         string
		image, failed bool
	}{
		{"a rainy city", false, false}, {"cat", false, false}, {"picture", true, false},
		{strings.Repeat("蓝色裙子", 40), false, false},
		{"./cat", false, true}, {"./missing.png", false, true}, {"https://example.org/query", true, false},
	} {
		image, err := vectorindex.ImageSource(tc.input)
		require.Equal(t, tc.image, image, tc.input)
		require.Equal(t, tc.failed, err != nil, tc.input)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/image" {
			w.Write(picture)
		} else {
			w.Write([]byte("not an image"))
		}
	}))
	defer server.Close()
	data, err := vectorindex.LoadImage(t.Context(), server.Client(), server.URL+"/image")
	require.NoError(t, err)
	require.Equal(t, picture, data)
	_, err = vectorindex.LoadImage(t.Context(), server.Client(), server.URL+"/invalid")
	require.Error(t, err)
	_, err = vectorindex.LoadImage(t.Context(), server.Client(), "./cat")
	require.Error(t, err)
}

func TestImageAllocationBudgetRejectsBeforeDecode(t *testing.T) {
	t.Chdir(t.TempDir())
	picture, err := vectormodel.ProbeImage()
	require.NoError(t, err)
	binary.BigEndian.PutUint32(picture[16:20], 100000)
	binary.BigEndian.PutUint32(picture[20:24], 100000)
	binary.BigEndian.PutUint32(picture[29:33], crc32.ChecksumIEEE(picture[12:29]))
	require.NoError(t, os.WriteFile("picture", picture, 0600))
	_, err = vectorindex.ImageSource("picture")
	require.Error(t, err)
	_, err = vectorindex.LoadImage(t.Context(), http.DefaultClient, "picture")
	require.ErrorContains(t, err, "allocation budget")
	picture, err = vectormodel.ProbeImage()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile("oversized.png", picture, 0600))
	require.NoError(t, os.Truncate("oversized.png", 512*1024*1024+1))
	_, err = vectorindex.ImageSource("oversized.png")
	require.Error(t, err)
	_, err = vectorindex.LoadImage(t.Context(), http.DefaultClient, "oversized.png")
	require.ErrorContains(t, err, "source limit")
}
