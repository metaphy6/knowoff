package game

import (
	"context"
	"errors"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"strings"
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
