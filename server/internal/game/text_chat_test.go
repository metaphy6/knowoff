package game

import (
	"context"
	"errors"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"testing"
)

func TestTextChatPolicyTermsAndPinnedProfanity(t *testing.T) {
	words := map[string][]string{"en": {"badword"}, "tr": {"kotu"}}
	allowed := false
	calls := 0
	policy := NewTextChatModerator(words, func(context.Context, string) error {
		calls++
		if !allowed {
			return errors.New("terms.required")
		}
		return nil
	})
	words["en"][0] = "changed"
	typed := v2.Action{Kind: v2.ActionChat, Text: "badword kotu", UILocale: "tr-TR"}
	if _, err := policy(context.Background(), "account", typed); err == nil {
		t.Fatal("unaccepted terms admitted authored chat")
	}
	allowed = true
	out, err := policy(context.Background(), "account", typed)
	if err != nil || out.Text != "b****** k***" {
		t.Fatalf("policy failed: %#v %v", out, err)
	}
	before := calls
	for _, phrase := range []string{"suspect", "fit", "weird", "trust", "not_me", "laugh"} {
		if _, err := policy(context.Background(), "account", v2.Action{Kind: v2.ActionChat, PhraseID: phrase, UILocale: "en"}); err != nil {
			t.Fatal(err)
		}
	}
	if calls != before {
		t.Fatal("server authored phrases require user terms")
	}
	if _, err := policy(context.Background(), "account", v2.Action{Kind: v2.ActionChat, PhraseID: "arbitrary"}); err == nil {
		t.Fatal("unknown phrase accepted")
	}
	if _, err := NewTextChatModerator(nil, nil)(context.Background(), "account", typed); err == nil {
		t.Fatal("missing policy opened authored chat")
	}
}
