package media

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func newDeterministicRng(t *testing.T, seed int64) *rand.Rand {
	t.Helper()
	return rand.New(rand.NewSource(seed))
}

// buildGoldenPack returns a small text-only pack with full band coverage.
func buildGoldenPack(t *testing.T) *Pack {
	t.Helper()
	const dim = 8
	media := []*MediaItem{
		makeTextMedia("nown-1", "A cat wearing a tiny hat", []float32{1, 0, 0, 0, 0, 0, 0, 0}),
		makeTextMedia("nown-2", "A dog on a skateboard", []float32{0, 1, 0, 0, 0, 0, 0, 0}),
		makeTextMedia("nown-3", "A slice of pizza dancing", []float32{0, 0, 1, 0, 0, 0, 0, 0}),
	}
	// High cards share the same unit vector as a Nown.
	// Distant cards share a 0.4 cosine vector.
	// Chaos cards are orthogonal.
	var cards []*CardItem
	for i, n := range media {
		for j := 0; j < 14; j++ {
			cards = append(cards, makeTextCard(fmt.Sprintf("card-h-%d-%d", i, j), fmt.Sprintf("High match %d-%d", i, j), clone(n.Embedding)))
		}
		for j := 0; j < 14; j++ {
			cards = append(cards, makeTextCard(fmt.Sprintf("card-d-%d-%d", i, j), fmt.Sprintf("Distant match %d-%d", i, j), distantVector(dim, i)))
		}
	}
	for j := 0; j < 40; j++ {
		cards = append(cards, makeTextCard(fmt.Sprintf("card-c-%d", j), fmt.Sprintf("Chaos %d", j), randomUnitVector(dim*3+j)))
	}

	pack := &Pack{
		Manifest: Manifest{
			PackTag:          "test-golden",
			FormatVersion:    1,
			Language:         "en",
			EmbeddingModel:   "synthetic",
			EmbeddingVersion: "1.0",
			AgeRating:        "everyone",
			CreatedAt:        "2026-08-20T00:00:00Z",
			Attribution:      nil,
			Checksums: map[string]string{
				"manifest.json": "",
				"media.jsonl":   "",
				"cards.jsonl":   "",
			},
		},
		Media: media,
		Cards: cards,
	}
	pack.Candidates = BuildCandidates(media, cards, defaultDealing())
	return pack
}

// buildBandStarvedPack has Nowns with almost no distant candidates.
func buildBandStarvedPack(t *testing.T) *Pack {
	t.Helper()
	const dim = 8
	media := []*MediaItem{
		makeTextMedia("nown-1", "Lonely Nown", []float32{1, 0, 0, 0, 0, 0, 0, 0}),
	}
	var cards []*CardItem
	for j := 0; j < 4; j++ {
		cards = append(cards, makeTextCard(fmt.Sprintf("card-h-%d", j), "High", clone(media[0].Embedding)))
	}
	// No distant cards, only chaos.
	for j := 0; j < 10; j++ {
		cards = append(cards, makeTextCard(fmt.Sprintf("card-c-%d", j), "Chaos", randomUnitVector(j+100)))
	}

	pack := &Pack{
		Manifest: Manifest{
			PackTag:          "test-starved",
			FormatVersion:    1,
			Language:         "en",
			EmbeddingModel:   "synthetic",
			EmbeddingVersion: "1.0",
			AgeRating:        "everyone",
			CreatedAt:        "2026-08-20T00:00:00Z",
			Attribution:      nil,
			Checksums:        map[string]string{},
		},
		Media: media,
		Cards: cards,
	}
	pack.Candidates = BuildCandidates(media, cards, defaultDealing())
	return pack
}

func makeTextMedia(id, content string, emb []float32) *MediaItem {
	return &MediaItem{
		ID:         id,
		Type:       MediaTypeText,
		Content:    content,
		Embedding:  normalize(emb),
		Tags:       []string{"text"},
		ToneBucket: "millennial-cope",
		Rating:     RatingEveryone,
		License:    "CC0-1.0",
	}
}

func makeTextCard(id, content string, emb []float32) *CardItem {
	return &CardItem{
		ID:         id,
		Type:       MediaTypeText,
		Content:    content,
		Embedding:  normalize(emb),
		Tags:       []string{"text"},
		ToneBucket: "chaos",
	}
}

func clone(v []float32) []float32 {
	out := make([]float32, len(v))
	copy(out, v)
	return out
}

func distantVector(dim, axis int) []float32 {
	v := make([]float32, dim)
	// Strong on the target axis (high for it), moderate on all others (distant
	// for them). After normalisation this lands in the distant band for every
	// non-target Nown.
	for i := 0; i < dim; i++ {
		if i == axis {
			v[i] = 0.58
		} else {
			v[i] = 0.30
		}
	}
	return normalize(v)
}

