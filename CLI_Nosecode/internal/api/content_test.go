package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestChatMessageConImagenSerializaImageURL(t *testing.T) {
	msg := ChatMessage{
		Role: "user",
		Content: []ContentPart{
			{Type: "text", Text: "mira"},
			{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/jpeg;base64,abc"}},
		},
	}
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, `"type":"image_url"`) ||
		!strings.Contains(got, `"image_url":{"url":"data:image/jpeg;base64,abc"}`) {
		t.Fatalf("json = %s", got)
	}
}
