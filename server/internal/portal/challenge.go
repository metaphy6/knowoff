package portal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ContributionConsent records the exact terms shown and explicitly accepted.
type ContributionConsent struct {
	Version  string
	Accepted bool
}

// CurrentTerms returns the most recent terms that have actually taken effect.
func (m *Manager) CurrentTerms(ctx context.Context) (*TermsVersion, error) {
	var t TermsVersion
	err := m.db.QueryRowContext(ctx, `SELECT version,title,body,active_from FROM portal_terms WHERE active_from <= now() ORDER BY active_from DESC,version DESC LIMIT 1`).Scan(&t.Version, &t.Title, &t.Body, &t.ActiveFrom)
	if err != nil {
		return nil, fmt.Errorf("load current terms: %w", err)
	}
	return &t, nil
}

// auditTx makes the audit record part of the product mutation's transaction.
func auditTx(ctx context.Context, tx *sql.Tx, adminID, action, targetType, targetID string, before, after map[string]any) error {
	b, err := json.Marshal(before)
	if err != nil {
		return err
	}
	a, err := json.Marshal(after)
	if err != nil {
		return err
	}
	var actor any
	if adminID != "" {
		actor = adminID
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO admin_audit_log(admin_id,action,target_type,target_id,before_state,after_state) VALUES($1,$2,$3,$4,$5,$6)`, actor, action, targetType, targetID, b, a)
	return err
}

func (m *Manager) maxChallengeEntries() int {
	n := m.cfg.Tuning.LiveOps.ChallengeMaxEntries
	if n <= 0 {
		return 100
	}
	return n
}
func (m *Manager) MaxTextBytes() int {
	portalMax := m.cfg.Tuning.Portal.MaxTextSubmissionLength
	contractMax := m.cfg.Tuning.Contract.MaxTextBytes
	if portalMax < 1 || contractMax < 1 {
		return 0
	}
	if portalMax < contractMax {
		return portalMax
	}
	return contractMax
}

func weekMonday(t time.Time) time.Time {
	day := serverDay(t.UTC())
	return day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
}
func topicOpen(t *ChallengeTopic, now time.Time) bool {
	return t.ClosedAt == nil && !now.Before(t.WeekStart) && now.Before(t.WeekEnd.AddDate(0, 0, 1))
}

func (m *Manager) CreateChallengeTopic(ctx context.Context, adminID string, weekStart time.Time, nownMediaID string) (*ChallengeTopic, error) {
	if !weekStart.Equal(weekMonday(weekStart)) {
		return nil, errors.New("week must begin Monday at 00:00 UTC")
	}
	// Rescreen the source before publication, including legacy approvals.
	var screenedText string
	if err := m.db.QueryRowContext(ctx, `SELECT content FROM portal_submissions WHERE id=$1 AND status IN ('approved','published') AND media_type='text'`, nownMediaID).Scan(&screenedText); err != nil {
		return nil, errors.New("approved text topic required")
	}
	if normalized, err := m.normalizedContribution(screenedText); err != nil || normalized != screenedText {
		return nil, errors.New("approved short text topic required")
	}
	if err := m.screenText(ctx, screenedText); err != nil {
		return nil, err
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = lockPortalAdmin(ctx, tx, adminID, m.now()); err != nil {
		return nil, err
	}
	var status, kind, content string
	if err = tx.QueryRowContext(ctx, `SELECT status,media_type,content FROM portal_submissions WHERE id=$1 FOR SHARE`, nownMediaID).Scan(&status, &kind, &content); err != nil {
		return nil, errors.New("approved topic submission required")
	}
	if (status != "approved" && status != "published") || kind != "text" {
		return nil, errors.New("approved text topic required")
	}
	if content != screenedText {
		return nil, errors.New("topic changed during screening; review again")
	}
	id := uuid.NewString()
	end := weekStart.AddDate(0, 0, 6)
	var activated any
	if !m.now().Before(weekStart) && m.now().Before(end.AddDate(0, 0, 1)) {
		activated = m.now()
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO challenge_topics(id,week_start,week_end,nown_media_id,published_at,activated_at,source_revision) VALUES($1,$2::date,$3::date,$4,$5,$6,$7)`, id, weekStart, end, nownMediaID, weekStart, activated, ContentRevision(screenedText))
	if err != nil {
		return nil, err
	}
	if err = auditTx(ctx, tx, adminID, "challenge_topic_create", "challenge_topic", id, map[string]any{}, map[string]any{"week_start": weekStart, "nown_media_id": nownMediaID}); err != nil {
		return nil, err
	}
	if err = lockPortalAdmin(ctx, tx, adminID, m.now()); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return m.GetChallengeTopic(ctx, id)
}

