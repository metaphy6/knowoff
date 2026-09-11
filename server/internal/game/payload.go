// Package game implements the authoritative realtime match state machine for
// Knowoff. The payload renderer in this file is the single choke point through
// which every Nown payload must pass before leaving the server; it enforces
// role secrecy by ensuring Nown content and identifiers never reach Donowers or
// eliminated spectators.
package game

import (
	"fmt"
	"time"

	"github.com/knowoff/knowoff/server/pkg/media"
)

// RecipientView selects which visual information a recipient is allowed to see.
type RecipientView int

const (
	// ViewNower grants the real Nown payload.
	ViewNower RecipientView = iota
	// ViewDecoy grants only the generic placeholder; used for Donowers and
	// any player eliminated from the match.
	ViewDecoy
)

// PayloadRenderer is the only server component that builds Nown-shaped outbound
// payloads. All gameplay paths must call it rather than assembling Nown maps
// by hand, so secrecy can be enforced in one place and verified by static and
// runtime tests.
type PayloadRenderer struct {
	media     *media.Manager
	issuer    *media.SignedURLIssuer
	assetBase string
}

// NewPayloadRenderer returns a renderer bound to the active media pack, signed
// URL issuer, and asset base URL (e.g. https://cdn.example.com).
func NewPayloadRenderer(media *media.Manager, issuer *media.SignedURLIssuer, assetBase string) *PayloadRenderer {
	return &PayloadRenderer{
		media:     media,
		issuer:    issuer,
		assetBase: assetBase,
	}
}

// NownPayload builds the round payload for a single recipient. For ViewNower it
// returns {nown: {id, signed_url, type[, content]}}; for ViewDecoy it returns
// {decoy: true}. No other shape is ever produced.
func (r *PayloadRenderer) NownPayload(roundID string, nownID string, view RecipientView) (map[string]any, error) {
	if view == ViewDecoy {
		return map[string]any{"decoy": true}, nil
	}

	if r.media == nil {
		return nil, fmt.Errorf("media manager not loaded")
	}
	item := r.media.MediaByID(nownID)
	if item == nil {
		return nil, fmt.Errorf("nown %q not found", nownID)
	}
	nownMap, err := r.mediaItemPayload(roundID, item.ID, item.Type, item.Content, item.AssetRef)
	if err != nil {
		return nil, err
	}
	return map[string]any{"nown": nownMap}, nil
}

// MediaPayload builds a {id, type[, content][, signed_url]} payload for any
// Nown by id, regardless of round secrecy. Used for the verdict reveal, where
// every Nown is shown to every player once the match ends.
func (r *PayloadRenderer) MediaPayload(roundID string, nownID string) (map[string]any, error) {
	if r.media == nil {
		return nil, fmt.Errorf("media manager not loaded")
	}
	item := r.media.MediaByID(nownID)
	if item == nil {
		return nil, fmt.Errorf("nown %q not found", nownID)
	}
	return r.mediaItemPayload(roundID, item.ID, item.Type, item.Content, item.AssetRef)
}

// CardPayload builds a {id, type[, content][, signed_url]} payload for a hand
// or draw-pile card by id. Cards carry no round secrecy, so any recipient may
// receive the same payload for a given card id.
func (r *PayloadRenderer) CardPayload(roundID string, cardID string) (map[string]any, error) {
	if r.media == nil {
		return nil, fmt.Errorf("media manager not loaded")
	}
	card := r.media.CardByID(cardID)
	if card == nil {
		return nil, fmt.Errorf("card %q not found", cardID)
	}
	return r.mediaItemPayload(roundID, card.ID, card.Type, card.Content, card.AssetRef)
}

// mediaItemPayload assembles the common {id, type[, content][, signed_url]}
// shape shared by Nowns and cards.
func (r *PayloadRenderer) mediaItemPayload(roundID, id string, typ media.MediaType, content, assetRef string) (map[string]any, error) {
	payload := map[string]any{
		"id":   id,
		"type": string(typ),
	}
	switch typ {
	case media.MediaTypeText:
		// Text items carry their literal content; there is no asset to sign.
		payload["content"] = content
	case media.MediaTypeImage:
		if assetRef == "" {
			return nil, fmt.Errorf("item %q has empty asset_ref", id)
		}
		token, _, err := r.issuer.Issue(roundID, assetRef, time.Now())
		if err != nil {
			return nil, fmt.Errorf("issue signed url: %w", err)
		}
		payload["signed_url"] = signedURL(r.assetBase, assetRef, token)
	default:
		return nil, fmt.Errorf("unknown item type %q", typ)
	}
	return payload, nil
}

// signedURL assembles the absolute URL a Nower client fetches. The token is
// exposed as a query parameter so the CDN/edge can verify it without path
// rewriting and the local development stack can serve assets from a simple
// handler.
func signedURL(base, assetRef, token string) string {
	if base == "" {
		base = "/assets"
	}
	// Avoid double slashes while preserving a trailing path segment.
	if base[len(base)-1] == '/' {
		return fmt.Sprintf("%s%s?token=%s", base, assetRef, token)
	}
	return fmt.Sprintf("%s/%s?token=%s", base, assetRef, token)
}
