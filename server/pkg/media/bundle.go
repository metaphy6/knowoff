// Package media provides immutable, versioned text catalogs and their evidence.
package media

import (
	"crypto/sha256"
	"encoding/hex"
)

// ContentHash returns the SHA-256 identifier of exact captured bytes.
func ContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