// CurrentChallengeTopic includes a closed current week so its result stays visible.
func (m *Manager) CurrentChallengeTopic(ctx context.Context) (*ChallengeTopic, error) {
	return m.currentChallengeTopicAt(ctx, m.now())
}
func (m *Manager) currentChallengeTopicAt(ctx context.Context, now time.Time) (*ChallengeTopic, error) {
	var id string
	err := m.db.QueryRowContext(ctx, `SELECT id FROM challenge_topics WHERE activated_at IS NOT NULL AND week_start<=$1::date AND week_end>=$1::date ORDER BY week_start DESC LIMIT 1`, serverDay(now.UTC())).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m.GetChallengeTopic(ctx, id)
}
func (m *Manager) ActiveChallengeTopic(ctx context.Context) (*ChallengeTopic, error) {
	t, err := m.CurrentChallengeTopic(ctx)
	if err != nil || t == nil {
		return t, err
	}
	if !topicOpen(t, m.now()) {
		return nil, nil
	}
	return t, nil
}

// lockTopic is shared by intake, review, voting and close; they serialize on one row.
func lockTopic(ctx context.Context, tx *sql.Tx, id string) (*ChallengeTopic, error) {
	var t ChallengeTopic
	var closed sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT id,week_start,week_end,closed_at FROM challenge_topics WHERE id=$1 FOR UPDATE`, id).Scan(&t.ID, &t.WeekStart, &t.WeekEnd, &closed)
	if err != nil {
		return nil, errors.New("challenge_not_open")
	}
	t.ClosedAt = nullableTime(closed)
	return &t, nil
}

func (m *Manager) SubmitChallengeEntry(ctx context.Context, accountID, topicID string, entryType MediaType, content string, consent ...ContributionConsent) (*ChallengeEntry, error) {
	if !m.portalAccountAllowed(ctx, accountID) {
		return nil, errors.New("challenge_forbidden")
	}
	normalized, validationErr := m.normalizedContribution(content)
	content = normalized
	if entryType != MediaText || validationErr != nil {
		return nil, errors.New("invalid_request")
	}
	if len(consent) != 1 || !consent[0].Accepted {
		return nil, errors.New("terms_required")
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	topic, err := lockTopic(ctx, tx, topicID)
	if err != nil {
		return nil, err
	}
	if err = m.authorizePortalWriteTx(ctx, tx, accountID, "", true); err != nil {
		return nil, err
	}
	if !topicOpen(topic, m.now()) {
		return nil, errors.New("challenge_not_open")
	}
	var terms string
	if err = tx.QueryRowContext(ctx, `SELECT version FROM portal_terms WHERE active_from<=now() ORDER BY active_from DESC,version DESC LIMIT 1 FOR SHARE`).Scan(&terms); err != nil {
		return nil, err
	}
	if consent[0].Version != terms {
		return nil, errors.New("terms_outdated")
	}
	var already bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM challenge_entries WHERE account_id=$1 AND topic_id=$2)`, accountID, topicID).Scan(&already); err != nil {
		return nil, err
	}
	if already {
		return nil, errors.New("challenge_already_submitted")
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM challenge_entries WHERE topic_id=$1 AND status<>'rejected'`, topicID).Scan(&count); err != nil {
		return nil, err
	}
	if count >= m.maxChallengeEntries() {
		return nil, errors.New("challenge_full")
	}
	var slot int
	if err = tx.QueryRowContext(ctx, `SELECT n FROM generate_series(1,$2) AS n WHERE NOT EXISTS(SELECT 1 FROM challenge_entries WHERE topic_id=$1 AND status<>'rejected' AND slot_number=n) ORDER BY n LIMIT 1`, topicID, m.maxChallengeEntries()).Scan(&slot); err != nil {
		return nil, err
	}
	id := uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO challenge_entries(id,account_id,topic_id,entry_type,content,terms_version,terms_accepted_at,status,slot_number) VALUES($1,$2,$3,'text',$4,$5,now(),'submitted',$6)`, id, accountID, topicID, content, terms, slot)
	if err != nil {
		return nil, err
	}
	if err = auditTx(ctx, tx, "", "challenge_entry_submit", "challenge_entry", id, map[string]any{}, map[string]any{"actor_account_id": accountID, "topic_id": topicID, "terms_version": terms, "status": "submitted"}); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return m.GetChallengeEntry(ctx, id)
}

