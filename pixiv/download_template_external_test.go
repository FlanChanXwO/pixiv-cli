package pixiv_test

import (
	"context"
	"testing"

	"github.com/FlanChanXwO/pixiv-cli/pixiv"
)

func TestDownloadRejectsUnknownAndUnbalancedFilenameTemplateBeforeNetwork(t *testing.T) {
	for _, template := range []string{"{id}-{unknown}", "{id", "id}"} {
		if got := validateTemplateThroughDownload(t, template); got == nil {
			t.Fatalf("template %q unexpectedly succeeded", template)
		}
	}
}

func validateTemplateThroughDownload(t *testing.T, template string) error {
	t.Helper()
	client, err := pixiv.NewClient(pixiv.NewClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.DownloadAllWith(context.Background(), []string{"123"}, pixiv.DownloadOptions{FilenameTemplate: template})
	return err
}
