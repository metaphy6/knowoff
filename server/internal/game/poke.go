package game

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/knowoff/knowoff/server/internal/transport"
)

type profanityFilter struct {
	wordLists map[string]map[string]struct{}
}

func newProfanityFilter(wordLists map[string][]string) *profanityFilter {
	filter := &profanityFilter{wordLists: make(map[string]map[string]struct{}, len(wordLists))}
	for language, words := range wordLists {
		list := make(map[string]struct{}, len(words))
		for _, word := range words {
			list[strings.ToLower(word)] = struct{}{}
		}
		filter.wordLists[language] = list
	}
	return filter
}

func (f *profanityFilter) mask(text, language string) string {
	languages := []string{"en"}
	if language = strings.Split(strings.ToLower(language), "-")[0]; language != "" && language != "en" {
		languages = append(languages, language)
	}

	var result strings.Builder
	for index := 0; index < len(text); {
		runeValue, size := utf8.DecodeRuneInString(text[index:])
		if !isChatWordRune(runeValue) {
			result.WriteString(text[index : index+size])
			index += size
			continue
		}
		start := index
		for index < len(text) {
			runeValue, size := utf8.DecodeRuneInString(text[index:])
			if !isChatWordRune(runeValue) {
				break
			}
			index += size
		}
		word := text[start:index]
		if f.contains(word, languages) {
			runes := []rune(word)
			result.WriteRune(runes[0])
			result.WriteString(strings.Repeat("*", len(runes)-1))
		} else {
			result.WriteString(word)
		}
	}
	return result.String()
}

func isChatWordRune(value rune) bool {
	return unicode.IsLetter(value) || unicode.IsNumber(value)
}

func (f *profanityFilter) contains(word string, languages []string) bool {
	for _, language := range languages {
		if _, found := f.wordLists[language][strings.ToLower(word)]; found {
			return true
		}
	}
	return false
}

func (m *Match) handlePoke(seat int, payload map[string]any) error {
	if m.phase != PhasePlay && m.phase != PhaseDiscussion && m.phase != PhaseKnowoff && m.phase != PhaseRunoff {
		return fmt.Errorf("cannot poke outside play, discussion, or voting phases")
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
	text, _ := payload["text"].(string)
	language, _ := payload["language"].(string)
	text = strings.TrimSpace(text)
	if phrase == "" && text == "" {
		return fmt.Errorf("phrase_id or text required")
	}
	if phrase != "" && text != "" {
		return fmt.Errorf("phrase_id and text are mutually exclusive")
	}
	if len(text) > 280 {
		return fmt.Errorf("chat message too long")
	}
	event := map[string]any{
		"kind":      "chat",
		"from_seat": seat,
	}
	if phrase != "" {
		event["phrase_id"] = phrase
	} else {
		event["text"] = m.chatFilter.mask(text, language)
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