// ListChallengeReviewEntries is private to authenticated reviewer surfaces.
func (m *Manager) ListChallengeReviewEntries(ctx context.Context, topicID string) ([]ChallengeEntry, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id,account_id,topic_id,entry_type,content,asset_ref,status,vote_count,slot_number,rejection_reason FROM challenge_entries WHERE topic_id=$1 ORDER BY created_at,id`, topicID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChallengeEntries(rows)
}

func (m *Manager) VoteChallengeEntry(ctx context.Context, accountID, topicID, entryID string) error {
	if !m.portalAccountAllowed(ctx, accountID) {
		return errors.New("challenge_forbidden")
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	topic, err := lockTopic(ctx, tx, topicID)
	if err != nil {
		return err
	}
	if err = m.authorizePortalWriteTx(ctx, tx, accountID, "", false); err != nil {
		return err
	}
	if !topicOpen(topic, m.now()) {
		return errors.New("challenge_not_open")
	}
	var owner, status string
	if err = tx.QueryRowContext(ctx, `SELECT account_id,status FROM challenge_entries WHERE id=$1 AND topic_id=$2`, entryID, topicID).Scan(&owner, &status); err != nil {
		return errors.New("challenge_entry_unavailable")
	}
	if owner == accountID {
		return errors.New("challenge_self_vote")
	}
	if status != "approved" {
		return errors.New("challenge_entry_unavailable")
	}
	// The voter account lock also serializes either direction of a block write.
	// Existing votes remain immutable; only new interaction with hidden UGC stops.
	var blocked bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM player_blocks WHERE (actor_id=$1 AND target_id=$2) OR (actor_id=$2 AND target_id=$1))`, accountID, owner).Scan(&blocked); err != nil {
		return err
	}
	if blocked {
		return errors.New("challenge_entry_unavailable")
	}
	var id string
	err = tx.QueryRowContext(ctx, `INSERT INTO challenge_votes(account_id,topic_id,entry_id) VALUES($1,$2,$3) ON CONFLICT(account_id,topic_id) DO NOTHING RETURNING id`, accountID, topicID, entryID).Scan(&id)
	if err == sql.ErrNoRows {
		return errors.New("challenge_already_voted")
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE challenge_entries SET vote_count=vote_count+1,updated_at=now() WHERE id=$1`, entryID); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *Manager) ApproveChallengeEntry(ctx context.Context, adminID, entryID string) error {
	return m.decideChallengeEntry(ctx, adminID, entryID, true, "")
}
func (m *Manager) RejectChallengeEntry(ctx context.Context, adminID, entryID, reason string) error {
	return m.decideChallengeEntry(ctx, adminID, entryID, false, reason)
}
func (m *Manager) decideChallengeEntry(ctx context.Context, adminID, entryID string, approve bool, reason string) error {
	var screenedContent string
	if approve {
		entry, err := m.GetChallengeEntry(ctx, entryID)
		if err != nil {
			return err
		}
		if entry.EntryType != MediaText {
			return errors.New("unsupported entry type")
		}
		if entry.Status != EntrySubmitted && entry.Status != EntryScreening {
			return errors.New("entry not screenable")
		}
		if err = m.screenText(ctx, entry.Content); err != nil {
			return err
		}
		screenedContent = entry.Content
	}

	if !approve && strings.TrimSpace(reason) == "" {
		return errors.New("rejection reason required")
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var topicID string
	if err = tx.QueryRowContext(ctx, `SELECT topic_id FROM challenge_entries WHERE id=$1`, entryID).Scan(&topicID); err != nil {
		return err
	}
	topic, err := lockTopic(ctx, tx, topicID)
	if err != nil {
		return err
	}
	if err = lockPortalAdmin(ctx, tx, adminID, m.now()); err != nil {
		return err
	}
	if !topicOpen(topic, m.now()) {
		return errors.New("challenge_not_open")
	}
	var status, content string
	var slot sql.NullInt32
	if err = tx.QueryRowContext(ctx, `SELECT status,slot_number,content FROM challenge_entries WHERE id=$1 FOR UPDATE`, entryID).Scan(&status, &slot, &content); err != nil {
		return err
	}
	if status != "submitted" && status != "screening" && (!(!approve && status == "approved")) {
		return errors.New("entry not screenable")
	}
	if approve && content != screenedContent {
		return errors.New("entry changed during screening")
	}
	next := "rejected"
	var nextSlot any
	if approve {
		next = "approved"
		nextSlot = slot
		if !slot.Valid { // Legacy pending rows predate intake reservation.
			var n int
			err = tx.QueryRowContext(ctx, `SELECT n FROM generate_series(1,$2) AS n WHERE NOT EXISTS(SELECT 1 FROM challenge_entries WHERE topic_id=$1 AND status<>'rejected' AND slot_number=n) ORDER BY n LIMIT 1`, topicID, m.maxChallengeEntries()).Scan(&n)
			if err != nil {
				return errors.New("challenge_full")
			}
			nextSlot = n
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE challenge_entries SET status=$2,slot_number=$3,screen_decided_at=COALESCE(screen_decided_at,now()),screen_decided_by=COALESCE(screen_decided_by,$4),rejection_reason=$5,updated_at=now() WHERE id=$1`, entryID, next, nextSlot, adminID, reason)
	if err != nil {
		return err
	}
	if err = auditTx(ctx, tx, adminID, "challenge_entry_"+next, "challenge_entry", entryID, map[string]any{"status": status}, map[string]any{"status": next, "reason": reason}); err != nil {
		return err
	}
	if err = lockPortalAdmin(ctx, tx, adminID, m.now()); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *Manager) GetChallengeTopic(ctx context.Context, id string) (*ChallengeTopic, error) {
	var t ChallengeTopic
	var closedAt sql.NullTime
	var winnerID sql.NullString
	err := m.db.QueryRowContext(ctx,
		`SELECT id, week_start, week_end, nown_media_id, published_at, closed_at, winner_entry_id
		 FROM challenge_topics WHERE id = $1`,
		id,
	).Scan(&t.ID, &t.WeekStart, &t.WeekEnd, &t.NownMediaID, &t.PublishedAt, &closedAt, &winnerID)
	if err != nil {
		return nil, fmt.Errorf("load topic: %w", err)
	}
	t.ClosedAt = nullableTime(closedAt)
	t.WinnerEntryID = nullableString(winnerID)
	return &t, nil
}

