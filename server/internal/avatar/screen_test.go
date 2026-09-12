package avatar

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"github.com/knowoff/knowoff/server/internal/config"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type imageTransport func(*http.Request) (*http.Response, error)

func (f imageTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestImageScreeningFailClosedProviderContract(t *testing.T) {
	image, err := normalizeAvatar(bytes.NewReader(avatarPNG(t)))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, body string
		status     int
		want       error
	}{
		{"approved", `{"results":[{"flagged":false}]}`, 200, nil},
		{"flagged", `{"results":[{"flagged":true}]}`, 200, ErrFlagged},
		{"missing", `{"results":[{}]}`, 200, ErrUnavailable},
		{"null", `{"results":[{"flagged":null}]}`, 200, ErrUnavailable},
		{"many", `{"results":[{"flagged":false},{"flagged":false}]}`, 200, ErrUnavailable},
		{"empty", `{"results":[]}`, 200, ErrUnavailable},
		{"malformed", `{"results":`, 200, ErrUnavailable},
		{"oversized", strings.Repeat(" ", (1<<20)+1), 200, ErrUnavailable},
		{"redirect", `{"results":[{"flagged":false}]}`, 307, ErrUnavailable},
		{"provider_error", `sensitive provider details`, 503, ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: imageTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://api.openai.com/v1/moderations" || r.Method != http.MethodPost {
					t.Fatal("unexpected destination")
				}
				if r.Header.Get("Authorization") != "Bearer fixture-key" {
					t.Fatal("missing provider credential")
				}
				var request struct {
					Model string `json:"model"`
					Input []struct {
						Type  string `json:"type"`
						Image struct {
							URL string `json:"url"`
						} `json:"image_url"`
					} `json:"input"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				if request.Model != "omni-moderation-latest" || len(request.Input) != 1 || request.Input[0].Type != "image_url" {
					t.Fatal("not image moderation")
				}
				raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(request.Input[0].Image.URL, "data:image/webp;base64,"))
				if err != nil || !bytes.Equal(raw, image) {
					t.Fatal("different pixels sent")
				}
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			if err := imageScreen(client, "omni-moderation-latest", "fixture-key")(t.Context(), image); !errors.Is(err, tc.want) {
				t.Fatalf("error=%v wanted=%v", err, tc.want)
			}
		})
	}
}
func TestImageScreeningTimeoutAndInvalidConfiguration(t *testing.T) {
	for _, c := range []config.ContentScreeningConfig{{}, {Provider: "disabled"}, {Provider: "openai", Model: "omni-moderation-latest"}, {Provider: "openai", Model: "text-moderation-latest", APIKey: "test"}, {Provider: "openai", Model: "omni-moderation-latest", APIKey: "test", TimeoutS: 31}} {
		if newImageScreen(c) != nil {
			t.Fatal("invalid config enabled")
		}
	}
	client := &http.Client{Timeout: 20 * time.Millisecond, Transport: imageTransport(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })}
	if err := imageScreen(client, "omni-moderation-latest", "test")(context.Background(), []byte("pixels")); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
func TestAvatarDimensionPreflightRejectsDecompressionBomb(t *testing.T) {
	data := avatarPNG(t)
	// Preserve a valid PNG IHDR while advertising a gigantic raster. No IDAT is
	// decoded: the preflight rejects dimensions before any large allocation.
	binary.BigEndian.PutUint32(data[16:20], 0x7fffffff)
	binary.BigEndian.PutUint32(data[20:24], 0x7fffffff)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	if _, err := normalizeAvatar(bytes.NewReader(data)); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{nil, []byte("<svg/>"), bytes.Repeat([]byte{'x'}, maxAvatarBytes+1)} {
		if _, err := normalizeAvatar(bytes.NewReader(raw)); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
}

func TestAvatarNormalizationCentersCropAndRemovesActualEXIF(t *testing.T) {
	source, err := png.Decode(bytes.NewReader(avatarPNG(t)))
	if err != nil {
		t.Fatal(err)
	}
	var jpegBytes bytes.Buffer
	if err := jpeg.Encode(&jpegBytes, source, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	metadata := []byte("Exif\x00\x00private-gps-fixture")
	original := jpegBytes.Bytes()
	withEXIF := append([]byte{}, original[:2]...)
	withEXIF = append(withEXIF, 0xff, 0xe1, byte((len(metadata)+2)>>8), byte(len(metadata)+2))
	withEXIF = append(withEXIF, metadata...)
	withEXIF = append(withEXIF, original[2:]...)
	normalized, err := normalizeAvatar(bytes.NewReader(withEXIF))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(normalized, []byte("Exif")) || bytes.Contains(normalized, metadata) {
		t.Fatal("actual metadata survived re-encode")
	}
	decoded, _, err := image.Decode(bytes.NewReader(normalized))
	if err != nil {
		t.Fatal(err)
	}
	red, _, _, alpha := decoded.At(0, 100).RGBA()
	// The 400x200 input has red=x; centered square begins around x=100,
	// while a full-image resize begins around x=0. Allow lossy JPEG/WebP noise.
	if red/257 < 70 || red/257 > 130 || alpha != 65535 {
		t.Fatalf("crop did not retain center: red=%d alpha=%d", red/257, alpha)
	}
}
