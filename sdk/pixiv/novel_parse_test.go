package pixiv

import (
	"strings"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func TestParseNovelContentStructured(t *testing.T) {
	html := `<html><body><div class="novel-view">
<h1 class="novel-title">Sample Title</h1>
<div class="novel-caption"><p>a caption</p></div>
<div class="novel-body">
<p class="noveltext">First <strong>bold</strong> and <em>italic</em> text.</p>
<h2 class="noveltitle">Chapter One</h2>
<p class="noveltext">Ruby <ruby>漢字<rp>(</rp><rt>かんじ</rt><rp>)</rp></ruby> here.</p>
<figure class="novelimage" data-resource-id="1"><img src="https://i.pximg.net/img/1.jpg"></figure>
<figure class="novelfile"><a href="https://i.pximg.net/files/1.zip">archive.zip</a></figure>
</div>
</div></body></html>`
	client, _ := New("token")
	content, err := client.parseNovelContent(101, []byte(html))
	if err != nil {
		t.Fatalf("parseNovelContent: %v", err)
	}
	if content.NovelID != 101 || content.Title != "Sample Title" {
		t.Fatalf("novel = %+v", content)
	}
	if len(content.Blocks) != 5 {
		t.Fatalf("blocks = %d, want 5", len(content.Blocks))
	}
	first := content.Blocks[0]
	if first.Kind != NovelBlockParagraph || !strings.Contains(first.Text, "bold") {
		t.Fatalf("first block = %+v", first)
	}
	hasStrong := false
	for _, mark := range first.Marks {
		if mark.Kind == NovelMarkStrong && mark.Text == "bold" {
			hasStrong = true
		}
	}
	if !hasStrong {
		t.Fatalf("expected strong mark, got %+v", first.Marks)
	}
	header := content.Blocks[1]
	if header.Kind != NovelBlockHeader || header.Text != "Chapter One" {
		t.Fatalf("header = %+v", header)
	}
	rubyBlock := content.Blocks[2]
	hasRuby := false
	for _, mark := range rubyBlock.Marks {
		if mark.Kind == NovelMarkRuby && mark.Ruby != nil && mark.Ruby.Furigana == "かんじ" {
			hasRuby = true
		}
	}
	if !hasRuby {
		t.Fatalf("expected ruby mark, got %+v", rubyBlock.Marks)
	}
	image := content.Blocks[3]
	if image.Kind != NovelBlockImage || image.Image == nil {
		t.Fatalf("image block = %+v", image)
	}
	file := content.Blocks[4]
	if file.Kind != NovelBlockFile || file.File == nil {
		t.Fatalf("file block = %+v", file)
	}
}

func TestParseNovelContentUnknownBlockPreserved(t *testing.T) {
	html := `<html><body><div class="novel-view"><div class="novel-body">
<p class="noveltext">known text</p>
<div class="novel_something">unknown block payload</div>
</div></div></body></html>`
	client, _ := New("token")
	content, err := client.parseNovelContent(1, []byte(html))
	if err != nil {
		t.Fatalf("parseNovelContent: %v", err)
	}
	if len(content.Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(content.Blocks))
	}
	unknown := content.Blocks[1]
	if unknown.Kind != NovelBlockUnknown || unknown.Unknown == nil || unknown.Unknown.Payload["text"] == "" {
		t.Fatalf("unknown block = %+v", unknown)
	}
}

func TestParseNovelContentEmptyFails(t *testing.T) {
	client, _ := New("token")
	if _, err := client.parseNovelContent(1, []byte("<html></html>")); sdk.CodeOf(err) != sdk.CodeMalformedUpstreamResponse {
		t.Fatalf("expected CodeMalformedUpstreamResponse, got %v", err)
	}
}
