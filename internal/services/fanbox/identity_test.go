package fanbox

import (
	"testing"
)

func TestParseIdentityMetadataHTML(t *testing.T) {
	document := []byte(`<html><head><meta name="metadata" content='{"context":{"user":{"userId":"800","name":"verified","creatorId":"artist","creatorStatus":"explicit","isCreator":true}}}'></head></html>`)
	identity, err := ParseIdentityMetadataHTML(document)
	if err != nil {
		t.Fatalf("ParseIdentityMetadataHTML() error = %v", err)
	}
	want := Identity{UserID: 800, DisplayName: "verified", CreatorID: "artist", CreatorStatus: "explicit", IsCreator: true}
	if identity != want {
		t.Fatalf("identity = %+v, want %+v", identity, want)
	}
}

func TestParseIdentityMetadataHTMLHandlesNumericUserIDAndNullCreator(t *testing.T) {
	document := []byte(`<meta name="metadata" content='{"context":{"user":{"userId":800,"name":"numeric","creatorId":null}} }'>`)
	identity, err := ParseIdentityMetadataHTML(document)
	if err != nil {
		t.Fatalf("ParseIdentityMetadataHTML() error = %v", err)
	}
	if identity.UserID != 800 || identity.DisplayName != "numeric" || identity.CreatorID != "" || identity.IsCreator {
		t.Fatalf("identity = %+v", identity)
	}
}

func TestParseIdentityMetadataHTMLRejectsInvalid(t *testing.T) {
	for name, document := range map[string]string{
		"no metadata":      `<html><head></head></html>`,
		"invalid json":     `<meta name="metadata" content="not json">`,
		"missing user id":  `<meta name="metadata" content='{"context":{"user":{"name":"n"}}}'>`,
		"missing name":     `<meta name="metadata" content='{"context":{"user":{"userId":"800"}}}'>`,
		"invalid user id":  `<meta name="metadata" content='{"context":{"user":{"userId":"-1","name":"n"}}}'>`,
		"trailing content": `<meta name="metadata" content='{"context":{"user":{"userId":"1","name":"n"}}} extra'>`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseIdentityMetadataHTML([]byte(document)); err == nil {
				t.Fatalf("ParseIdentityMetadataHTML() unexpectedly succeeded")
			}
		})
	}
}
