// Package media owns the media-pack format, loader, relevance mesh, dealing,
// and signed-URL issuing for the Knowoff game server.
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Manifest is the top-level descriptor for a media pack.
type Manifest struct {
	PackTag          string            `json:"pack_tag"`
	FormatVersion    int               `json:"format_version"`
	Language         string            `json:"language"`
	EmbeddingModel   string            `json:"embedding_model"`
	EmbeddingVersion string            `json:"embedding_version"`
	AgeRating        string            `json:"age_rating"`
	CreatedAt        string            `json:"created_at"`
	Attribution      []Attribution     `json:"attribution"`
	Checksums        map[string]string `json:"checksums"`
}

// Attribution records the source and license for copyleft-sourced assets.
type Attribution struct {
	AssetID         string `json:"asset_id"`
	Source          string `json:"source"`
	License         string `json:"license"`
	AttributionText string `json:"attribution_text"`
}

// MediaType is the kind of media item.
type MediaType string

const (
	MediaTypeImage MediaType = "image"
	MediaTypeGIF   MediaType = "gif"
	MediaTypeText  MediaType = "text"
)

// Rating is the content rating for an asset.
type Rating string

const (
	RatingEveryone Rating = "everyone"
	RatingTeen     Rating = "teen"
	RatingAdult    Rating = "adult"
)

// MediaItem is a Nown (the round media) in a pack.
type MediaItem struct {
	ID          string    `json:"id"`
	Type        MediaType `json:"type"`
	AssetRef    string    `json:"asset_ref"`
	Content     string    `json:"content,omitempty"`
	Embedding   []float32 `json:"embedding"`
	Tags        []string  `json:"tags"`
	ToneBucket  string    `json:"tone_bucket"`
	Rating      Rating    `json:"rating"`
	License     string    `json:"license"`
	Attribution string    `json:"attribution"`
}

// CardItem is a playable hand card in a pack.
type CardItem struct {
	ID         string    `json:"id"`
	Type       MediaType `json:"type"`
	AssetRef   string    `json:"asset_ref"`
	Content    string    `json:"content,omitempty"`
	Embedding  []float32 `json:"embedding"`
	Tags       []string  `json:"tags"`
	ToneBucket string    `json:"tone_bucket"`
}

// Pack is a loaded, validated media pack with runtime indexes.
type Pack struct {
	Manifest Manifest
	Media    []*MediaItem
	Cards    []*CardItem
	Assets   map[string][]byte // asset_ref -> bytes

	// Per-Nown candidate lists precomputed from the relevance mesh.
	Candidates map[string]*BandCandidates
}

// BandCandidates holds card IDs split by relevance band for one Nown.
type BandCandidates struct {
	High    []string
	Distant []string
	Chaos   []string
}

// ContentHash returns the SHA-256 hex digest of data.
func ContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// HashManifest returns a stable JSON representation of the manifest for
// signing and verification.
func HashManifest(m Manifest) string {
	b, _ := json.Marshal(m)
	return ContentHash(b)
}

// ValidateManifest checks the manifest for required fields.
func ValidateManifest(m *Manifest) error {
	if m.PackTag == "" {
		return fmt.Errorf("manifest pack_tag is required")
	}
	if m.FormatVersion != 1 {
		return fmt.Errorf("unsupported format_version: %d", m.FormatVersion)
	}
	if m.Language == "" {
		return fmt.Errorf("manifest language is required")
	}
	if m.EmbeddingModel == "" || m.EmbeddingVersion == "" {
		return fmt.Errorf("manifest embedding_model and embedding_version are required")
	}
	if m.AgeRating == "" {
		return fmt.Errorf("manifest age_rating is required")
	}
	if len(m.Checksums) == 0 {
		return fmt.Errorf("manifest checksums are required")
	}
	return nil
}
