// Package media owns the media-pack format, loader, relevance mesh, dealing,
// and signed-URL issuing for the Knowoff game server.
package media

import (
	"sync/atomic"
)

// Manager holds the currently active media pack with lock-free atomic access.
// It supports hot-swapping a new pack while rooms running against the previous
// pack continue untouched.
type Manager struct {
	pack atomic.Pointer[Pack]
}

// NewManager returns a Manager seeded with the provided pack. If p is nil, the
// Manager reports as not loaded until Load is called.
func NewManager(p *Pack) *Manager {
	m := &Manager{}
	if p != nil {
		m.pack.Store(p)
	}
	return m
}

// Load swaps the active pack atomically. Any room already holding a reference
// to the previous pack keeps using it.
func (m *Manager) Load(p *Pack) {
	if p != nil {
		m.pack.Store(p)
	}
}

// Active returns the currently loaded pack.
func (m *Manager) Active() *Pack {
	return m.pack.Load()
}

// AssetBytes returns raw asset bytes from the active pack, or nil if no asset
// with the requested reference exists or no pack is loaded.
func (m *Manager) AssetBytes(ref string) []byte {
	p := m.Active()
	if p == nil {
		return nil
	}
	return p.Assets[ref]
}

// MediaByID returns a Nown by id from the active pack.
func (m *Manager) MediaByID(id string) *MediaItem {
	p := m.Active()
	if p == nil {
		return nil
	}
	for _, n := range p.Media {
		if n.ID == id {
			return n
		}
	}
	return nil
}

// CardByID returns a card by id from the active pack.
func (m *Manager) CardByID(id string) *CardItem {
	p := m.Active()
	if p == nil {
		return nil
	}
	for _, c := range p.Cards {
		if c.ID == id {
			return c
		}
	}
	return nil
}
