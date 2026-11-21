package vectorindex_test

import (
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
