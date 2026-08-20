package media

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadPack loads a pack bundle from a local directory, verifying every
// checksum declared in the manifest. A tampered bundle returns an error and
// does not partially load.
func LoadPack(root string, dealing DealingTuning) (*Pack, error) {
	manifestPath := filepath.Join(root, "manifest.json")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := ValidateManifest(&manifest); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	expectedChecksums := manifest.Checksums

	mediaPath := filepath.Join(root, "media.jsonl")
	if err := verifyChecksum(mediaPath, expectedChecksums["media.jsonl"]); err != nil {
		return nil, fmt.Errorf("media.jsonl checksum mismatch: %w", err)
	}
	media, err := loadMediaItems(mediaPath)
	if err != nil {
		return nil, fmt.Errorf("load media: %w", err)
	}

	cardsPath := filepath.Join(root, "cards.jsonl")
	if err := verifyChecksum(cardsPath, expectedChecksums["cards.jsonl"]); err != nil {
		return nil, fmt.Errorf("cards.jsonl checksum mismatch: %w", err)
	}
	cards, err := loadCardItems(cardsPath)
	if err != nil {
		return nil, fmt.Errorf("load cards: %w", err)
	}

	assets := make(map[string][]byte)
	assetsDir := filepath.Join(root, "assets")
	entries, err := os.ReadDir(assetsDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read assets dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		data, err := os.ReadFile(filepath.Join(assetsDir, name))
		if err != nil {
			return nil, fmt.Errorf("read asset %s: %w", name, err)
		}
		ref := name
		if got := ContentHash(data); got != ref {
			return nil, fmt.Errorf("asset %s hash mismatch: expected %s, got %s", name, ref, got)
		}
		assets[ref] = data
	}

	for _, m := range media {
		if m.Type != MediaTypeText && m.AssetRef != "" {
			if _, ok := assets[m.AssetRef]; !ok {
				return nil, fmt.Errorf("missing asset for media %s: %s", m.ID, m.AssetRef)
			}
		}
	}
	for _, c := range cards {
		if c.Type != MediaTypeText && c.AssetRef != "" {
			if _, ok := assets[c.AssetRef]; !ok {
				return nil, fmt.Errorf("missing asset for card %s: %s", c.ID, c.AssetRef)
			}
		}
	}

	pack := &Pack{
		Manifest:   manifest,
		Media:      media,
		Cards:      cards,
		Assets:     assets,
		Candidates: BuildCandidates(media, cards, dealing),
	}
	return pack, nil
}

func verifyChecksum(path, expected string) error {
	if expected == "" {
		return fmt.Errorf("no checksum declared for %s", filepath.Base(path))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	got := sha256.Sum256(data)
	gotHex := hex.EncodeToString(got[:])
	if gotHex != expected {
		return fmt.Errorf("expected %s, got %s", expected, gotHex)
	}
	return nil
}

func loadMediaItems(path string) ([]*MediaItem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var items []*MediaItem
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var it MediaItem
		if err := json.Unmarshal([]byte(line), &it); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if err := validateMediaItem(&it, lineNo); err != nil {
			return nil, err
		}
		items = append(items, &it)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func loadCardItems(path string) ([]*CardItem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var items []*CardItem
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var it CardItem
		if err := json.Unmarshal([]byte(line), &it); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if err := validateCardItem(&it, lineNo); err != nil {
			return nil, err
		}
		items = append(items, &it)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func validateMediaItem(it *MediaItem, lineNo int) error {
	if it.ID == "" {
		return fmt.Errorf("media line %d: id is required", lineNo)
	}
	if it.Type == "" {
		return fmt.Errorf("media line %d: type is required", lineNo)
	}
	if len(it.Embedding) == 0 {
		return fmt.Errorf("media line %d: embedding is required", lineNo)
	}
	if it.ToneBucket == "" {
		return fmt.Errorf("media line %d: tone_bucket is required", lineNo)
	}
	if it.License == "" {
		return fmt.Errorf("media line %d: license is required", lineNo)
	}
	return nil
}

func validateCardItem(it *CardItem, lineNo int) error {
	if it.ID == "" {
		return fmt.Errorf("card line %d: id is required", lineNo)
	}
	if it.Type == "" {
		return fmt.Errorf("card line %d: type is required", lineNo)
	}
	if len(it.Embedding) == 0 {
		return fmt.Errorf("card line %d: embedding is required", lineNo)
	}
	if it.ToneBucket == "" {
		return fmt.Errorf("card line %d: tone_bucket is required", lineNo)
	}
	return nil
}
