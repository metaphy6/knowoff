package game

import (
	"context"
	"errors"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NewTextChatModerator pins the configured profanity lists and gates authored
// words on independently versioned user terms. Canned phrases contain no user text.
func NewTextChatModerator(words map[string][]string, requireTerms func(context.Context, string) error) func(context.Context, string, v2.Action) (v2.Action, error) {
	filter := newProfanityFilter(words)
	return func(ctx context.Context, account string, action v2.Action) (v2.Action, error) {
		if action.Kind != v2.ActionChat {
			return v2.Action{}, errors.New("chat.invalid")
		}
		if action.PhraseID != "" {
			if action.Text != "" {
				return v2.Action{}, errors.New("chat.invalid")
			}
			switch action.PhraseID {
			case "suspect", "fit", "weird", "trust", "not_me", "laugh":
				return action, nil
			}
			return v2.Action{}, errors.New("chat.invalid_phrase")
		}
		if requireTerms == nil {
			return v2.Action{}, errors.New("terms.required")
		}
		if err := requireTerms(ctx, account); err != nil {
			return v2.Action{}, err
		}
		action.Text = strings.TrimSpace(action.Text)
		if action.Text == "" {
			return v2.Action{}, errors.New("chat.empty")
		}
		action.Text = filter.mask(action.Text, action.UILocale)
		return action, nil
	}
}

// The Unicode word masker is retained for authored text chat. It has no
// dependency on the retired image engine or its websocket envelopes.
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
