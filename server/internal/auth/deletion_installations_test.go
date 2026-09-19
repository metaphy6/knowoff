package auth

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/privacy"
)

type deletionInstallationKeyring struct{ installation string }

func (k deletionInstallationKeyring) Keys(context.Context) (string, map[string][]byte, error) {
	return k.installation, map[string][]byte{"fixture-key-1": bytes.Repeat([]byte{31}, 32)}, nil
}

func TestDeletionInstallationCannotBootstrapSurvivingAccount(t *testing.T) {
	db := deletionIsolatedDB(t)
	m := newTestManager(db)
	hash := HashDevice(uuid.NewString())
	deleted, err := m.AuthenticateDevice(t.Context(), hash)
	if err != nil {
		t.Fatal(err)
	}
	survivor, err := m.AuthenticateDevice(t.Context(), HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO device_tokens(account_id,device_hash) VALUES($1,$2)`, survivor.AccountID, hash); err != nil {
		t.Fatal(err)
	}
	d, err := NewDeletionManager(m, db, nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	id, secret := deletionTestEnrollment(t, d, deleted.AccessToken)
	c := deletionTestCommand(t, d, deleted.AccountID, id, secret)
	if _, err = d.Confirm(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`SELECT privacy_bind_suppression($1,1,$2)`, c.RequestID, bytes.Repeat([]byte{8}, 32)); err != nil {
		t.Fatal(err)
	}
	deletionCredentialsErase(t, db, c.RequestID)
	authority, err := privacy.NewInstallationAuthority(db, deletionInstallationKeyring{uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	complete := false
	for range 10 {
		r, e := authority.EraseBatch(t.Context(), c.RequestID, 1)
		if e != nil {
			t.Fatal(e)
		}
		if r.Complete {
			complete = true
			break
		}
	}
	if !complete {
		t.Fatal("installation cleanup did not converge")
	}
	var accountsBefore, profilesBefore int
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM accounts),(SELECT count(*) FROM profiles)`).Scan(&accountsBefore, &profilesBefore); err != nil {
		t.Fatal(err)
	}
	if got, e := newTestManager(db).AuthenticateDevice(t.Context(), hash); e == nil {
		t.Fatalf("deleted bootstrap adopted surviving account: %s", got.AccountID)
	}
	m.SetInstallationAuthority(authority)
	if _, e := m.AuthenticateDevice(t.Context(), hash); e == nil {
		t.Fatal("configured authority admitted erased bootstrap")
	}
	var accountsAfter, profilesAfter int
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM accounts),(SELECT count(*) FROM profiles)`).Scan(&accountsAfter, &profilesAfter); err != nil || accountsAfter != accountsBefore || profilesAfter != profilesBefore {
		t.Fatal("refused bootstrap committed candidate state", err)
	}
	if got, e := m.Refresh(t.Context(), survivor.RefreshToken); e != nil || got.AccountID != survivor.AccountID {
		t.Fatal("explicit surviving account lost access", e)
	}
	fresh := HashDevice(uuid.NewString())
	if _, e := newTestManager(db).AuthenticateDevice(t.Context(), fresh); e == nil {
		t.Fatal("configuration downgrade bypassed required finite-evidence key")
	}
	if got, e := m.AuthenticateDevice(t.Context(), fresh); e != nil || got.AccountID == survivor.AccountID || got.AccountID == deleted.AccountID {
		t.Fatal("registered fresh bootstrap failed", e)
	}
	if _, err = db.Exec(`UPDATE privacy_installation_evidence SET match_until=clock_timestamp(),purge_after=clock_timestamp() WHERE request_id=$1`, c.RequestID); err != nil {
		t.Fatal(err)
	}
	if got, e := m.AuthenticateDevice(t.Context(), hash); e != nil || got.AccountID == deleted.AccountID || got.AccountID == survivor.AccountID {
		t.Fatal("expired bootstrap evidence restored an old identity", e)
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM device_tokens WHERE account_id=$1 AND device_hash=$2`, survivor.AccountID, hash).Scan(&n); err != nil || n != 1 {
		t.Fatal("survivor association removed", n, err)
	}
}
