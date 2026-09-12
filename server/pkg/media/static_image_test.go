package media

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

// Historical synthetic bytes are retained unchanged as unsupported inputs.
const staticWebP = "UklGRhwAAABXRUJQVlA4TA8AAAAvAUAAAAcQ/Y/+ByKi/wEA"
const animatedWebP = "UklGRoQAAABXRUJQVlA4WAoAAAACAAAAAQAAAQAAQU5JTQYAAAAAAAAAAABBTk1GKAAAAAAAAAAAAAEAAAEAAGQAAAJWUDhMDwAAAC8BQAAABxD9j/4HIqL/AQBBTk1GKAAAAAAAAAAAAAEAAAEAAGQAAABWUDhMDwAAAC8BQAAABxDR//4HIqL/AQA="
const staticGIF = "R0lGODlhAgACAIEAAP8AAAAAAAAAAAAAACH/C05FVFNDQVBFMi4wAwEAAAAh+QQACgAAACwAAAAAAgACAAAIBgABCAQQEAA7"

func imageBytes(t *testing.T, encoded string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestPlayableContentIsTextOnlyForNownsAndCards(t *testing.T) {
	for _, typ := range []string{"image", "gif", "video", "webp", "unknown"} {
		for _, item := range []string{"nown", "card"} {
			t.Run(typ+"/"+item, func(t *testing.T) {
				b := textFixtureBundle(t, "en")
				if item == "nown" {
					b.Nowns[0].Type = typ
				} else {
					b.Cards[0].Type = typ
				}
				if _, err := SealTextBundle(b, textFixtureLimits()); err == nil {
					t.Fatal("nontext playable type accepted")
				}
			})
		}
	}
}

func TestBinaryAssetsCannotEnterTextBundles(t *testing.T) {
	for _, encoded := range []string{staticWebP, animatedWebP, staticGIF, base64.StdEncoding.EncodeToString([]byte("not an image"))} {
		data := imageBytes(t, encoded)
		b := textFixtureBundle(t, "en")
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		var object map[string]any
		if err := json.Unmarshal(raw, &object); err != nil {
			t.Fatal(err)
		}
		object["assets"] = map[string]string{ContentHash(data): base64.StdEncoding.EncodeToString(data)}
		raw, err = json.Marshal(object)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeTextBundle(raw, textFixtureLimits()); err == nil {
			t.Fatal("playable binary asset entered strict text bundle")
		}
	}
}
