package saving

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

// CLI 接受先前输出的资源身份；定位 URL 始终由绑定账号重新查询。
func saveReference(ctx context.Context, client *pixiv.Client, ref resourceReference, path string) error {
	if ref.Product != "pixiv" {
		return errors.New("pixiv:SaveResource: invalid_argument: invalid resource reference")
	}
	var identity struct {
		Kind    string `json:"k"`
		ID      int64  `json:"id"`
		Page    int    `json:"p"`
		Variant string `json:"v"`
	}
	if err := json.Unmarshal(ref.Payload, &identity); err != nil || identity.ID <= 0 || identity.Page < -1 {
		return errors.New("pixiv:SaveResource: invalid_argument: invalid resource reference")
	}
	var image pixiv.Image
	switch identity.Kind {
	case "artwork":
		art, err := client.Artwork(identity.ID).Get(ctx)
		if err != nil {
			return downloadError("SaveResource", err)
		}
		image = art.Cover
		if identity.Page >= 0 {
			if identity.Page >= len(art.Pages) {
				return errors.New("pixiv:SaveResource: malformed_upstream_response: resource metadata has no usable URL")
			}
			image = art.Pages[identity.Page].Original
		}
		if identity.Variant != "" && identity.Variant != "original" && identity.Variant != image.Variant() {
			image, err = image.Quality(identity.Variant)
			if err != nil {
				return downloadError("SaveResource", err)
			}
		}
	case "user_profile":
		user, err := client.User(identity.ID).Get(ctx)
		if err != nil {
			return downloadError("SaveResource", err)
		}
		image = user.ProfileImage
	case "novel_cover":
		novel, err := client.Novel(identity.ID).Get(ctx)
		if err != nil {
			return downloadError("SaveResource", err)
		}
		image = novel.Cover
		if identity.Variant != "" && identity.Variant != image.Variant() {
			image, err = image.Quality(identity.Variant)
			if err != nil {
				return downloadError("SaveResource", err)
			}
		}
	case "ugoira_archive":
		metadata, err := client.Artwork(identity.ID).UgoiraMetadata(ctx)
		if err != nil {
			return downloadError("SaveResource", err)
		}
		quality := identity.Variant
		if quality == "" {
			quality = string(SelectUgoiraArchive(metadata).Quality)
		}
		found := false
		for _, archive := range metadata.Archives {
			if string(archive.Quality) == quality {
				image = archive.Image
				found = true
				break
			}
		}
		if !found {
			return errors.New("pixiv:SaveResource: malformed_upstream_response: resource metadata has no usable URL")
		}
	case "stamp":
		stamps, err := client.Stamps(ctx)
		if err != nil {
			return downloadError("SaveResource", err)
		}
		found := false
		for _, stamp := range stamps {
			if stamp.ID == identity.ID {
				image = stamp.Image
				found = true
				break
			}
		}
		if !found {
			return errors.New("pixiv:SaveResource: malformed_upstream_response: resource metadata has no usable URL")
		}
	case "novel_image", "novel_file":
		_, err := client.Novel(identity.ID).Content(ctx)
		return downloadError("SaveResource", err)
	default:
		return errors.New("pixiv:SaveResource: invalid_argument: resource kind is unsupported")
	}
	if err := os.MkdirAll(filepath.Dir(path), downloadDirectoryMode); err != nil {
		return err
	}
	return downloadError("SaveResource", image.Save(ctx, path))
}

const resourceReferenceVersion = 1

// resourceReference 只保存已有输入的身份；定位 URL 始终由绑定账号重新查询。
type resourceReference struct {
	Text    string
	Product string `json:"p"`
	Payload []byte `json:"d"`
	Version int    `json:"v"`
}

func parseResourceReference(text string) (resourceReference, error) {
	var ref resourceReference
	raw, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		return ref, errors.New("download source is invalid")
	}
	if err := json.Unmarshal(raw, &ref); err != nil || ref.Version != resourceReferenceVersion || ref.Product == "" || len(ref.Payload) == 0 {
		return ref, errors.New("download source is invalid")
	}
	ref.Text = text
	return ref, nil
}
