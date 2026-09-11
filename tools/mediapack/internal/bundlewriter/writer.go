// Package bundlewriter persists an in-memory pack to the bundle directory
// format consumed by server/pkg/media.
package bundlewriter

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/knowoff/knowoff/server/pkg/media"
)

// Write persists pack to dir, creating manifest.json, media.jsonl,
// cards.jsonl, and assets/ when needed.
func Write(pack *media.Pack, dir string) error {
	if err := media.ValidatePackContent(pack); err != nil {
		return fmt.Errorf("invalid pack content: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create bundle dir: %w", err)
	}
	assetsDir := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return fmt.Errorf("create assets dir: %w", err)
	}
	// ValidatePackContent has already verified each reference is exactly the
	// SHA-256 of these bytes, so it is safe to use as a single filename.
	for ref, data := range pack.Assets {
		if err := os.WriteFile(filepath.Join(assetsDir, ref), data, 0o644); err != nil {
			return fmt.Errorf("write asset %s: %w", ref, err)
		}
	}

	mediaPath := filepath.Join(dir, "media.jsonl")
	cardsPath := filepath.Join(dir, "cards.jsonl")
	manifestPath := filepath.Join(dir, "manifest.json")

	mediaChecksum, err := writeJSONL(mediaPath, pack.Media)
	if err != nil {
		return fmt.Errorf("write media.jsonl: %w", err)
	}
	cardsChecksum, err := writeJSONL(cardsPath, pack.Cards)
	if err != nil {
		return fmt.Errorf("write cards.jsonl: %w", err)
	}

	pack.Manifest.Checksums["media.jsonl"] = mediaChecksum
	pack.Manifest.Checksums["cards.jsonl"] = cardsChecksum

	manifestRaw, err := json.MarshalIndent(pack.Manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifestRaw, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

func writeJSONL(path string, records any) (string, error) {
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	w := bufio.NewWriter(f)

	switch items := records.(type) {
	case []*media.MediaItem:
		for _, it := range items {
			b, err := json.Marshal(it)
			if err != nil {
				return "", err
			}
			if _, err := fmt.Fprintln(w, string(b)); err != nil {
				return "", err
			}
		}
	case []*media.CardItem:
		for _, it := range items {
			b, err := json.Marshal(it)
			if err != nil {
				return "", err
			}
			if _, err := fmt.Fprintln(w, string(b)); err != nil {
				return "", err
			}
		}
	default:
		return "", fmt.Errorf("unsupported record type")
	}

	if err := w.Flush(); err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
