package media

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var textDataMembers = []string{"media.jsonl", "cards.jsonl", "suitability.jsonl"}

func textClone[T any](value T) T {
	raw, _ := json.Marshal(value)
	var copy T
	_ = json.Unmarshal(raw, &copy)
	return copy
}
func textLines[T any](values []T) []byte {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	for _, value := range values {
		_ = encoder.Encode(value)
	}
	return out.Bytes()
}
func textMembers(b TextBundle) map[string][]byte {
	return map[string][]byte{"media.jsonl": textLines(b.Nowns), "cards.jsonl": textLines(b.Cards), "suitability.jsonl": textLines(b.Suitability)}
}

// SealTextBundle prepares canonical member hashes. It does not create approvals,
// screening results, certification or rewards, and never rewrites accepted text.
func SealTextBundle(bundle TextBundle, limits TextLimits) (TextBundle, error) {
	bundle = textClone(bundle)
	if err := validateTextBundle(bundle, limits); err != nil {
		return TextBundle{}, err
	}
	bundle.Manifest.Checksums = map[string]string{}
	for name, raw := range textMembers(bundle) {
		bundle.Manifest.Checksums[name] = ContentHash(raw)
	}
	if _, err := NewTextSnapshot(bundle, limits); err != nil {
		return TextBundle{}, err
	}
	return bundle, nil
}

func NewTextSnapshot(bundle TextBundle, limits TextLimits) (*TextSnapshot, error) {
	if err := validateTextBundle(bundle, limits); err != nil {
		return nil, err
	}
	members := textMembers(bundle)
	if len(bundle.Manifest.Checksums) != len(members) {
		return nil, fmt.Errorf("missing or unknown text member checksum")
	}
	for name, raw := range members {
		if !textHashValid(bundle.Manifest.Checksums[name]) || ContentHash(raw) != bundle.Manifest.Checksums[name] {
			return nil, fmt.Errorf("text member checksum mismatch")
		}
	}
	for name, raw := range bundle.Artifacts {
		members[name] = raw
	}
	manifestRaw, err := json.Marshal(bundle.Manifest)
	if err != nil {
		return nil, err
	}
	members["manifest.json"] = manifestRaw
	var total int64
	for _, raw := range members {
		if int64(len(raw)) > limits.MaxFileBytes {
			return nil, fmt.Errorf("text member exceeds configured byte bound")
		}
		total += int64(len(raw))
	}
	if total > limits.MaxBundleBytes {
		return nil, fmt.Errorf("text bundle exceeds configured byte bound")
	}
	s := &TextSnapshot{bundle: textClone(bundle), nowns: map[string]int{}, cards: map[string]int{}, bands: map[string]string{}}
	s.manifestHash = ContentHash(manifestRaw)
	// Certificates bind this content identity; exclude their own references to
	// avoid recursive certificate/self-hash cycles. All artifact bytes are still
	// independently checked above and retained in this immutable snapshot.
	identity := textClone(bundle.Manifest)
	identity.CertificationArtifacts = nil
	raw, _ := json.Marshal(identity)
	s.hash = ContentHash(raw)
	for i, n := range s.bundle.Nowns {
		s.nowns[n.ID] = i
	}
	for i, c := range s.bundle.Cards {
		s.cards[c.ID] = i
	}
	for _, r := range s.bundle.Suitability {
		s.bands[textPair(r.Mode, r.NownID, r.CardID)] = r.Band
	}
	return s, nil
}

func (s *TextSnapshot) Manifest() TextManifest {
	if s == nil {
		return TextManifest{}
	}
	return textClone(s.bundle.Manifest)
}
func (s *TextSnapshot) SHA256() string {
	if s == nil {
		return ""
	}
	return s.hash
}

// ManifestSHA256 includes certification artifact references as well as content.
// Use it for immutable publication identity; SHA256 is the certificate subject.
func (s *TextSnapshot) ManifestSHA256() string {
	if s == nil {
		return ""
	}
	return s.manifestHash
}

// DecodeTextBundle is the same strict schema boundary used by the file loader.
// Prepared input can omit file hashes; SealTextBundle creates those afterwards.
func DecodeTextBundle(raw []byte, limits TextLimits) (TextBundle, error) {
	if limits.MaxBundleBytes < 1 || int64(len(raw)) > limits.MaxBundleBytes {
		return TextBundle{}, fmt.Errorf("candidate exceeds byte bound")
	}
	var b TextBundle
	if err := textStrictJSON(raw, &b); err != nil {
		return TextBundle{}, err
	}
	return SealTextBundle(b, limits)
}
func (s *TextSnapshot) Bundle() TextBundle {
	if s == nil {
		return TextBundle{}
	}
	return textClone(s.bundle)
}
func (s *TextSnapshot) Nown(id string) (TextNown, bool) {
	if s == nil {
		return TextNown{}, false
	}
	i, ok := s.nowns[id]
	if !ok {
		return TextNown{}, false
	}
	return textClone(s.bundle.Nowns[i]), true
}
func (s *TextSnapshot) Card(id string) (TextCard, bool) {
	if s == nil {
		return TextCard{}, false
	}
	i, ok := s.cards[id]
	if !ok {
		return TextCard{}, false
	}
	return textClone(s.bundle.Cards[i]), true
}

