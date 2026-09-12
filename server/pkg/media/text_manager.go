package media

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// TextManager activates complete validated releases. Its caller must authorize
// and durably audit the operation. It has no publication/contribution rewards.
// Matches retain Active's immutable pointer across later activation/rollback.
type TextManager struct {
	mu        sync.Mutex
	active    atomic.Pointer[TextSnapshot]
	releases  map[string]string
	revisions map[string]string
}

type TextRevisionIdentity struct {
	Language  string `json:"language"`
	ContentID string `json:"content_id"`
	Revision  uint64 `json:"revision"`
	SHA256    string `json:"sha256"`
}
type TextReleaseLineage struct {
	ReleaseID      string                 `json:"release_id"`
	ManifestSHA256 string                 `json:"manifest_sha256"`
	SnapshotSHA256 string                 `json:"snapshot_sha256"`
	Revisions      []TextRevisionIdentity `json:"revisions"`
}

// Lineage is immutable identity input for the durable publication transaction.
// Persist/check it before activation; process-local manager maps alone cannot
// protect a release or content revision against redefinition after a restart.
func (s *TextSnapshot) Lineage() TextReleaseLineage {
	if s == nil {
		return TextReleaseLineage{}
	}
	result := TextReleaseLineage{ReleaseID: s.bundle.Manifest.ReleaseID, ManifestSHA256: s.manifestHash, SnapshotSHA256: s.hash}
	add := func(id string, revision uint64, value any) {
		raw, _ := json.Marshal(value)
		result.Revisions = append(result.Revisions, TextRevisionIdentity{Language: s.bundle.Manifest.Language, ContentID: id, Revision: revision, SHA256: ContentHash(raw)})
	}
	for _, n := range s.bundle.Nowns {
		add(n.ID, n.Revision, n)
	}
	for _, c := range s.bundle.Cards {
		add(c.ID, c.Revision, c)
	}
	return result
}

func (m *TextManager) Active() *TextSnapshot {
	if m == nil {
		return nil
	}
	return m.active.Load()
}

func (m *TextManager) Activate(candidate *TextSnapshot, rules string, tuning TextDealTuning) error {
	if err := candidate.ValidateActivation(rules, tuning); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	manifest := candidate.bundle.Manifest
	if hash, ok := m.releases[manifest.ReleaseID]; ok && hash != candidate.manifestHash {
		return fmt.Errorf("immutable release ID conflict")
	}
	revisions := map[string]string{}
	add := func(id string, revision uint64, value any) error {
		key := fmt.Sprintf("%s/%s/%d", manifest.Language, id, revision)
		raw, _ := json.Marshal(value)
		hash := ContentHash(raw)
		if old, ok := m.revisions[key]; ok && old != hash {
			return fmt.Errorf("immutable content revision conflict")
		}
		revisions[key] = hash
		return nil
	}
	for _, nown := range candidate.bundle.Nowns {
		if err := add(nown.ID, nown.Revision, nown); err != nil {
			return err
		}
	}
	for _, card := range candidate.bundle.Cards {
		if err := add(card.ID, card.Revision, card); err != nil {
			return err
		}
	}
	if m.releases == nil {
		m.releases = map[string]string{}
	}
	if m.revisions == nil {
		m.revisions = map[string]string{}
	}
	m.releases[manifest.ReleaseID] = candidate.manifestHash
	for key, hash := range revisions {
		m.revisions[key] = hash
	}
	m.active.Store(candidate)
	return nil
}