func (m *Manager) GetChallengeEntry(ctx context.Context, id string) (*ChallengeEntry, error) {
	var e ChallengeEntry
	var assetRef sql.NullString
	var slotNumber sql.NullInt32
	var rejection sql.NullString
	err := m.db.QueryRowContext(ctx,
		`SELECT id, account_id, topic_id, entry_type, content, asset_ref, status, vote_count, slot_number, rejection_reason
		 FROM challenge_entries WHERE id = $1`,
		id,
	).Scan(&e.ID, &e.AccountID, &e.TopicID, &e.EntryType, &e.Content, &assetRef, &e.Status, &e.VoteCount, &slotNumber, &rejection)
	if err != nil {
		return nil, fmt.Errorf("load entry: %w", err)
	}
	e.AssetRef = assetRef.String
	e.SlotNumber = int(slotNumber.Int32)
	e.RejectionReason = rejection.String
	return &e, nil
}

func (m *Manager) ListChallengeEntries(ctx context.Context, topicID string) ([]ChallengeEntry, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, account_id, topic_id, entry_type, content, asset_ref, status, vote_count, slot_number, rejection_reason
		 FROM challenge_entries
		 WHERE topic_id = $1 AND status = 'approved'
		 ORDER BY slot_number ASC, created_at ASC`,
		topicID,
	)
	if err != nil {
		return nil, fmt.Errorf("query entries: %w", err)
	}
	defer rows.Close()
	return scanChallengeEntries(rows)
}
func challengeEntryJSON(e ChallengeEntry) map[string]any {
	return map[string]any{"id": e.ID, "account_id": e.AccountID, "entry_type": e.EntryType, "content": e.Content, "status": e.Status, "vote_count": e.VoteCount, "rejection_reason": e.RejectionReason}
}

// ChallengeSnapshot never exposes another player's unscreened entry.
func (m *Manager) ChallengeSnapshot(ctx context.Context, accountID string) (map[string]any, error) {
	if !m.portalAccountAllowed(ctx, accountID) {
		return nil, errors.New("challenge_forbidden")
	}
	topic, err := m.CurrentChallengeTopic(ctx)
	if err != nil || topic == nil {
		return nil, err
	}
	nown, err := m.GetSubmission(ctx, topic.NownMediaID)
	if err != nil {
		return nil, err
	}
	if nown.MediaType != MediaText || (nown.Status != StatusApproved && nown.Status != StatusPublished) {
		return nil, errors.New("topic media unavailable")
	}
	terms, err := m.CurrentTerms(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := m.db.QueryContext(ctx, `SELECT e.id,e.account_id,e.entry_type,e.content,e.vote_count,COALESCE(a.nickname,'') FROM challenge_entries e JOIN accounts a ON a.id=e.account_id WHERE e.topic_id=$1 AND e.status='approved'
	 AND NOT EXISTS(SELECT 1 FROM player_blocks b WHERE (b.actor_id=$2 AND b.target_id=e.account_id) OR (b.actor_id=e.account_id AND b.target_id=$2))
	 ORDER BY e.slot_number,e.created_at,e.id LIMIT $3`, topic.ID, accountID, m.maxChallengeEntries())
	if err != nil {
		return nil, err
	}
	public := make([]map[string]any, 0)
	for rows.Next() {
		var id, owner, kind, content, nickname string
		var votes int
		if err = rows.Scan(&id, &owner, &kind, &content, &votes, &nickname); err != nil {
			rows.Close()
			return nil, err
		}
		public = append(public, map[string]any{"id": id, "account_id": owner, "entry_type": kind, "content": content, "vote_count": votes, "nickname": nickname})
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	var own any
	var ownID string
	err = m.db.QueryRowContext(ctx, `SELECT id FROM challenge_entries WHERE topic_id=$1 AND account_id=$2`, topic.ID, accountID).Scan(&ownID)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if ownID != "" {
		e, err := m.GetChallengeEntry(ctx, ownID)
		if err != nil {
			return nil, err
		}
		own = challengeEntryJSON(*e)
	}
	var votedID string
	err = m.db.QueryRowContext(ctx, `SELECT entry_id FROM challenge_votes WHERE topic_id=$1 AND account_id=$2`, topic.ID, accountID).Scan(&votedID)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	var vote any
	if votedID != "" {
		vote = votedID
	}
	var admitted int
	if err = m.db.QueryRowContext(ctx, `SELECT count(*) FROM challenge_entries WHERE topic_id=$1 AND status<>'rejected'`, topic.ID).Scan(&admitted); err != nil {
		return nil, err
	}
	remaining := max(0, m.maxChallengeEntries()-admitted)
	open := topicOpen(topic, m.now())
	return map[string]any{"topic": map[string]any{"id": topic.ID, "week_start": topic.WeekStart, "week_end": topic.WeekEnd, "closed_at": topic.ClosedAt, "winner_entry_id": topic.WinnerEntryID, "nown": map[string]any{"type": "text", "content": nown.Content}}, "entries": public, "own_entry": own, "voted_entry_id": vote, "terms": map[string]any{"version": terms.Version, "title": terms.Title, "body": terms.Body}, "intake_remaining": remaining, "can_submit": open && ownID == "" && remaining > 0, "can_vote": open && votedID == "", "max_text_bytes": m.MaxTextBytes()}, nil
}
