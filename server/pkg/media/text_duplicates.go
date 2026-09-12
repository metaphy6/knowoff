package media

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"golang.org/x/text/cases"
)

type TextDuplicateCandidate struct {
	FirstID  string `json:"first_id"`
	SecondID string `json:"second_id"`
	Kind     string `json:"kind"`
	Reviewed bool   `json:"reviewed"`
}

// InspectTextDuplicates accepts a bounded draft for review without granting the
// provenance/activation guarantees required by DecodeTextBundle and the loader.
func InspectTextDuplicates(raw []byte, limits TextLimits) ([]TextDuplicateCandidate, error) {
	if limits.MaxBundleBytes < 1 || int64(len(raw)) > limits.MaxBundleBytes {
		return nil, fmt.Errorf("draft byte bound exceeded")
	}
	var bundle TextBundle
	if err := textStrictJSON(raw, &bundle); err != nil {
		return nil, err
	}
	return TextDuplicateCandidates(bundle, limits)
}

// TextDuplicateCandidates offers bounded editorial leads, never rewrites or
// merges. Case/spacing/punctuation similarity can be wrong across languages;
// every such match remains a human decision and is not a dealing restriction.
func TextDuplicateCandidates(bundle TextBundle, limits TextLimits) ([]TextDuplicateCandidate, error) {
	if limits.MaxRecords < 1 || len(bundle.Nowns)+len(bundle.Cards) > limits.MaxRecords || len(bundle.Manifest.DuplicateReviews) > limits.MaxRecords {
		return nil, fmt.Errorf("duplicate review input bound exceeded")
	}
	groups := map[string][]string{}
	wording := map[string]string{}
	reviewed := map[string]bool{}
	add := func(id, text string) error {
		if !gamecontract.ValidIdentifier(id) || wording[id] != "" {
			return fmt.Errorf("draft IDs must be valid and unique")
		}
		normal, err := NormalizeText(text, limits.MaxTextBytes)
		if err != nil {
			return err
		}
		folded := cases.Fold().String(normal)
		folded = strings.Map(func(r rune) rune {
			if unicode.IsPunct(r) {
				return -1
			}
			return r
		}, folded)
		key := strings.Join(strings.Fields(folded), " ")
		groups[key] = append(groups[key], id)
		wording[id] = normal
		return nil
	}
	for _, n := range bundle.Nowns {
		if err := add(n.ID, n.Text); err != nil {
			return nil, err
		}
	}
	for _, c := range bundle.Cards {
		if err := add(c.ID, c.Text); err != nil {
			return nil, err
		}
	}
	for _, r := range bundle.Manifest.DuplicateReviews {
		pair := textDuplicatePair(r.FirstID, r.SecondID)
		if wording[r.FirstID] == "" || wording[r.SecondID] == "" || r.FirstID == r.SecondID || strings.TrimSpace(r.ReviewReference) == "" || len(r.ReviewReference) > limits.MaxTextBytes || reviewed[pair] {
			return nil, fmt.Errorf("draft duplicate review must identify a unique known pair and review reference")
		}
		reviewed[pair] = true
	}
	var result []TextDuplicateCandidate
	for _, ids := range groups {
		sort.Strings(ids)
		for i := range ids {
			for j := i + 1; j < len(ids); j++ {
				if len(result) >= limits.MaxRecords {
					return nil, fmt.Errorf("duplicate review output bound exceeded")
				}
				kind := "case-space-punctuation"
				if wording[ids[i]] == wording[ids[j]] {
					kind = "canonical"
				}
				result = append(result, TextDuplicateCandidate{FirstID: ids[i], SecondID: ids[j], Kind: kind, Reviewed: reviewed[textDuplicatePair(ids[i], ids[j])]})
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].FirstID != result[j].FirstID {
			return result[i].FirstID < result[j].FirstID
		}
		return result[i].SecondID < result[j].SecondID
	})
	return result, nil
}
