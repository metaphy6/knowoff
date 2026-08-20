package game

import (
	"strings"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/pkg/media"
)

func newTestPack() *media.Pack {
	return &media.Pack{
		Manifest: media.Manifest{PackTag: "test"},
		Media: []*media.MediaItem{
			{ID: "nown-image", Type: media.MediaTypeImage, AssetRef: "assets/img.webp", Embedding: []float32{1, 0}},
			{ID: "nown-text", Type: media.MediaTypeText, Content: "a very normal sentence", Embedding: []float32{0, 1}},
		},
		Assets: map[string][]byte{
			"assets/img.webp": {0x1, 0x2},
		},
	}
}

func newRenderer(pack *media.Pack) *PayloadRenderer {
	mgr := media.NewManager(pack)
	issuer := media.NewSignedURLIssuer([]byte("test-key"), 30*time.Second)
	return NewPayloadRenderer(mgr, issuer, "https://cdn.example.com")
}

func TestPayloadRenderer_NowerSeesImage(t *testing.T) {
	r := newRenderer(newTestPack())
	payload, err := r.NownPayload("round-1", "nown-image", ViewNower)
	if err != nil {
		t.Fatalf("render nower payload: %v", err)
	}

	if payload["decoy"] != nil {
		t.Fatal("nower payload must not be a decoy")
	}
	nown, ok := payload["nown"].(map[string]any)
	if !ok {
		t.Fatalf("expected nown map, got %T", payload["nown"])
	}
	if nown["id"] != "nown-image" {
		t.Fatalf("expected id nown-image, got %v", nown["id"])
	}
	if nown["type"] != string(media.MediaTypeImage) {
		t.Fatalf("expected type image, got %v", nown["type"])
	}
	url, ok := nown["signed_url"].(string)
	if !ok || url == "" {
		t.Fatalf("expected signed_url, got %v", nown["signed_url"])
	}
	if !strings.Contains(url, "token=") {
		t.Fatalf("signed_url must contain token query: %s", url)
	}
	if strings.Contains(url, "decoy") {
		t.Fatal("signed_url must not leak decoy marker")
	}
}

func TestPayloadRenderer_NowerSeesTextContent(t *testing.T) {
	r := newRenderer(newTestPack())
	payload, err := r.NownPayload("round-1", "nown-text", ViewNower)
	if err != nil {
		t.Fatalf("render nower payload: %v", err)
	}
	nown := payload["nown"].(map[string]any)
	if nown["content"] != "a very normal sentence" {
		t.Fatalf("expected text content, got %v", nown["content"])
	}
	if _, hasURL := nown["signed_url"]; hasURL {
		t.Fatal("text nown must not carry a signed_url")
	}
}

func TestPayloadRenderer_DecoyHasNoNown(t *testing.T) {
	r := newRenderer(newTestPack())
	for _, view := range []RecipientView{ViewDecoy} {
		payload, err := r.NownPayload("round-1", "nown-image", view)
		if err != nil {
			t.Fatalf("render decoy payload: %v", err)
		}
		if payload["decoy"] != true {
			t.Fatalf("expected decoy true, got %v", payload)
		}
		if payload["nown"] != nil {
			t.Fatalf("decoy payload must not contain nown, got %v", payload["nown"])
		}
		for _, forbidden := range []string{"id", "signed_url", "content", "type"} {
			if _, ok := payload[forbidden]; ok {
				t.Fatalf("decoy payload leaked field %q", forbidden)
			}
		}
	}
}

func TestPayloadRenderer_DecoyLeakScanner(t *testing.T) {
	// This test is the CI-enforced event-stream leak scanner in embryo: every
	// combination of media type and decoy view must produce only the decoy
	// marker and no Nown-identifying information.
	r := newRenderer(newTestPack())
	for _, nownID := range []string{"nown-image", "nown-text"} {
		payload, err := r.NownPayload("round-1", nownID, ViewDecoy)
		if err != nil {
			t.Fatalf("render decoy for %s: %v", nownID, err)
		}
		assertNoNownLeak(t, payload)
	}
}

func assertNoNownLeak(t *testing.T, payload map[string]any) {
	t.Helper()
	if payload["decoy"] != true {
		t.Fatalf("expected decoy marker, got %v", payload)
	}
	if payload["nown"] != nil {
		t.Fatalf("leaked nown map in decoy payload: %v", payload)
	}
	for _, key := range []string{"id", "signed_url", "content", "type"} {
		if _, ok := payload[key]; ok {
			t.Fatalf("leaked field %q in decoy payload", key)
		}
	}
}

func TestPayloadRenderer_MissingNownErrors(t *testing.T) {
	r := newRenderer(newTestPack())
	if _, err := r.NownPayload("round-1", "missing", ViewNower); err == nil {
		t.Fatal("expected error for missing nown")
	}
}

func TestPayloadRenderer_MissingMediaManager(t *testing.T) {
	r := NewPayloadRenderer(nil, media.NewSignedURLIssuer([]byte("k"), time.Second), "")
	if _, err := r.NownPayload("round-1", "x", ViewNower); err == nil {
		t.Fatal("expected error when media manager is nil")
	}
}
