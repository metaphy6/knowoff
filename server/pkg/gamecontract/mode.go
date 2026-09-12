// Package gamecontract holds versioned identities shared by config, transport
// and standalone tools. A known mode is not necessarily enabled for admission.
package gamecontract

import "golang.org/x/text/language"

// ValidIdentifier is the shared wire/config identity syntax. IDs are opaque,
// case-sensitive ASCII tokens, never user-visible text or filesystem paths.
func ValidIdentifier(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r > 127 || !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' || r == ':') {
			return false
		}
	}
	return true
}

// ValidContentLanguage requires a canonical BCP 47 tag with a known language.
// Syntactic validity does not certify a pack or enable a public language cohort.
func ValidContentLanguage(tag string) bool {
	parsed, err := language.Parse(tag)
	base, _, _ := parsed.Raw()
	return err == nil && tag != "" && parsed.String() == tag && base.String() != "und"
}

type ModeID string

const (
	ModeMissedTheBriefing ModeID = "missed_the_briefing"
	ModeSecretScale       ModeID = "secret_scale"
	ModeMakeRoom          ModeID = "make_room"
	ModeBadBargains       ModeID = "bad_bargains"
	ModeTopThat           ModeID = "top_that"
)

func AllModes() []ModeID {
	return []ModeID{ModeMissedTheBriefing, ModeSecretScale, ModeMakeRoom, ModeBadBargains, ModeTopThat}
}

func (m ModeID) Valid() bool {
	switch m {
	case ModeMissedTheBriefing, ModeSecretScale, ModeMakeRoom, ModeBadBargains, ModeTopThat:
		return true
	default:
		return false
	}
}