// LoadTextPack reads only bounded regular files within a confined root.
// Historical playable-image and format-v1 bundles are rejected.
func LoadTextPack(path string, limits TextLimits) (*TextSnapshot, error) {
	if limits.MaxFileBytes < 1 || limits.MaxBundleBytes < limits.MaxFileBytes || limits.MaxRecords < 1 || limits.MaxTextBytes < 1 {
		return nil, fmt.Errorf("text limits must be configured")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("text bundle root must be a regular directory")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	// There are three data files, a manifest and at most seven evidence artifacts.
	entries, err := dir.ReadDir(12)
	_ = dir.Close()
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(entries) > 11 {
		return nil, fmt.Errorf("too many text bundle members")
	}
	allowed := map[string]bool{"manifest.json": true}
	for _, name := range textDataMembers {
		allowed[name] = true
	}
	for _, name := range []string{"technical.json", "editorial.json", "actions.json", "screening.json", "release.json", "replay.json", "action-replay.json"} {
		allowed[name] = true
	}
	raws := map[string][]byte{}
	var total int64
	for _, entry := range entries {
		if !allowed[entry.Name()] || !entry.Type().IsRegular() {
			return nil, fmt.Errorf("unexpected or nonregular text bundle member")
		}
		file, err := root.Open(entry.Name())
		if err != nil {
			return nil, err
		}
		stat, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		if !stat.Mode().IsRegular() || stat.Size() > limits.MaxFileBytes {
			_ = file.Close()
			return nil, fmt.Errorf("oversized or nonregular text member")
		}
		raw, readErr := io.ReadAll(io.LimitReader(file, limits.MaxFileBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if int64(len(raw)) > limits.MaxFileBytes {
			return nil, fmt.Errorf("text member exceeds byte bound")
		}
		total += int64(len(raw))
		if total > limits.MaxBundleBytes {
			return nil, fmt.Errorf("text bundle exceeds byte bound")
		}
		raws[entry.Name()] = raw
	}
	var b TextBundle
	if err := textStrictJSON(raws["manifest.json"], &b.Manifest); err != nil {
		return nil, fmt.Errorf("text manifest: %w", err)
	}
	for _, name := range textDataMembers {
		raw, ok := raws[name]
		if !ok || ContentHash(raw) != b.Manifest.Checksums[name] {
			return nil, fmt.Errorf("missing or tampered text data member")
		}
	}
	count := 0
	parse := func(raw []byte, appendRow func([]byte) error) error {
		scanner := bufio.NewScanner(bytes.NewReader(raw))
		scanner.Buffer(make([]byte, 1024), int(limits.MaxFileBytes))
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(bytes.TrimSpace(line)) == 0 {
				return fmt.Errorf("blank JSONL record")
			}
			count++
			if count > limits.MaxRecords {
				return fmt.Errorf("text record bound exceeded")
			}
			if err := appendRow(line); err != nil {
				return err
			}
		}
		return scanner.Err()
	}
	if err := parse(raws["media.jsonl"], func(raw []byte) error {
		var n TextNown
		if err := textStrictJSON(raw, &n); err != nil {
			return err
		}
		b.Nowns = append(b.Nowns, n)
		return nil
	}); err != nil {
		return nil, err
	}
	if err := parse(raws["cards.jsonl"], func(raw []byte) error {
		var c TextCard
		if err := textStrictJSON(raw, &c); err != nil {
			return err
		}
		b.Cards = append(b.Cards, c)
		return nil
	}); err != nil {
		return nil, err
	}
	if err := parse(raws["suitability.jsonl"], func(raw []byte) error {
		var r TextSuitability
		if err := textStrictJSON(raw, &r); err != nil {
			return err
		}
		b.Suitability = append(b.Suitability, r)
		return nil
	}); err != nil {
		return nil, err
	}
	b.Artifacts = map[string][]byte{}
	for name, raw := range raws {
		if name != "manifest.json" && !strings.HasSuffix(name, ".jsonl") {
			b.Artifacts[name] = raw
		}
	}
	// Canonical bytes are required: whitespace/order changes need a newly sealed
	// bundle, never a hash-blind reinterpretation of a published artifact.
	return NewTextSnapshot(b, limits)
}

// WriteTextBundle reserves a new directory exclusively and writes the manifest
// last. It never overwrites published files; a partial preparation cannot load.
func WriteTextBundle(path string, bundle TextBundle, limits TextLimits) (err error) {
	snapshot, err := NewTextSnapshot(bundle, limits)
	if err != nil {
		return err
	}
	bundle = snapshot.Bundle()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err := os.Mkdir(path, 0700); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(path)
		}
	}()
	members := textMembers(bundle)
	for name, raw := range bundle.Artifacts {
		members[name] = raw
	}
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	manifestRaw, err := json.Marshal(bundle.Manifest)
	if err != nil {
		return err
	}
	members["manifest.json"] = manifestRaw
	names = append(names, "manifest.json")
	for _, name := range names {
		if err = os.WriteFile(filepath.Join(path, name), members[name], 0600); err != nil {
			return err
		}
	}
	return nil
}
