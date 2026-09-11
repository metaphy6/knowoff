package media

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two-pixel synthetic files exercise the actual byte formats without shipping
// playable content. All images are generated solid colors.
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

func TestLoadPack_OnlyImageAndTextTypes(t *testing.T) {
	for _, typ := range []MediaType{"gif", "video", "unknown"} {
		for _, item := range []string{"nown", "card"} {
			t.Run(string(typ)+"/"+item, func(t *testing.T) {
				pack := buildGoldenPack(t)
				if item == "nown" {
					pack.Media[0].Type = typ
				} else {
					pack.Cards[0].Type = typ
				}
				dir := t.TempDir()
				writeFormatBundle(t, dir, pack)
				if _, err := LoadPack(dir, defaultDealing()); err == nil || !strings.Contains(err.Error(), "type") {
					t.Fatalf("unsupported %s type %q should fail, got %v", item, typ, err)
				}
				if cert := Certify(pack, defaultDealing(), defaultHand()); cert.Passed {
					t.Fatal("unsupported media type must fail certification")
				}
			})
		}
	}
}

func TestLoadPack_ImageBytesMustBeStaticWebP(t *testing.T) {
	for _, tc := range []struct {
		name, encoded string
		accepted      bool
	}{
		{"static WebP", staticWebP, true},
		{"GIF disguised as image", staticGIF, false},
		{"animated WebP disguised as image", animatedWebP, false},
		{"malformed image", base64.StdEncoding.EncodeToString([]byte("not an image")), false},
	} {
		for _, item := range []string{"nown", "card"} {
			t.Run(tc.name+"/"+item, func(t *testing.T) {
				pack := buildGoldenPack(t)
				data := imageBytes(t, tc.encoded)
				ref := ContentHash(data)
				pack.Assets = map[string][]byte{ref: data}
				if item == "nown" {
					pack.Media[0].Type, pack.Media[0].AssetRef = MediaTypeImage, ref
				} else {
					pack.Cards[0].Type, pack.Cards[0].AssetRef = MediaTypeImage, ref
				}
				dir := t.TempDir()
				writeFormatBundle(t, dir, pack)
				_, err := LoadPack(dir, defaultDealing())
				if (err == nil) != tc.accepted {
					t.Fatalf("accepted=%v, error=%v", tc.accepted, err)
				}
				if cert := Certify(pack, defaultDealing(), defaultHand()); cert.Passed != tc.accepted {
					t.Fatalf("certification accepted=%v, errors=%v", tc.accepted, cert.Errors)
				}
			})
		}
	}
}

func writeFormatBundle(t *testing.T, dir string, pack *Pack) {
	t.Helper()
	assetsDir := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for ref, data := range pack.Assets {
		if err := os.WriteFile(filepath.Join(assetsDir, ref), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pack.Manifest.Checksums["media.jsonl"] = hashFileContent(writeJSONL(t, filepath.Join(dir, "media.jsonl"), mediaItemsToRaw(pack.Media)))
	pack.Manifest.Checksums["cards.jsonl"] = hashFileContent(writeJSONL(t, filepath.Join(dir, "cards.jsonl"), cardItemsToRaw(pack.Cards)))
	raw, err := json.Marshal(pack.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