func randomUnitVector(seed int) []float32 {
	rng := rand.New(rand.NewSource(int64(seed)))
	v := make([]float32, 8)
	for i := range v {
		v[i] = rng.Float32()*2 - 1
	}
	return normalize(v)
}

func normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return v
	}
	n := float32(1.0 / sqrtFloat64(sum))
	for i := range v {
		v[i] *= n
	}
	return v
}

func sqrtFloat64(x float64) float64 {
	// Avoid importing math in helper file to keep it small; this is test-only.
	if x == 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// writeGoldenBundle writes a golden pack to disk as a bundle directory.
func writeGoldenBundle(t *testing.T, dir string) {
	t.Helper()
	pack := buildGoldenPack(t)

	manifestPath := filepath.Join(dir, "manifest.json")
	mediaPath := filepath.Join(dir, "media.jsonl")
	cardsPath := filepath.Join(dir, "cards.jsonl")
	assetsDir := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}

	// Include a dummy asset file so loader hash-mismatch tests have something
	// to tamper with. It is not referenced by any media item.
	dummy := []byte("dummy-asset")
	dummyRef := ContentHash(dummy)
	if err := os.WriteFile(filepath.Join(assetsDir, dummyRef), dummy, 0o644); err != nil {
		t.Fatalf("write dummy asset: %v", err)
	}

	pack.Manifest.Checksums["media.jsonl"] = hashFileContent(writeJSONL(t, mediaPath, mediaItemsToRaw(pack.Media)))
	pack.Manifest.Checksums["cards.jsonl"] = hashFileContent(writeJSONL(t, cardsPath, cardItemsToRaw(pack.Cards)))

	manifestRaw, _ := json.Marshal(pack.Manifest)
	if err := os.WriteFile(manifestPath, manifestRaw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// writeBundleWithAssetRef writes a minimal bundle where one media item
// references an asset file.
func writeBundleWithAssetRef(t *testing.T, dir string) {
	t.Helper()
	media := []*MediaItem{
		{
			ID:         "image-nown-1",
			Type:       MediaTypeImage,
			AssetRef:   "",
			Embedding:  normalize([]float32{1, 0, 0, 0, 0, 0, 0, 0}),
			Tags:       []string{"image"},
			ToneBucket: "chaos",
			Rating:     RatingEveryone,
			License:    "CC0-1.0",
		},
	}
	cards := []*CardItem{
		makeTextCard("card-1", " unrelated", normalize([]float32{0, 1, 0, 0, 0, 0, 0, 0})),
	}
	pack := &Pack{
		Manifest: Manifest{
			PackTag:          "test-asset-ref",
			FormatVersion:    1,
			Language:         "en",
			EmbeddingModel:   "synthetic",
			EmbeddingVersion: "1.0",
			AgeRating:        "everyone",
			CreatedAt:        "2026-08-20T00:00:00Z",
			Checksums:        map[string]string{"media.jsonl": "", "cards.jsonl": ""},
		},
		Media: media,
		Cards: cards,
	}

	assetsDir := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	assetData := []byte("a tiny image")
	pack.Media[0].AssetRef = ContentHash(assetData)
	if err := os.WriteFile(filepath.Join(assetsDir, pack.Media[0].AssetRef), assetData, 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	manifestPath := filepath.Join(dir, "manifest.json")
	mediaPath := filepath.Join(dir, "media.jsonl")
	cardsPath := filepath.Join(dir, "cards.jsonl")
	pack.Manifest.Checksums["media.jsonl"] = hashFileContent(writeJSONL(t, mediaPath, mediaItemsToRaw(pack.Media)))
	pack.Manifest.Checksums["cards.jsonl"] = hashFileContent(writeJSONL(t, cardsPath, cardItemsToRaw(pack.Cards)))
	manifestRaw, _ := json.Marshal(pack.Manifest)
	if err := os.WriteFile(manifestPath, manifestRaw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func mediaItemsToRaw(items []*MediaItem) []any {
	var out []any
	for _, it := range items {
		out = append(out, it)
	}
	return out
}

func cardItemsToRaw(items []*CardItem) []any {
	var out []any
	for _, it := range items {
		out = append(out, it)
	}
	return out
}

func writeJSONL(t *testing.T, path string, records []any) []byte {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	for _, r := range records {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal record: %v", err)
		}
		if _, err := fmt.Fprintln(f, string(b)); err != nil {
			t.Fatalf("write record: %v", err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back %s: %v", path, err)
	}
	return data
}

func hashFileContent(data []byte) string {
	return ContentHash(data)
}
