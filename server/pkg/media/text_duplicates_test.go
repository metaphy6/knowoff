package media

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTextDraftDuplicateInspectionRejectsMalformedIdentityAndReviews(t *testing.T) {
	for _, kind := range []string{"blank_id", "invalid_id", "duplicate_id", "cross_kind_id", "empty_reference", "unknown_id", "self_pair", "duplicate_pair"} {
		t.Run(kind, func(t *testing.T) {
			b := textFixtureBundle(t, "en")
			b.Cards[1].Text = b.Cards[0].Text
			r := TextDuplicateReview{FirstID: b.Cards[0].ID, SecondID: b.Cards[1].ID, ReviewReference: "draft-review"}
			switch kind {
			case "blank_id":
				b.Cards[0].ID = ""
			case "invalid_id":
				b.Cards[0].ID = "bad/id"
			case "duplicate_id":
				b.Cards[1].ID = b.Cards[0].ID
			case "cross_kind_id":
				b.Cards[0].ID = b.Nowns[0].ID
			case "empty_reference":
				r.ReviewReference = ""
			case "unknown_id":
				r.SecondID = "missing"
			case "self_pair":
				r.SecondID = r.FirstID
			case "duplicate_pair":
				b.Manifest.DuplicateReviews = append(b.Manifest.DuplicateReviews, r)
			}
			b.Manifest.DuplicateReviews = append(b.Manifest.DuplicateReviews, r)
			raw, err := json.Marshal(b)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := InspectTextDuplicates(raw, textFixtureLimits()); err == nil {
				t.Fatal("malformed draft identity or review was accepted")
			}
		})
	}
}

func TestTextNearDuplicatesAreBoundedReviewCandidatesOnly(t *testing.T) {
	b := textFixtureBundle(t, "tr")
	b.Cards[0].Text = "Spare key"
	b.Cards[1].Text = "SPARE  KEY!"
	for i := 0; i < 2; i++ {
		b.Cards[i].Provenance.AcceptedTextSHA256 = ContentHash([]byte(b.Cards[i].Text))
	}
	before := textClone(b)
	candidates, err := TextDuplicateCandidates(b, textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Kind != "case-space-punctuation" || candidates[0].Reviewed {
		t.Fatalf("expected an unreviewed near candidate: %+v", candidates)
	}
	if !reflect.DeepEqual(before, b) {
		t.Fatal("near-duplicate detection merged or rewrote accepted text")
	}
	b.Manifest.DuplicateReviews = []TextDuplicateReview{{FirstID: b.Cards[0].ID, SecondID: b.Cards[1].ID, ReviewReference: "synthetic-near-review"}}
	candidates, err = TextDuplicateCandidates(b, textFixtureLimits())
	if err != nil || !candidates[0].Reviewed {
		t.Fatal("human decision reference was lost")
	}
	for i := range b.Cards {
		b.Cards[i].Text = "same"
	}
	limits := textFixtureLimits()
	limits.MaxRecords = 40
	if _, err := TextDuplicateCandidates(b, limits); err == nil {
		t.Fatal("unbounded duplicate candidate explosion accepted")
	}
}
