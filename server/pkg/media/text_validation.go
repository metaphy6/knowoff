package media

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"golang.org/x/text/unicode/norm"
)

// NormalizeText preserves case, internal spacing and language. NFC and trimming
// outer Unicode spaces are the only transformations. Call BEFORE accepting a
// contribution: loading an accepted revision never silently changes its bytes.
// Markup is inert plain display data. Render with Flutter Text or escaped HTML,
// never a Markdown/HTML interpreter. No external asset links are accepted.
func NormalizeText(raw string, maxBytes int) (string, error) {
	if maxBytes < 1 || !utf8.ValidString(raw) || len(raw) > maxBytes {
		return "", fmt.Errorf("invalid text encoding or byte bound")
	}
	for _, r := range raw {
		if unicode.IsControl(r) || (unicode.Is(unicode.Cf, r) && r != '\u200c' && r != '\u200d') || r == '\u2028' || r == '\u2029' {
			return "", fmt.Errorf("text contains spoofing/control character")
		}
	}
	text := norm.NFC.String(strings.TrimSpace(raw))
	if text == "" || len(text) > maxBytes {
		return "", fmt.Errorf("blank or oversized text")
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.Contains(lower, "www.") {
		return "", fmt.Errorf("external asset links are not playable text")
	}
	return text, nil
}

func textStrictJSON(raw []byte, target any) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("invalid JSON UTF-8")
	}
	// encoding/json replaces unpaired UTF-16 escapes with U+FFFD. Reject them
	// before decoding so ingestion cannot create a different accepted revision.
	inString := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || raw[i] != '\\' {
			continue
		}
		if i+1 >= len(raw) {
			return fmt.Errorf("invalid JSON escape")
		}
		if raw[i+1] != 'u' {
			i++
			continue
		}
		if i+6 > len(raw) {
			return fmt.Errorf("invalid Unicode escape")
		}
		code, err := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
		if err != nil {
			return err
		}
		if code >= 0xdc00 && code <= 0xdfff {
			return fmt.Errorf("unpaired low surrogate")
		}
		if code >= 0xd800 && code <= 0xdbff {
			if i+12 > len(raw) || string(raw[i+6:i+8]) != `\u` {
				return fmt.Errorf("unpaired high surrogate")
			}
			low, err := strconv.ParseUint(string(raw[i+8:i+12]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return fmt.Errorf("unpaired high surrogate")
			}
			i += 11
		} else {
			i += 5
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 64 {
			return fmt.Errorf("JSON structure is too deep")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return fmt.Errorf("duplicate JSON member")
				}
				seen[name] = true
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("invalid JSON delimiter")
		}
		_, err = decoder.Token()
		return err
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON value")
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func textModeKind(mode gamecontract.ModeID) (string, string) {
	switch mode {
	case gamecontract.ModeMissedTheBriefing:
		return "situation", "response"
	case gamecontract.ModeSecretScale, gamecontract.ModeTopThat:
		return "criterion", "item"
	case gamecontract.ModeMakeRoom, gamecontract.ModeBadBargains:
		return "plan", "item"
	}
	return "", ""
}

func textHasMode(modes []gamecontract.ModeID, mode gamecontract.ModeID) bool {
	for _, m := range modes {
		if m == mode {
			return true
		}
	}
	return false
}
func textModesValid(modes, allowed []gamecontract.ModeID) bool {
	seen := map[gamecontract.ModeID]bool{}
	if len(modes) == 0 {
		return false
	}
	for _, m := range modes {
		if !m.Valid() || seen[m] || !textHasMode(allowed, m) {
			return false
		}
		seen[m] = true
	}
	return true
}
func textHashValid(hash string) bool {
	return len(hash) == 64 && strings.Trim(hash, "0123456789abcdef") == ""
}
func textPair(mode gamecontract.ModeID, nown, card string) string {
	return string(mode) + "/" + nown + "/" + card
}
func textDuplicatePair(first, second string) string {
	if first > second {
		first, second = second, first
	}
	return first + "/" + second
}

func validateTextBundle(b TextBundle, limits TextLimits) error {
	if limits.MaxTextBytes < 1 || limits.MaxRecords < 1 || limits.MaxFileBytes < 1 || limits.MaxBundleBytes < limits.MaxFileBytes {
		return fmt.Errorf("text limits must be configured")
	}
	m := b.Manifest
	if m.SchemaVersion != TextSchemaVersion || !gamecontract.ValidIdentifier(m.ReleaseID) || !gamecontract.ValidIdentifier(m.RulesVersion) || !gamecontract.ValidContentLanguage(m.Language) || !textModesValid(m.Modes, gamecontract.AllModes()) || m.SuitabilityVersion != TextSuitabilityVersion {
		return fmt.Errorf("incompatible text manifest")
	}
	if m.AgeRating != "everyone" && m.AgeRating != "teen" && m.AgeRating != "adult" {
		return fmt.Errorf("invalid age rating")
	}
	if len(b.Nowns) == 0 || len(b.Cards) == 0 || len(b.Nowns)+len(b.Cards)+len(b.Suitability) > limits.MaxRecords {
		return fmt.Errorf("text record capacity invalid")
	}
	if m.Evaluator != nil && (!gamecontract.ValidIdentifier(m.Evaluator.Provider) || !gamecontract.ValidIdentifier(m.Evaluator.Model) || !gamecontract.ValidIdentifier(m.Evaluator.Version) || !textHashValid(m.Evaluator.EvidenceSHA256)) {
		return fmt.Errorf("unversioned optional evaluator")
	}
	allowedArtifacts := map[string]bool{"technical.json": true, "editorial.json": true, "actions.json": true, "screening.json": true, "release.json": true, "replay.json": true, "action-replay.json": true}
	if len(m.CertificationArtifacts) != len(b.Artifacts) {
		return fmt.Errorf("artifact manifest mismatch")
	}
	for name, hash := range m.CertificationArtifacts {
		if !allowedArtifacts[name] || !textHashValid(hash) || ContentHash(b.Artifacts[name]) != hash || !json.Valid(b.Artifacts[name]) {
			return fmt.Errorf("invalid certification artifact")
		}
	}
	seen := map[string]bool{}
	nowns := map[string]TextNown{}
	cards := map[string]TextCard{}
	wording := map[string]string{}
	validate := func(id string, revision uint64, typ, text, tone string, p TextProvenance) error {
		if !gamecontract.ValidIdentifier(id) || revision == 0 || seen[id] || typ != "text" {
			return fmt.Errorf("invalid or duplicate text identity")
		}
		seen[id] = true
		normal, err := NormalizeText(text, limits.MaxTextBytes)
		if err != nil || normal != text {
			return fmt.Errorf("accepted text is invalid or noncanonical")
		}
		wording[id] = text
		if tone != "millennial-cope" && tone != "gen-z-absurdism" && tone != "social-awkwardness" && tone != "chaos" {
			return fmt.Errorf("invalid tone bucket")
		}
		if !gamecontract.ValidIdentifier(p.SourceID) || p.SourceRevision == 0 || p.AcceptedTextSHA256 != ContentHash([]byte(text)) || p.License == "" || p.Attribution == "" {
			return fmt.Errorf("missing immutable text provenance")
		}
		if m.Synthetic {
			if p.SourceKind != "synthetic" {
				return fmt.Errorf("synthetic provenance mismatch")
			}
		} else {
			if (p.SourceKind != "original" && p.SourceKind != "contribution") || !gamecontract.ValidIdentifier(p.TermsVersion) || !gamecontract.ValidIdentifier(p.ConsentReference) || p.ConsentAtMS <= 0 || !gamecontract.ValidIdentifier(p.ApprovalReference) || !gamecontract.ValidIdentifier(p.EditorReference) || p.ReviewedAtMS < p.ConsentAtMS {
				return fmt.Errorf("missing reviewed contribution provenance")
			}
		}
		return nil
	}
	for _, n := range b.Nowns {
		if err := validate(n.ID, n.Revision, n.Type, n.Text, n.ToneBucket, n.Provenance); err != nil {
			return err
		}
		if !textModesValid(n.Modes, m.Modes) {
			return fmt.Errorf("invalid nown mode")
		}
		for _, mode := range n.Modes {
			kind, _ := textModeKind(mode)
			if n.Kind != kind {
				return fmt.Errorf("nown kind/mode mismatch")
			}
		}
		nowns[n.ID] = n
	}
	for _, c := range b.Cards {
		if err := validate(c.ID, c.Revision, c.Type, c.Text, c.ToneBucket, c.Provenance); err != nil {
			return err
		}
		if !textModesValid(c.Modes, m.Modes) {
			return fmt.Errorf("invalid card mode")
		}
		for _, mode := range c.Modes {
			_, pool := textModeKind(mode)
			if c.Pool != pool {
				return fmt.Errorf("card pool/mode mismatch")
			}
		}
		cards[c.ID] = c
	}
	relations := map[string]bool{}
	for _, r := range b.Suitability {
		n, nok := nowns[r.NownID]
		c, cok := cards[r.CardID]
		key := textPair(r.Mode, r.NownID, r.CardID)
		if !nok || !cok || r.NownRevision != n.Revision || r.CardRevision != c.Revision || !textHasMode(n.Modes, r.Mode) || !textHasMode(c.Modes, r.Mode) || relations[key] || !gamecontract.ValidIdentifier(r.ReviewReference) || (r.Band != "high" && r.Band != "distant" && r.Band != "chaos") {
			return fmt.Errorf("invalid reviewed suitability")
		}
		relations[key] = true
	}
	for _, mode := range m.Modes {
		nownCount, cardCount := 0, 0
		for _, n := range b.Nowns {
			if !textHasMode(n.Modes, mode) {
				continue
			}
			nownCount++
			for _, c := range b.Cards {
				if textHasMode(c.Modes, mode) && !relations[textPair(mode, n.ID, c.ID)] {
					return fmt.Errorf("missing reviewed pair coverage")
				}
			}
		}
		for _, c := range b.Cards {
			if textHasMode(c.Modes, mode) {
				cardCount++
			}
		}
		if nownCount == 0 || cardCount == 0 {
			return fmt.Errorf("empty mode coverage")
		}
	}
	reviews := map[string]bool{}
	for _, r := range m.DuplicateReviews {
		key := textDuplicatePair(r.FirstID, r.SecondID)
		if r.FirstID == r.SecondID || !seen[r.FirstID] || !seen[r.SecondID] || reviews[key] || !gamecontract.ValidIdentifier(r.ReviewReference) {
			return fmt.Errorf("invalid duplicate review")
		}
		reviews[key] = true
	}
	groups := map[string][]string{}
	for id, text := range wording {
		groups[text] = append(groups[text], id)
	}
	for _, ids := range groups {
		sort.Strings(ids)
		for i := range ids {
			for j := i + 1; j < len(ids); j++ {
				if !reviews[textDuplicatePair(ids[i], ids[j])] {
					return fmt.Errorf("canonical duplicate requires review")
				}
			}
		}
	}
	return nil
}
