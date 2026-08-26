package game

import (
	"fmt"

	"github.com/knowoff/knowoff/server/internal/transport"
)

func (m *Match) handlePoke(seat int, payload map[string]any) error {
	if m.phase != PhasePlay && m.phase != PhaseDiscussion {
		return fmt.Errorf("cannot poke outside play or discussion")
	}
	targetF, _ := payload["target_seat"].(float64)
	target := int(targetF)
	if target < 0 || target >= m.size || m.eliminated[target] {
		return fmt.Errorf("invalid target")
	}
	if target == seat {
		return fmt.Errorf("cannot poke self")
	}
	if !m.connected[target] {
		return fmt.Errorf("target disconnected")
	}
	p := m.players[seat]
	if p.PokesUsed == nil {
		p.PokesUsed = make(map[int]bool)
	}
	if p.PokesUsed[target] {
		return fmt.Errorf("already poked this target this round")
	}
	p.PokesUsed[target] = true
	m.bcast.Broadcast(transport.NewEvent(transport.EventQuickChat, map[string]any{
		"kind":        "poke",
		"from_seat":   seat,
		"target_seat": target,
	}), -1)
	return nil
}

func (m *Match) handleQuickChat(seat int, payload map[string]any) error {
	phrase, _ := payload["phrase_id"].(string)
	if phrase == "" {
		return fmt.Errorf("phrase_id required")
	}
	event := map[string]any{
		"kind":      "chat",
		"from_seat": seat,
		"phrase_id": phrase,
	}
	// Targeted phrases (suspect/trust) name a seat — tapped on that seat's
	// box on the table — everyone else stays untargeted table talk.
	if raw, ok := payload["target_seat"]; ok {
		targetF, ok := raw.(float64)
		if !ok {
			return fmt.Errorf("invalid target")
		}
		target := int(targetF)
		if target < 0 || target >= m.size || m.eliminated[target] {
			return fmt.Errorf("invalid target")
		}
		if target == seat {
			return fmt.Errorf("cannot target self")
		}
		event["target_seat"] = target
	}
	m.bcast.Broadcast(transport.NewEvent(transport.EventQuickChat, event), -1)
	return nil
}
