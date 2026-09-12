package store

import (
	"github.com/google/uuid"
	"testing"
)

func TestTextSourceSubmittedAndReviewedBytesImmutable(t *testing.T) {
	db, _ := textValueDB(t)
	admin, author := releaseTestAdmin(t, db)
	submission := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO portal_submissions(id,account_id,media_type,content,status,terms_version,terms_accepted_at,submitted_at) VALUES($1,$2,'text','Original','submitted','test-terms',now(),now())`, submission, author); err != nil {
		t.Fatal(err)
	}
	rejects := []string{
		`UPDATE portal_submissions SET content='Changed' WHERE id=$1`,
		`UPDATE portal_submissions SET status='draft',content='Changed' WHERE id=$1`,
		`UPDATE portal_submissions SET asset_ref='hidden-image' WHERE id=$1`,
		`UPDATE portal_submissions SET terms_accepted_at=now()+interval '1 second' WHERE id=$1`,
	}
	for _, q := range rejects {
		if _, err := db.Exec(q, submission); err == nil {
			t.Errorf("submitted rewrite accepted: %s", q)
		}
	}
	if _, err := db.Exec(`UPDATE portal_submissions SET status='draft',submitted_at=NULL WHERE id=$1`, submission); err != nil {
		t.Fatal("explicit withdrawal", err)
	}
	if _, err := db.Exec(`UPDATE portal_submissions SET content='Corrected' WHERE id=$1`, submission); err != nil {
		t.Fatal("draft edit after withdrawal", err)
	}
	if _, err := db.Exec(`UPDATE portal_submissions SET status='approved',submitted_at=now(),decided_at=now(),decided_by=$2 WHERE id=$1`, submission, admin); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`UPDATE portal_submissions SET asset_ref='replaced' WHERE id=$1`, `UPDATE portal_submissions SET asset_blob=decode('01','hex') WHERE id=$1`, `UPDATE portal_submissions SET content='Reworded' WHERE id=$1`, `UPDATE portal_submissions SET decided_by=NULL WHERE id=$1`} {
		if _, err := db.Exec(q, submission); err == nil {
			t.Errorf("reviewed rewrite accepted: %s", q)
		}
	}
	topic := uuid.NewString()
	entry := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO challenge_topics(id,week_start,week_end,nown_media_id) VALUES($1,'2026-09-07','2026-09-14',$2)`, topic, submission); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO challenge_entries(id,account_id,topic_id,entry_type,content,terms_version,terms_accepted_at,status) VALUES($1,$2,$3,'text','Entry','test-terms',now(),'submitted')`, entry, author, topic); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`UPDATE challenge_entries SET content='Changed' WHERE id=$1`, `UPDATE challenge_entries SET asset_ref='image' WHERE id=$1`, `DELETE FROM challenge_entries WHERE id=$1`} {
		if _, err := db.Exec(q, entry); err == nil {
			t.Errorf("entry rewrite accepted: %s", q)
		}
	}
	if _, err := db.Exec(`UPDATE challenge_entries SET status='approved',screen_decided_at=now(),screen_decided_by=$2 WHERE id=$1`, entry, admin); err != nil {
		t.Fatal("screening state", err)
	}
	if _, err := db.Exec(`UPDATE challenge_entries SET vote_count=vote_count+1 WHERE id=$1`, entry); err != nil {
		t.Fatal("voting bookkeeping", err)
	}
}
