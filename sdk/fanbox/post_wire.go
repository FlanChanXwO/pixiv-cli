package fanbox

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

type postWire struct {
	ID                string          `json:"id"`
	Title             string          `json:"title"`
	PublishedDateTime string          `json:"publishedDatetime"`
	CreatorID         string          `json:"creatorId"`
	FeeRequired       int             `json:"feeRequired"`
	IsRestricted      bool            `json:"isRestricted"`
	IsPinned          bool            `json:"isPinned"`
	RestrictedFor     int             `json:"restrictedFor"`
	CommentCount      int             `json:"commentCount"`
	CoverImageURL     string          `json:"coverImageUrl"`
	Body              json.RawMessage `json:"body"`
}

func (c *Client) mapPost(operation string, source postWire) (*Post, error) {
	id := strings.TrimSpace(source.ID)
	if id == "" {
		return nil, newError(operation, sdk.MalformedUpstreamResponse, "FANBOX post is missing an ID")
	}
	published, err := time.Parse(time.RFC3339, source.PublishedDateTime)
	if err != nil {
		return nil, newError(operation, sdk.MalformedUpstreamResponse, "invalid FANBOX post publish time")
	}
	data := postData{Title: source.Title, PublishedAt: published.UTC(), FeeRequired: source.FeeRequired, IsRestricted: source.IsRestricted, IsPinned: source.IsPinned, RestrictedFor: source.RestrictedFor, CommentCount: source.CommentCount}
	if creatorID := strings.TrimSpace(source.CreatorID); creatorID != "" {
		data.Creator = &Creator{CreatorRef: c.Creator(creatorID)}
	}
	data.Cover, err = c.image(operation, source.CoverImageURL)
	if err != nil {
		return nil, err
	}
	if len(source.Body) != 0 && strings.TrimSpace(string(source.Body)) != "null" {
		data.Body, err = c.mapPostBody(operation, source.Body)
		if err != nil {
			return nil, err
		}
	}
	ref := c.Post(id)
	ref.CreatorID = strings.TrimSpace(source.CreatorID)
	return &Post{PostRef: ref, postData: data}, nil
}

type postBodyWire struct {
	Text     string               `json:"text"`
	Images   *[]imageWire         `json:"images"`
	Files    *[]fileWire          `json:"files"`
	Blocks   []json.RawMessage    `json:"blocks"`
	ImageMap map[string]imageWire `json:"imageMap"`
	FileMap  map[string]fileWire  `json:"fileMap"`
}

type imageWire struct {
	ID           string `json:"id"`
	Extension    string `json:"extension"`
	OriginalURL  string `json:"originalUrl"`
	ThumbnailURL string `json:"thumbnailUrl"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
}

type fileWire struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Extension string `json:"extension"`
	URL       string `json:"url"`
}

type blockWire struct {
	Type    BlockStyle `json:"type"`
	Text    string     `json:"text"`
	Caption string     `json:"caption"`
	ImageID *string    `json:"imageId"`
	FileID  *string    `json:"fileId"`
}

func (c *Client) mapPostBody(operation string, raw json.RawMessage) (*PostBody, error) {
	var source postBodyWire
	if json.Unmarshal(raw, &source) != nil {
		return nil, newError(operation, sdk.MalformedUpstreamResponse, "invalid FANBOX post body")
	}
	blocks := make([]blockWire, len(source.Blocks))
	for i, rawBlock := range source.Blocks {
		if strings.TrimSpace(string(rawBlock)) == "null" || json.Unmarshal(rawBlock, &blocks[i]) != nil {
			return nil, newError(operation, sdk.MalformedUpstreamResponse, "invalid FANBOX post block")
		}
	}
	images, files, err := bodyMedia(operation, source, blocks)
	if err != nil {
		return nil, err
	}
	body := &PostBody{Text: source.Text, Images: make([]Image, len(images)), Files: make([]File, len(files)), Blocks: make([]PostBlock, len(blocks)), Payload: raw}
	imageByID := make(map[string]*Image, len(images))
	for i, image := range images {
		body.Images[i], err = c.postImage(operation, image)
		if err != nil {
			return nil, err
		}
		imageByID[image.ID] = &body.Images[i]
	}
	fileByID := make(map[string]*File, len(files))
	for i, file := range files {
		body.Files[i], err = c.postFile(operation, file)
		if err != nil {
			return nil, err
		}
		fileByID[file.ID] = &body.Files[i]
	}
	for i, block := range blocks {
		out := PostBlock{Style: block.Type, Text: block.Text, Caption: block.Caption, Payload: source.Blocks[i]}
		if block.ImageID != nil {
			out.Image = imageByID[*block.ImageID]
		}
		if block.FileID != nil {
			out.File = fileByID[*block.FileID]
		}
		body.Blocks[i] = out
	}
	return body, nil
}

func bodyMedia(operation string, source postBodyWire, blocks []blockWire) ([]imageWire, []fileWire, error) {
	if source.Images != nil {
		return *source.Images, nil, nil
	}
	if source.Files != nil {
		return nil, *source.Files, nil
	}
	var images []imageWire
	var files []fileWire
	seenImages, seenFiles := map[string]bool{}, map[string]bool{}
	for _, block := range blocks {
		if block.ImageID != nil && !seenImages[*block.ImageID] {
			image, ok := source.ImageMap[*block.ImageID]
			if !ok || image.ID != *block.ImageID {
				return nil, nil, newError(operation, sdk.MalformedUpstreamResponse, "FANBOX block references a missing or mismatched image")
			}
			seenImages[*block.ImageID] = true
			images = append(images, image)
		}
		if block.FileID != nil && !seenFiles[*block.FileID] {
			file, ok := source.FileMap[*block.FileID]
			if !ok || file.ID != *block.FileID {
				return nil, nil, newError(operation, sdk.MalformedUpstreamResponse, "FANBOX block references a missing or mismatched file")
			}
			seenFiles[*block.FileID] = true
			files = append(files, file)
		}
	}
	return images, files, nil
}
