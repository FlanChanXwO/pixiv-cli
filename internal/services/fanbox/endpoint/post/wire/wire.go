// Package wire contains FANBOX post response decoding shared by the post
// endpoint families. It exposes only normalized results; wire structs remain
// private to this protocol boundary.
package wire

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/endpoint/post"
	"github.com/FlanChanXwO/pixiv-cli/internal/services/fanbox/protocol"
)

// DecodePostInfo decodes the post.info body.post envelope.
func DecodePostInfo(raw json.RawMessage) (post.Post, error) {
	var envelope postInfoDTO
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return post.Post{}, errors.New("decode FANBOX post.info response")
	}
	return mapPost(envelope.Post)
}

// DecodePage decodes a post-list body. Home/supporting endpoints pass
// acceptItems=true because those routes use body.items; creator/tagged lists
// use body.posts.
func DecodePage(raw json.RawMessage, acceptItems bool) (post.Page, error) {
	var envelope pageDTO
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return post.Page{}, errors.New("decode FANBOX post list response")
	}
	items := envelope.Posts
	if acceptItems && items == nil {
		items = envelope.Items
	}
	posts := make([]post.Post, 0, len(items))
	for _, item := range items {
		value, err := mapPost(item)
		if err != nil {
			return post.Page{}, err
		}
		posts = append(posts, value)
	}
	return post.Page{Posts: posts, NextURL: firstContinuation(envelope.PageURLs, envelope.NextURL)}, nil
}

type postInfoDTO struct {
	Post postDTO `json:"post"`
}

type pageDTO struct {
	Posts    []postDTO `json:"posts"`
	Items    []postDTO `json:"items"`
	NextURL  string    `json:"nextUrl"`
	PageURLs []string  `json:"pageUrls"`
}

type postDTO struct {
	ID                string       `json:"id"`
	Title             string       `json:"title"`
	PublishedDateTime string       `json:"publishedDatetime"`
	CreatorID         string       `json:"creatorId"`
	FeeRequired       int          `json:"feeRequired"`
	IsRestricted      bool         `json:"isRestricted"`
	IsPinned          bool         `json:"isPinned"`
	RestrictedFor     int          `json:"restrictedFor"`
	CommentCount      int          `json:"commentCount"`
	Body              *postBodyDTO `json:"body"`
}

type postBodyDTO struct {
	Text     string              `json:"text"`
	Files    *[]fileDTO          `json:"files"`
	Images   *[]imageDTO         `json:"images"`
	Blocks   *[]blockDTO         `json:"blocks"`
	ImageMap map[string]imageDTO `json:"imageMap"`
	FileMap  map[string]fileDTO  `json:"fileMap"`
}

type blockDTO struct {
	Type    string  `json:"type"`
	ImageID *string `json:"imageId"`
	FileID  *string `json:"fileId"`
}

type imageDTO struct {
	ID           string `json:"id"`
	Extension    string `json:"extension"`
	OriginalURL  string `json:"originalUrl"`
	ThumbnailURL string `json:"thumbnailUrl"`
}

type fileDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Extension string `json:"extension"`
	URL       string `json:"url"`
}

func mapPost(source postDTO) (post.Post, error) {
	if strings.TrimSpace(source.ID) == "" {
		return post.Post{}, errors.New("FANBOX post has no id")
	}
	value := post.Post{
		ID:                source.ID,
		Title:             source.Title,
		PublishedDateTime: source.PublishedDateTime,
		CreatorID:         source.CreatorID,
		FeeRequired:       source.FeeRequired,
		IsRestricted:      source.IsRestricted,
		IsPinned:          source.IsPinned,
		RestrictedFor:     source.RestrictedFor,
		CommentCount:      source.CommentCount,
	}
	if source.Body == nil {
		return value, nil
	}
	body, err := mapBody(*source.Body)
	if err != nil {
		return post.Post{}, err
	}
	value.Body = &body
	return value, nil
}

func mapBody(source postBodyDTO) (post.Body, error) {
	body := post.Body{Text: source.Text}
	switch {
	case source.Images != nil:
		body.Images = make([]post.Image, 0, len(*source.Images))
		body.Assets = make([]post.Asset, 0, len(*source.Images))
		for _, item := range *source.Images {
			image, asset, err := mapImage(item)
			if err != nil {
				return post.Body{}, err
			}
			body.Images = append(body.Images, image)
			body.Assets = append(body.Assets, asset)
		}
	case source.Files != nil:
		body.Files = make([]post.File, 0, len(*source.Files))
		body.Assets = make([]post.Asset, 0, len(*source.Files))
		for _, item := range *source.Files {
			file, asset, err := mapFile(item)
			if err != nil {
				return post.Body{}, err
			}
			body.Files = append(body.Files, file)
			body.Assets = append(body.Assets, asset)
		}
	case source.Blocks != nil:
		body.Blocks = make([]post.Block, 0, len(*source.Blocks))
		body.Assets = make([]post.Asset, 0, len(*source.Blocks))
		for _, item := range *source.Blocks {
			block := post.Block{Type: item.Type}
			if item.ImageID != nil {
				block.ImageID = *item.ImageID
				image, found := source.ImageMap[block.ImageID]
				if !found {
					return post.Body{}, errors.New("FANBOX blog block references a missing image")
				}
				_, asset, err := mapImage(image)
				if err != nil {
					return post.Body{}, err
				}
				body.Assets = append(body.Assets, asset)
			}
			if item.FileID != nil {
				block.FileID = *item.FileID
				file, found := source.FileMap[block.FileID]
				if !found {
					return post.Body{}, errors.New("FANBOX blog block references a missing file")
				}
				_, asset, err := mapFile(file)
				if err != nil {
					return post.Body{}, err
				}
				body.Assets = append(body.Assets, asset)
			}
			body.Blocks = append(body.Blocks, block)
		}
	}
	return body, nil
}

func mapImage(source imageDTO) (post.Image, post.Asset, error) {
	if strings.TrimSpace(source.ID) == "" {
		return post.Image{}, post.Asset{}, errors.New("FANBOX image has no id")
	}
	if err := protocol.ValidateMediaURL(source.OriginalURL); err != nil {
		return post.Image{}, post.Asset{}, err
	}
	if source.ThumbnailURL != "" {
		if err := protocol.ValidateMediaURL(source.ThumbnailURL); err != nil {
			return post.Image{}, post.Asset{}, err
		}
	}
	image := post.Image{ID: source.ID, Extension: source.Extension, OriginalURL: source.OriginalURL, ThumbnailURL: source.ThumbnailURL}
	asset := post.Asset{ID: source.ID, Kind: post.AssetKindImage, Extension: source.Extension, URL: source.OriginalURL, ThumbnailURL: source.ThumbnailURL}
	return image, asset, nil
}

func mapFile(source fileDTO) (post.File, post.Asset, error) {
	if strings.TrimSpace(source.ID) == "" {
		return post.File{}, post.Asset{}, errors.New("FANBOX file has no id")
	}
	if err := protocol.ValidateMediaURL(source.URL); err != nil {
		return post.File{}, post.Asset{}, err
	}
	file := post.File{ID: source.ID, Name: source.Name, Extension: source.Extension, URL: source.URL}
	asset := post.Asset{ID: source.ID, Kind: post.AssetKindFile, Name: source.Name, Extension: source.Extension, URL: source.URL}
	return file, asset, nil
}

func firstContinuation(pageURLs []string, nextURL string) string {
	if len(pageURLs) > 0 {
		return pageURLs[0]
	}
	return nextURL
}
