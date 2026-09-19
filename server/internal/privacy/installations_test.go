package privacy

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestInstallationSelectorsSeparatePurposeInstallationAndKey(t *testing.T) {
	ctx := context.Background()
	key := bytes.Repeat([]byte{37}, 32)
	k := installationKeySnapshot{uuid.NewString(), map[string][]byte{"k1": key}}
	a, err := installationSelectors(ctx, k, "synthetic-installation-a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := installationSelectors(ctx, k, "synthetic-installation-a")
	if err != nil || !bytes.Equal(a.sanctions[0], b.sanctions[0]) || !bytes.Equal(a.bootstraps[0], b.bootstraps[0]) {
		t.Fatal("exact derivation changed", err)
	}
	if bytes.Equal(a.sanctions[0], a.bootstraps[0]) {
		t.Fatal("purpose separation absent")
	}
	for _, other := range []installationKeySnapshot{{uuid.NewString(), k.keys}, {k.installation, map[string][]byte{"k1": bytes.Repeat([]byte{38}, 32)}}} {
		b, err = installationSelectors(ctx, other, "synthetic-installation-a")
		if err != nil || bytes.Equal(a.sanctions[0], b.sanctions[0]) || bytes.Equal(a.bootstraps[0], b.bootstraps[0]) {
			t.Fatal("key or installation separation absent", err)
		}
	}
	b, err = installationSelectors(ctx, k, "synthetic-installation-b")
	if err != nil || bytes.Equal(a.sanctions[0], b.sanctions[0]) {
		t.Fatal("device separation absent", err)
	}
}

func TestInstallationSelectorsRejectUnboundedOrMissingAuthority(t *testing.T) {
	ctx := context.Background()
	valid := installationKeySnapshot{uuid.NewString(), map[string][]byte{"k1": bytes.Repeat([]byte{37}, 32)}}
	for _, raw := range []string{"", "bad\x00device", string(bytes.Repeat([]byte{'a'}, 257)), "bad\nline", "bad\x7fcontrol", "bad\vcontrol"} {
		if _, err := installationSelectors(ctx, valid, raw); err == nil {
			t.Fatal("invalid installation accepted")
		}
	}
	for _, bad := range []installationKeySnapshot{{"", valid.keys}, {valid.installation, nil}, {valid.installation, map[string][]byte{"": bytes.Repeat([]byte{1}, 32)}}, {valid.installation, map[string][]byte{"k1": {1}}}, {valid.installation, map[string][]byte{"1": bytes.Repeat([]byte{1}, 32), "2": bytes.Repeat([]byte{2}, 32), "3": bytes.Repeat([]byte{3}, 32), "4": bytes.Repeat([]byte{4}, 32), "5": bytes.Repeat([]byte{5}, 32)}}} {
		if _, err := installationSelectors(ctx, bad, "device"); err == nil {
			t.Fatal("invalid keyring accepted")
		}
	}
	if _, err := NewInstallationAuthority(nil, valid); err == nil {
		t.Fatal("missing executor accepted")
	}
	var missing *InstallationAuthority
	if err := missing.Register(ctx, "device"); err == nil {
		t.Fatal("nil authority registered")
	}
	if _, err := missing.EraseBatch(ctx, uuid.NewString(), 1); err == nil {
		t.Fatal("nil authority erased")
	}
}
