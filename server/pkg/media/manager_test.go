package media

import (
	"reflect"
	"testing"
)

func TestTextManagerExposesOnlyImmutableRevisionCopies(t *testing.T) {
	snapshot, err := NewTextSnapshot(textTestReviewedBundle(t, "retirement-one"), textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	manager := new(TextManager)
	if err := manager.Activate(snapshot, "text-v2", textFixtureTuning()); err != nil {
		t.Fatal(err)
	}
	active := manager.Active()
	original := active.Bundle()
	copy := active.Bundle()
	copy.Cards[0].Text = "caller mutation"
	copy.Artifacts["technical.json"][0] = 'x'
	if !reflect.DeepEqual(active.Bundle(), original) {
		t.Fatal("caller mutated pinned text or evidence bytes")
	}
	if _, ok := active.Nown("missing"); ok {
		t.Fatal("missing Nown fabricated")
	}
	if _, ok := active.Card("missing"); ok {
		t.Fatal("missing card fabricated")
	}
}

func TestTextManagerActivationPreservesPreviousReleaseAndRejectsFailedCandidate(t *testing.T) {
	manager := new(TextManager)
	first, err := NewTextSnapshot(textTestReviewedBundle(t, "retirement-first"), textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewTextSnapshot(textTestReviewedBundle(t, "retirement-second"), textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Activate(first, "text-v2", textFixtureTuning()); err != nil {
		t.Fatal(err)
	}
	pinned := manager.Active()
	before := pinned.Bundle()
	if err := manager.Activate(second, "text-v2", textFixtureTuning()); err != nil {
		t.Fatal(err)
	}
	if manager.Active() != second || !reflect.DeepEqual(pinned.Bundle(), before) {
		t.Fatal("activation changed a begun match's release")
	}
	synthetic, err := NewTextSnapshot(textFixtureBundle(t, "en"), textFixtureLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Activate(synthetic, "text-v2", textFixtureTuning()); err == nil {
		t.Fatal("failed candidate activated")
	}
	if manager.Active() != second || !reflect.DeepEqual(pinned.Bundle(), before) {
		t.Fatal("failed activation replaced current or pinned release")
	}
}
