package pixiv_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
	"github.com/stretchr/testify/require"
)

const novelHTML = `<div class="novel-view"><h1 class="novel-title">Title</h1><div class="novel-caption">Caption</div><h2>Heading</h2><p class="noveltext">A<strong>B<em>C</em></strong><del>D</del><ruby>漢<rp>(</rp><rt>かん</rt><rp>)</rp></ruby><a href="https://www.pixiv.net/">E</a><span class="novelcustom">F</span><span class="other">G</span><br>H<script>hidden</script></p><figure class="novel-image"><span><img src="https://i.pximg.net/body.jpg"></span><figcaption>Picture</figcaption></figure><figure class="novel-file"><span><a data-file-url="https://i.pximg.net/file.pdf">file.pdf</a></span></figure><div class="novel-widget"><p>Future<script>hidden</script></p></div></div>`

func TestNovelContentBlocks(t *testing.T) {
	content, err := (&pixiv.Client{}).Novel(23).ParseContent([]byte(novelHTML))
	require.NoError(t, err)
	require.EqualValues(t, 23, content.NovelID)
	require.Equal(t, "Title", content.Title)
	require.Equal(t, "Caption", content.Caption)
	require.Len(t, content.Blocks, 5)
	require.Equal(t, []pixiv.NovelBlockFormat{pixiv.NovelBlockHeader, pixiv.NovelBlockParagraph, pixiv.NovelBlockImage, pixiv.NovelBlockFile, pixiv.NovelBlockUnknown}, []pixiv.NovelBlockFormat{content.Blocks[0].Format, content.Blocks[1].Format, content.Blocks[2].Format, content.Blocks[3].Format, content.Blocks[4].Format})
	p := content.Blocks[1]
	require.Equal(t, "ABCD漢EFG\nH", p.Text)
	require.Len(t, p.Marks, 7)
	styles := []pixiv.NovelMarkStyle{pixiv.NovelMarkStrong, pixiv.NovelMarkEmphasis, pixiv.NovelMarkDelete, pixiv.NovelMarkRuby, pixiv.NovelMarkLink, pixiv.NovelMarkCustom, pixiv.NovelMarkUnknown}
	for i, style := range styles {
		require.Equal(t, style, p.Marks[i].Style)
	}
	require.Equal(t, "BC", p.Marks[0].Text)
	require.Equal(t, &pixiv.NovelRuby{Text: "漢", Furigana: "かん"}, p.Marks[3].Ruby)
	require.Equal(t, "https://www.pixiv.net/", p.Marks[4].Href)
	require.Equal(t, "https://i.pximg.net/body.jpg", content.Blocks[2].Image.Image.URL)
	require.Equal(t, "file.pdf", content.Blocks[3].File.File.Filename)
	require.Equal(t, map[string]string{"text": "Future"}, content.Blocks[4].Unknown.Payload)
	data, err := json.Marshal(content)
	require.NoError(t, err)
	require.NotContains(t, string(data), "hidden")
	require.NotContains(t, string(data), "<script")
}

func TestNovelContentUnknownContainerPreservesMedia(t *testing.T) {
	for _, element := range []string{"div", "figure"} {
		raw := fmt.Sprintf(`<%s class="novel-widget">Fallback<p class="noveltext">Paragraph</p><figure class="novel-image"><img src="https://i.pximg.net/nested.jpg"></figure></%s>`, element, element)
		content, err := (&pixiv.Client{}).Novel(23).ParseContent([]byte(raw))
		require.NoError(t, err)
		require.Len(t, content.Blocks, 3)
		require.Equal(t, pixiv.NovelBlockUnknown, content.Blocks[0].Format)
		require.Equal(t, map[string]string{"text": "Fallback"}, content.Blocks[0].Unknown.Payload)
		require.Equal(t, "Paragraph", content.Blocks[1].Text)
		require.Equal(t, pixiv.NovelBlockImage, content.Blocks[2].Format)
		require.Equal(t, "https://i.pximg.net/nested.jpg", content.Blocks[2].Image.Image.URL)
	}
}

func TestNovelContentFailures(t *testing.T) {
	for _, tc := range []struct {
		body   string
		reason sdk.Reason
	}{
		{``, sdk.MalformedUpstreamResponse}, {`<div class="novel-view"></div>`, sdk.MalformedUpstreamResponse},
		{`<figure class="novel-image"></figure>`, sdk.MalformedUpstreamResponse},
		{`<figure class="novel-file"><a>file</a></figure>`, sdk.MalformedUpstreamResponse},
		{`<figure class="novel-image"><img src="https://evil.example/image"></figure>`, sdk.ResourceForbidden},
		{`<p class="noveltext">valid</p><figure class="novel-file"><a href="http://i.pximg.net/file">file</a></figure>`, sdk.ResourceForbidden},
		{`<div class="novel-widget"><figure class="novel-image"><img src="https://evil.example/image"></figure></div>`, sdk.ResourceForbidden},
	} {
		t.Run(tc.body, func(t *testing.T) {
			value, err := (&pixiv.Client{}).Novel(23).ParseContent([]byte(`<h1 class="title">Title</h1><div class="caption">Caption</div>` + tc.body))
			require.Equal(t, tc.reason, sdk.ReasonOf(err))
			require.Equal(t, pixiv.NovelContent{}, value)
		})
	}
	var unbound pixiv.NovelRef
	_, err := unbound.ParseContent([]byte(novelHTML))
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
}

