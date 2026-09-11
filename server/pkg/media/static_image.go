package media

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"golang.org/x/image/webp"
)

// ValidatePackContent enforces the image/text contract at loading,
// certification and bundle creation. Image assets must be static WebP bytes;
// a declared image type or a filename cannot disguise another format.
func ValidatePackContent(pack *Pack) error {
	for i, item := range pack.Media {
		if err := validateMediaItem(item, i+1); err != nil {
			return err
		}
		if item.Type == MediaTypeImage && len(pack.Assets[item.AssetRef]) == 0 {
			return fmt.Errorf("missing asset for media %s", item.ID)
		}
	}
	for i, item := range pack.Cards {
		if err := validateCardItem(item, i+1); err != nil {
			return err
		}
		if item.Type == MediaTypeImage && len(pack.Assets[item.AssetRef]) == 0 {
			return fmt.Errorf("missing asset for card %s", item.ID)
		}
	}
	for ref, data := range pack.Assets {
		if ref != ContentHash(data) {
			return fmt.Errorf("asset %s hash mismatch", ref)
		}
		if err := validateStaticWebP(data); err != nil {
			return fmt.Errorf("asset %s: %w", ref, err)
		}
	}
	return nil
}

func validateStaticWebP(data []byte) error {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return fmt.Errorf("image asset must be static WebP")
	}
	if uint64(binary.LittleEndian.Uint32(data[4:8]))+8 != uint64(len(data)) {
		return fmt.Errorf("invalid WebP container size")
	}
	for offset := uint64(12); offset < uint64(len(data)); {
		if offset+8 > uint64(len(data)) {
			return fmt.Errorf("truncated WebP chunk")
		}
		kind := string(data[offset : offset+4])
		size := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		end := offset + 8 + size
		if end+size%2 > uint64(len(data)) {
			return fmt.Errorf("truncated WebP chunk")
		}
		if kind == "ANIM" || kind == "ANMF" || (kind == "VP8X" && size > 0 && data[offset+8]&0x02 != 0) {
			return fmt.Errorf("animated images are unsupported; use static WebP")
		}
		offset = end + size%2
	}
	config, err := webp.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("invalid WebP image: %w", err)
	}
	if config.Width < 1 || config.Height < 1 || config.Width > 720 || config.Height > 720 {
		return fmt.Errorf("image longest side must be at most 720px")
	}
	if _, err := webp.Decode(bytes.NewReader(data)); err != nil {
		return fmt.Errorf("invalid WebP image: %w", err)
	}
	return nil
}
