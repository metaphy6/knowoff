package store

import (
	"bytes"
	"github.com/google/uuid"
	"os"
	"testing"
	"time"
)

func TestPrivacyRequestAuthorityMigrationEmptyRoundTripAndRetainedRefusal(t *testing.T) {
	for _, retained := range []string{"empty", "capability", "intent"} {
		t.Run(retained, func(t *testing.T) {
			db, _ := textValueDB(t)
			account := valueAccount(t, db)
			before, err := db.Query(`SELECT to_jsonb(a)::text,to_jsonb(p)::text FROM accounts a JOIN profiles p ON p.account_id=a.id WHERE a.id=$1`, account)
			if err != nil {
				t.Fatal(err)
			}
			var originalAccount, originalProfile string
			if !before.Next() {
				t.Fatal("missing source")
			}
			if err = before.Scan(&originalAccount, &originalProfile); err != nil {
				t.Fatal(err)
			}
			before.Close()
			if retained == "capability" {
				if _, err = db.Exec(`INSERT INTO privacy_deletion_capabilities(account_id) VALUES($1)`, account); err != nil {
					t.Fatal(err)
				}
			}
			if retained == "intent" {
				if _, err = db.Exec(`SELECT privacy_begin_deletion_oauth($1,$2,$3,'google',$4,$5,$6,NULL)`, uuid.NewString(), bytes.Repeat([]byte{1}, 32), time.Now().Add(time.Minute), bytes.Repeat([]byte{2}, 32), string(bytes.Repeat([]byte{'v'}, 43)), string(bytes.Repeat([]byte{'n'}, 43))); err != nil {
					t.Fatal(err)
				}
			}
			down, err := os.ReadFile("../../migrations/000033_account_deletion_authority.down.sql")
			if err != nil {
				t.Fatal(err)
			}
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			_, err = tx.Exec(string(down))
			if retained != "empty" {
				if err == nil {
					t.Fatal("retained authority rollback succeeded")
				}
				if err = tx.Rollback(); err != nil {
					t.Fatal(err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				var absent bool
				if err = tx.QueryRow(`SELECT to_regclass('public.privacy_deletion_intents') IS NULL`).Scan(&absent); err != nil || !absent {
					t.Fatal("empty down did not drop authority", err)
				}
				up, e := os.ReadFile("../../migrations/000033_account_deletion_authority.up.sql")
				if e != nil {
					t.Fatal(e)
				}
				if _, e = tx.Exec(string(up)); e != nil {
					t.Fatal(e)
				}
				if e = tx.Commit(); e != nil {
					t.Fatal(e)
				}
			}
			var gotAccount, gotProfile string
			if err = db.QueryRow(`SELECT to_jsonb(a)::text,to_jsonb(p)::text FROM accounts a JOIN profiles p ON p.account_id=a.id WHERE a.id=$1`, account).Scan(&gotAccount, &gotProfile); err != nil || gotAccount != originalAccount || gotProfile != originalProfile {
				t.Fatal("migration changed original source", err)
			}
			var funcs int
			if err = db.QueryRow(`SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public' AND p.proname=ANY($1::text[]) AND p.prosecdef AND p.proconfig=ARRAY['search_path=pg_catalog']::text[]`, "{privacy_begin_enrollment,privacy_enroll_capability,privacy_begin_deletion_intent,privacy_begin_deletion_oauth,privacy_claim_deletion_oauth,privacy_complete_deletion_oauth,privacy_confirm_deletion,privacy_security_revoke_deletion,privacy_deletion_intent_status,privacy_expire_deletion_intents}").Scan(&funcs); err != nil || funcs != 10 {
				t.Fatal("authority not actually recreated", funcs, err)
			}
		})
	}
}
