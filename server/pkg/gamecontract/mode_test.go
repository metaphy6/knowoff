package gamecontract

import (
	"strings"
	"testing"
)

func TestIdentifierBoundaries(t *testing.T) {
	for _, value := range []string{"a", "copy:1", "rules_v2.1-beta", "-", strings.Repeat("a", 128)} {
		if !ValidIdentifier(value) {
			t.Fatalf("rejected wire identifier %q", value)
		}
	}
	for _, value := range []string{"", strings.Repeat("a", 129), " a", "a ", "a/b", "a\nb", "Türkçe"} {
		if ValidIdentifier(value) {
			t.Fatalf("accepted invalid identifier %q", value)
		}
	}
}

func TestCanonicalContentLanguage(t *testing.T) {
	for _, tag := range []string{"en", "tr", "en-US", "zh-Hant", "sr-Latn", "ar"} {
		if !ValidContentLanguage(tag) {
			t.Fatalf("rejected canonical tag %q", tag)
		}
	}
	for _, tag := range []string{"", "und", "EN", "en_us", "en-us", "en-INVALID", "../en", "not a language"} {
		if ValidContentLanguage(tag) {
			t.Fatalf("accepted invalid/noncanonical tag %q", tag)
		}
	}
}

func TestModeIDsAreStableAndClosed(t *testing.T) {
	want := []ModeID{"missed_the_briefing", "secret_scale", "make_room", "bad_bargains", "top_that"}
	got := AllModes()
	if len(got) != len(want) {
		t.Fatalf("modes: %v", got)
	}
	for i, mode := range want {
		if got[i] != mode || !mode.Valid() {
			t.Fatalf("mode %d: %q", i, got[i])
		}
	}
	for _, mode := range []ModeID{"", "show", "chaos", "Missed the Briefing", "unknown"} {
		if mode.Valid() {
			t.Fatalf("accepted unknown mode %q", mode)
		}
	}
	got[0] = "tampered"
	if AllModes()[0] != ModeMissedTheBriefing {
		t.Fatal("caller mutated mode registry")
	}
}