func TestNovelContentMarkRangesAndRuby(t *testing.T) {
	raw := `<p class="noveltext">漢<strong>漢<em>漢</em></strong><ruby> <strong><a href="/target">漢</a></strong><rp>(</rp><rt>かん</rt><rp>)</rp> </ruby>漢</p>`
	content, err := (&pixiv.Client{}).Novel(23).ParseContent([]byte(raw))
	require.NoError(t, err)
	block := content.Blocks[0]
	require.Equal(t, "漢漢漢漢漢", block.Text)
	require.Equal(t, []pixiv.NovelMark{
		{Style: pixiv.NovelMarkStrong, Text: "漢漢", Start: 3, End: 9},
		{Style: pixiv.NovelMarkEmphasis, Text: "漢", Start: 6, End: 9},
		{Style: pixiv.NovelMarkRuby, Text: "漢", Start: 9, End: 12, Ruby: &pixiv.NovelRuby{Text: "漢", Furigana: "かん"}},
		{Style: pixiv.NovelMarkStrong, Text: "漢", Start: 9, End: 12},
		{Style: pixiv.NovelMarkLink, Text: "漢", Start: 9, End: 12, Href: "/target"},
	}, block.Marks)
	for _, mark := range block.Marks {
		require.Equal(t, mark.Text, block.Text[mark.Start:mark.End])
	}
}

func TestNovelContentMultipleMediaFailsWholeBody(t *testing.T) {
	for _, body := range []string{
		`<figure class="novel-image"><img src="https://i.pximg.net/first"><img src="https://i.pximg.net/second"></figure>`,
		`<figure class="novel-image"><img src="https://i.pximg.net/first"><img src="https://evil.example/second"></figure>`,
		`<figure class="novel-image"><img src="https://i.pximg.net/first"><img></figure>`,
		`<figure class="novel-image"><img src="https://i.pximg.net/first"><figure class="novel-file"><a>file</a></figure></figure>`,
		`<figure class="novel-file"><a href="https://i.pximg.net/first">first</a><a>second</a></figure>`,
	} {
		content, err := (&pixiv.Client{}).Novel(23).ParseContent([]byte(body))
		require.Equal(t, sdk.MalformedUpstreamResponse, sdk.ReasonOf(err))
		require.Equal(t, pixiv.NovelContent{}, content)
	}
}

func TestNovelContentMediaLinksAndIcons(t *testing.T) {
	for _, figure := range []string{
		`<figure class="novel-image"><a href="https://www.pixiv.net/"><img src="https://i.pximg.net/body.jpg"></a><figcaption>Picture <a href="/credits">credits</a></figcaption></figure>`,
		`<figure class="novel-file"><a href="https://i.pximg.net/file.pdf"><img src="https://s.pximg.net/icon.png">file.pdf</a><figcaption>File <a href="/credits">credits</a></figcaption></figure>`,
	} {
		content, err := (&pixiv.Client{}).Novel(23).ParseContent([]byte(figure))
		require.NoError(t, err)
		require.Len(t, content.Blocks, 1)
		if content.Blocks[0].Format == pixiv.NovelBlockImage {
			require.Equal(t, "https://i.pximg.net/body.jpg", content.Blocks[0].Image.Image.URL)
		} else {
			require.Equal(t, "https://i.pximg.net/file.pdf", content.Blocks[0].File.File.URL)
			require.Equal(t, "file.pdf", content.Blocks[0].File.File.Filename)
		}
	}
}

func TestNovelAttachmentSave(t *testing.T) {
	for _, mode := range []string{"normal", "upstream", "truncated"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Empty(t, r.Header.Get("Authorization"))
				require.Empty(t, r.Header.Get("Cookie"))
				if mode == "upstream" {
					w.WriteHeader(403)
				}
				if mode == "truncated" {
					w.Header().Set("Content-Length", "100")
				}
				fmt.Fprint(w, "attachment")
			}))
			defer server.Close()
			value := pixiv.NewCredentials("fixture-refresh")
			value.SetTokens("fixture-access", "fixture-refresh")
			value.UserID, value.ExpiresAt = 11, time.Now().Add(time.Hour)
			store, err := pixiv.NewMemoryCredentials(value)
			require.NoError(t, err)
			client, err := pixiv.New(store, pixiv.Options{HTTPClient: fixtureHTTP(server)})
			require.NoError(t, err)
			content, err := client.Novel(23).ParseContent([]byte(novelHTML))
			require.NoError(t, err)
			destination := filepath.Join(t.TempDir(), "file.pdf")
			require.NoError(t, os.WriteFile(destination, []byte("existing"), 0600))
			err = content.Blocks[3].File.File.Save(t.Context(), destination)
			expected := "existing"
			if mode == "normal" {
				require.NoError(t, err)
				expected = "attachment"
			} else {
				require.Error(t, err)
			}
			data, readErr := os.ReadFile(destination)
			require.NoError(t, readErr)
			require.Equal(t, expected, string(data))
		})
	}
}

func TestNovelContentDecodeClearsBinding(t *testing.T) {
	content, err := (&pixiv.Client{}).Novel(23).ParseContent([]byte(novelHTML))
	require.NoError(t, err)
	imageBlock, fileBlock := *content.Blocks[2].Image, *content.Blocks[3].File
	file := fileBlock.File
	encoded, err := json.Marshal(file)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &file))
	_, err = file.Open(t.Context())
	require.Equal(t, sdk.InvalidArgument, sdk.ReasonOf(err))
	block := content.Blocks[2]
	require.NoError(t, json.Unmarshal([]byte(`{"format":"paragraph"}`), &block))
	require.Nil(t, block.Image)
	require.NoError(t, json.Unmarshal([]byte(`{}`), &imageBlock))
	require.Empty(t, imageBlock.Image.URL)
	require.NoError(t, json.Unmarshal([]byte(`{}`), &fileBlock))
	require.Empty(t, fileBlock.File.URL)
	require.NoError(t, json.Unmarshal([]byte(`{"title":"decoded"}`), &content))
	require.Empty(t, content.Blocks)
}
