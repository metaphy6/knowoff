package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/knowoff/knowoff/server/pkg/media"
	"github.com/knowoff/knowoff/server/pkg/textcert"
)

var ErrTextReleaseUnavailable = errors.New("text.release_unavailable")

type TextPackAccess struct {
	Class          string
	EntitlementKey string
}
type TextReleaseStore struct {
	db       *sql.DB
	tuning   config.TuningConfig
	screen   func(context.Context, string) error
	cacheMu  sync.Mutex
	verified map[string]*media.TextSnapshot
}

func NewTextReleaseStore(db *sql.DB, tuning config.TuningConfig, screen func(context.Context, string) error) *TextReleaseStore {
	return &TextReleaseStore{db: db, tuning: tuning.Clone(), screen: screen, verified: make(map[string]*media.TextSnapshot)}
}
func (s *TextReleaseStore) limits() media.TextLimits {
	t := s.tuning
	return media.TextLimits{MaxTextBytes: t.Contract.MaxTextBytes, MaxRecords: t.TextCatalog.MaxRecords, MaxFileBytes: t.TextCatalog.MaxFileBytes, MaxBundleBytes: t.TextCatalog.MaxBundleBytes}
}
func (s *TextReleaseStore) dealing() media.TextDealTuning {
	t := s.tuning
	return media.TextDealTuning{HandSize: t.Hand.Size, ReserveSize: t.Hand.DrawPile, MinHigh: t.Dealing.MinHighPerNown, MinDistant: t.Dealing.MinDistantPerNown, MaxSearchNodes: t.TextCatalog.MaxSearchNodes}
}
func textAdmin(ctx context.Context, tx *sql.Tx, id string) error {
	return LockAdminTx(ctx, tx, id, []string{"admin", "superadmin"})
}
func textReleaseAudit(ctx context.Context, tx *sql.Tx, admin, action, id string, detail any) error {
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO admin_audit_log(admin_id,action,target_type,target_id,after_state) VALUES($1,$2,'text_release',$3,$4)`, admin, action, id, string(raw))
	if err != nil {
		return err
	}
	return textAdmin(ctx, tx, admin)
}

type acceptedSource struct {
	text, terms, attribution, reviewer, account string
	consented, reviewed                         time.Time
}

func readAcceptedSource(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, kind, id string, lock bool) (acceptedSource, error) {
	var v acceptedSource
	query := ""
	switch kind {
	case "portal_submission":
		query = `SELECT p.content,p.terms_version,p.terms_accepted_at,p.decided_at,p.decided_by,a.nickname,p.account_id FROM portal_submissions p JOIN accounts a ON a.id=p.account_id WHERE p.id=$1 AND p.media_type='text' AND p.status IN ('approved','published') AND p.decided_at IS NOT NULL AND p.decided_by IS NOT NULL`
	case "challenge_entry":
		query = `SELECT p.content,p.terms_version,p.terms_accepted_at,p.screen_decided_at,p.screen_decided_by,a.nickname,p.account_id FROM challenge_entries p JOIN accounts a ON a.id=p.account_id WHERE p.id=$1 AND p.entry_type='text' AND p.status='approved' AND p.screen_decided_at IS NOT NULL AND p.screen_decided_by IS NOT NULL`
	default:
		return v, ErrTextArchiveConflict
	}
	if lock {
		query += " FOR SHARE OF p"
	}
	err := q.QueryRowContext(ctx, query, id).Scan(&v.text, &v.terms, &v.consented, &v.reviewed, &v.reviewer, &v.attribution, &v.account)
	if err != nil {
		return v, err
	}
	// The source-row lock may have waited behind confirmation. Use a new
	// READ COMMITTED statement after that wait, without acquiring a creator
	// account lock after the source lock. Read-only previews retain their
	// existing snapshot; the authoritative start always repeats this check.
	var available bool
	if err = q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts a WHERE a.id=$1 AND a.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM account_deletion_fences f WHERE f.account_id=a.id))`, v.account).Scan(&available); err != nil {
		return v, err
	}
	if !available {
		return v, ErrTextReleaseUnavailable
	}
	return v, nil
}

// CaptureAccepted rescreens immutable approved bytes and retains original consent,
// attribution and human review identity. It performs no approval or payout.
func (s *TextReleaseStore) CaptureAccepted(ctx context.Context, admin, kind, id string) (media.TextProvenance, error) {
	var out media.TextProvenance
	if !valueUUID(id) || !valueUUID(admin) || s.screen == nil {
		return out, ErrTextReleaseUnavailable
	}
	before, err := readAcceptedSource(ctx, s.db, kind, id, false)
	if err != nil {
		return out, err
	}
	normal, err := media.NormalizeText(before.text, s.tuning.Contract.MaxTextBytes)
	if err != nil || normal != before.text {
		return out, ErrTextArchiveConflict
	}
	if err = s.screen(ctx, before.text); err != nil {
		return out, err
	}
	err = WithValueTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := textAdmin(ctx, tx, admin); err != nil {
			return err
		}
		current, err := readAcceptedSource(ctx, tx, kind, id, true)
		if err != nil {
			return err
		}
		if current != before {
			return ErrTextArchiveConflict
		}
		inputID := uuid.NewString()
		out = media.TextProvenance{SourceKind: "contribution", SourceID: inputID, SourceRevision: 1, AcceptedTextSHA256: media.ContentHash([]byte(current.text)), TermsVersion: current.terms, ConsentReference: id, ConsentAtMS: current.consented.UnixMilli(), ApprovalReference: id, EditorReference: current.reviewer, ReviewedAtMS: current.reviewed.UnixMilli(), License: "contribution-terms:" + current.terms, Attribution: current.attribution}
		raw, err := json.Marshal(out)
		if err != nil {
			return err
		}
		var submission, entry any
		if kind == "portal_submission" {
			submission = id
		} else {
			entry = id
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO text_accepted_inputs(id,source_kind,source_id,text_content,provenance,submission_id,entry_id) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(source_kind,source_id) DO NOTHING`, inputID, kind, id, current.text, string(raw), submission, entry); err != nil {
			return err
		}
		var oldText string
		var oldRaw []byte
		if err = tx.QueryRowContext(ctx, `SELECT text_content,provenance FROM text_accepted_inputs WHERE source_kind=$1 AND source_id=$2`, kind, id).Scan(&oldText, &oldRaw); err != nil {
			return err
		}
		var old media.TextProvenance
		if err = json.Unmarshal(oldRaw, &old); err != nil {
			return err
		}
		out.SourceID = old.SourceID
		// Credit is frozen with the first accepted-input capture. A public profile
		// rename does not alter consent, reviewed wording or existing attribution.
		out.Attribution = old.Attribution
		if oldText != current.text || old != out {
			return ErrTextArchiveConflict
		}
		out = old
		if old.SourceID != inputID {
			return textAdmin(ctx, tx, admin)
		}
		return textReleaseAudit(ctx, tx, admin, "text_input_capture", inputID, map[string]string{"source_kind": kind, "source_id": id})
	})
	return out, err
}

// Publish commits artifact and revision lineage together, after the same runtime
// certificate validation and exact accepted-input checks. Activation is separate.
func (s *TextReleaseStore) Publish(ctx context.Context, admin string, candidate *media.TextSnapshot, access TextPackAccess) error {
	if candidate == nil || !valueUUID(admin) {
		return ErrTextReleaseUnavailable
	}
	if (access.Class != "core" && access.Class != "featured" && access.Class != "theme") || (access.Class == "theme") != (access.EntitlementKey != "") || access.EntitlementKey != "" && !gamecontract.ValidIdentifier(access.EntitlementKey) {
		return ErrTextArchiveConflict
	}
	m := candidate.Manifest()
	if err := textcert.ValidateActivationContext(ctx, candidate, m.RulesVersion, s.tuning); err != nil {
		return err
	}
	bundle := candidate.Bundle()
	raw, err := json.Marshal(bundle)
	if err != nil {
		return err
	}
	if int64(len(raw)) > s.limits().MaxBundleBytes {
		return ErrTextArchiveConflict
	}
	lineage := candidate.Lineage()
	return WithValueTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := textAdmin(ctx, tx, admin); err != nil {
			return err
		}
		// Serialize publication, activation and takedown without holding any match lock.
		if _, err := tx.ExecContext(ctx, `LOCK TABLE text_releases IN SHARE ROW EXCLUSIVE MODE`); err != nil {
			return err
		}
		var oldHash, oldSnapshot, oldClass, oldKey string
		err := tx.QueryRowContext(ctx, `SELECT manifest_sha256,snapshot_sha256,access_class,entitlement_key FROM text_releases WHERE release_id=$1`, m.ReleaseID).Scan(&oldHash, &oldSnapshot, &oldClass, &oldKey)
		if err == nil {
			if oldHash != lineage.ManifestSHA256 || oldSnapshot != lineage.SnapshotSHA256 || oldClass != access.Class || oldKey != access.EntitlementKey {
				return ErrTextArchiveConflict
			}
			if err := validateReleaseInputs(ctx, tx, bundle, true); err != nil {
				return err
			}
			return textAdmin(ctx, tx, admin)
		}
		if err != sql.ErrNoRows {
			return err
		}
		if err := validateReleaseInputs(ctx, tx, bundle, true); err != nil {
			return err
		}
		for _, r := range lineage.Revisions {
			if _, err := tx.ExecContext(ctx, `INSERT INTO text_content_revisions(language,content_id,revision,sha256) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, r.Language, r.ContentID, r.Revision, r.SHA256); err != nil {
				return err
			}
			var hash string
			if err := tx.QueryRowContext(ctx, `SELECT sha256 FROM text_content_revisions WHERE language=$1 AND content_id=$2 AND revision=$3`, r.Language, r.ContentID, r.Revision).Scan(&hash); err != nil {
				return err
			}
			if hash != r.SHA256 {
				return ErrTextArchiveConflict
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO text_releases(release_id,language,rules_version,manifest_sha256,snapshot_sha256,bundle,access_class,entitlement_key,published_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, m.ReleaseID, m.Language, m.RulesVersion, lineage.ManifestSHA256, lineage.SnapshotSHA256, string(raw), access.Class, access.EntitlementKey, admin); err != nil {
			return err
		}
		return textReleaseAudit(ctx, tx, admin, "text_release_publish", m.ReleaseID, map[string]string{"snapshot_sha256": candidate.SHA256()})
	})
}
func (s *TextReleaseStore) load(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (*media.TextSnapshot, TextPackAccess, error) {
	var raw []byte
	var manifestHash, snapshotHash string
	var a TextPackAccess
	err := q.QueryRowContext(ctx, `SELECT bundle,manifest_sha256,snapshot_sha256,access_class,entitlement_key FROM text_releases WHERE release_id=$1 AND withdrawn_at IS NULL`, id).Scan(&raw, &manifestHash, &snapshotHash, &a.Class, &a.EntitlementKey)
	if err != nil {
		return nil, a, err
	}
	if err := ctx.Err(); err != nil {
		return nil, a, err
	}
	key := manifestHash + ":" + snapshotHash
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if snap, ok := s.verified[key]; ok {
		return snap, a, nil
	}
	if int64(len(raw)) > s.limits().MaxBundleBytes {
		return nil, a, ErrTextArchiveConflict
	}
	var b media.TextBundle
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&b); err != nil {
		return nil, a, err
	}
	snap, err := media.NewTextSnapshot(b, s.limits())
	if err != nil {
		return nil, a, err
	}
	if snap.ManifestSHA256() != manifestHash || snap.SHA256() != snapshotHash {
		return nil, a, ErrTextArchiveConflict
	}
	if err = textcert.ValidateActivationContext(ctx, snap, snap.Manifest().RulesVersion, s.tuning); err != nil {
		return nil, a, err
	}
	if err = ctx.Err(); err != nil {
		return nil, a, err
	}
	if len(s.verified) >= 64 {
		s.verified = make(map[string]*media.TextSnapshot)
	}
	s.verified[key] = snap
	return snap, a, nil
}
func (s *TextReleaseStore) Activate(ctx context.Context, admin, id string) error {
	return WithValueTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := textAdmin(ctx, tx, admin); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `LOCK TABLE text_releases IN SHARE ROW EXCLUSIVE MODE`); err != nil {
			return err
		}
		snap, access, err := s.load(ctx, tx, id)
		if err != nil {
			return err
		}
		m := snap.Manifest()
		if err = validateReleaseInputs(ctx, tx, snap.Bundle(), true); err != nil {
			return err
		}
		key := access.Class
		if access.Class == "theme" {
			key = "theme:" + access.EntitlementKey
		}
		var old string
		err = tx.QueryRowContext(ctx, `SELECT release_id FROM text_active_releases WHERE language=$1 AND rules_version=$2 AND access_key=$3`, m.Language, m.RulesVersion, key).Scan(&old)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if old == id {
			return textAdmin(ctx, tx, admin)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO text_active_releases(language,rules_version,access_key,release_id) VALUES($1,$2,$3,$4) ON CONFLICT(language,rules_version,access_key) DO UPDATE SET release_id=EXCLUDED.release_id,activated_at=now()`, m.Language, m.RulesVersion, key, id); err != nil {
			return err
		}
		return textReleaseAudit(ctx, tx, admin, "text_release_activate", id, map[string]string{"previous": old})
	})
}
func (s *TextReleaseStore) Takedown(ctx context.Context, admin, id, reason string) error {
	return WithValueTransaction(ctx, s.db, func(tx *sql.Tx) error { return s.TakedownTx(ctx, tx, admin, id, reason) })
}

// TakedownTx lets a moderation case share the release withdrawal transaction.
// Caller commits or rolls back; no notices or public match state are mutated.
func (s *TextReleaseStore) TakedownTx(ctx context.Context, tx *sql.Tx, admin, id, reason string) error {
	if tx == nil || reason == "" || len(reason) > 1024 {
		return ErrTextArchiveConflict
	}
	if err := textAdmin(ctx, tx, admin); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `LOCK TABLE text_releases IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return err
	}
	var withdrawn sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT withdrawn_at FROM text_releases WHERE release_id=$1`, id).Scan(&withdrawn); err != nil {
		return ErrTextReleaseUnavailable
	}
	if withdrawn.Valid {
		return textAdmin(ctx, tx, admin)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE text_releases SET withdrawn_at=now() WHERE release_id=$1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM text_active_releases WHERE release_id=$1`, id); err != nil {
		return err
	}
	return textReleaseAudit(ctx, tx, admin, "text_release_takedown", id, map[string]string{"reason": reason})
}

// Resolve chooses the active free core release. Explicit pack choices use ResolveRelease.
func (s *TextReleaseStore) Resolve(ctx context.Context, language, rules string) (*media.TextSnapshot, error) {
	var id string
	if err := s.db.QueryRowContext(ctx, `SELECT release_id FROM text_active_releases WHERE language=$1 AND rules_version=$2 AND access_key='core'`, language, rules).Scan(&id); err != nil {
		return nil, ErrTextReleaseUnavailable
	}
	return s.ResolveRelease(ctx, language, rules, id)
}
func (s *TextReleaseStore) ResolveRelease(ctx context.Context, language, rules, id string) (*media.TextSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM text_active_releases WHERE language=$1 AND rules_version=$2 AND release_id=$3)`, language, rules, id).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrTextReleaseUnavailable
	}
	snap, _, err := s.load(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if snap.Manifest().RulesVersion != rules || snap.Manifest().Language != language {
		return nil, ErrTextReleaseUnavailable
	}
	if err = validateReleaseInputs(ctx, tx, snap.Bundle(), false); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return snap, nil
}

func validateReleaseInputs(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, bundle media.TextBundle, lock bool) error {
	validate := func(text string, p media.TextProvenance) error {
		if p.SourceKind != "contribution" || !valueUUID(p.SourceID) {
			return ErrTextArchiveConflict
		}
		var wording, kind, id string
		var stored []byte
		if err := q.QueryRowContext(ctx, `SELECT text_content,provenance,source_kind,source_id FROM text_accepted_inputs WHERE id=$1`, p.SourceID).Scan(&wording, &stored, &kind, &id); err != nil {
			return err
		}
		var accepted media.TextProvenance
		if err := json.Unmarshal(stored, &accepted); err != nil {
			return err
		}
		if wording != text || accepted != p {
			return ErrTextArchiveConflict
		}
		current, err := readAcceptedSource(ctx, q, kind, id, lock)
		if err != nil {
			return err
		}
		if current.text != wording || current.terms != p.TermsVersion || current.consented.UnixMilli() != p.ConsentAtMS || current.reviewed.UnixMilli() != p.ReviewedAtMS || current.reviewer != p.EditorReference {
			return ErrTextArchiveConflict
		}
		return nil
	}
	for _, n := range bundle.Nowns {
		if err := validate(n.Text, n.Provenance); err != nil {
			return err
		}
	}
	for _, c := range bundle.Cards {
		if err := validate(c.Text, c.Provenance); err != nil {
			return err
		}
	}
	return nil
}

// CheckAccess is the lobby preview. ValidateStart repeats it inside the durable
// start transaction while the participant account locks serialize entitlement changes.
func (s *TextReleaseStore) CheckAccess(ctx context.Context, account string, settings v2.LobbySettings, path string, at time.Time) error {
	if !valueUUID(account) || at.IsZero() {
		return ErrTextReleaseUnavailable
	}
	return WithValueTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := LockValueAccount(ctx, tx, account); err != nil {
			return err
		}
		return s.checkPackAccess(ctx, tx, account, settings, path, "", at)
	})
}

func (s *TextReleaseStore) ValidateStart(ctx context.Context, tx *sql.Tx, record TextMatchRecord, accounts []string, at time.Time) error {
	if record.Prototype {
		return nil
	}
	c := record.Contract
	if c.Eligibility.EntryPath == "local" {
		found := false
		for _, id := range accounts {
			if id == record.SponsorAccountID {
				found = true
			}
		}
		if !found {
			return ErrTextReleaseUnavailable
		}
	} else if record.SponsorAccountID != "" {
		return ErrTextReleaseUnavailable
	}
	return s.checkPackAccess(ctx, tx, record.SponsorAccountID, v2.LobbySettings{ModeID: c.ModeID, Size: c.OriginalSize, ContentLanguage: c.ContentLanguage, RulesVersion: c.RulesVersion, PackReleaseID: c.PackReleaseID}, c.Eligibility.EntryPath, c.PackSHA256, at)
}

func (s *TextReleaseStore) checkPackAccess(ctx context.Context, tx *sql.Tx, sponsor string, settings v2.LobbySettings, path, expectedHash string, at time.Time) error {
	if path != "local" && path != "quick_play" {
		return ErrTextReleaseUnavailable
	}
	var id string
	// Match the withdrawal lock order explicitly: release, then active mapping,
	// then source rows. A joined FOR SHARE has no guaranteed row-lock order.
	if err := tx.QueryRowContext(ctx, `SELECT release_id FROM text_releases WHERE release_id=$1 AND withdrawn_at IS NULL FOR SHARE`, settings.PackReleaseID).Scan(&id); err != nil {
		return ErrTextReleaseUnavailable
	}
	if err := tx.QueryRowContext(ctx, `SELECT release_id FROM text_active_releases WHERE release_id=$1 AND language=$2 AND rules_version=$3 FOR SHARE`, id, settings.ContentLanguage, settings.RulesVersion).Scan(&id); err != nil {
		return ErrTextReleaseUnavailable
	}

	snap, access, err := s.load(ctx, tx, id)
	if err != nil {
		return err
	}
	if expectedHash != "" && expectedHash != snap.SHA256() {
		return ErrTextReleaseUnavailable
	}
	allowed := false
	for _, mode := range snap.Manifest().Modes {
		if mode == settings.ModeID {
			allowed = true
		}
	}
	if !allowed || (settings.Size != 4 && settings.Size != 6) {
		return ErrTextReleaseUnavailable
	}
	if err = validateReleaseInputs(ctx, tx, snap.Bundle(), true); err != nil {
		return err
	}
	if access.Class != "theme" {
		return nil
	}
	if path != "local" || !valueUUID(sponsor) {
		return ErrTextReleaseUnavailable
	}
	var owns bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM entitlements WHERE account_id=$1 AND entitlement_type='theme_pack' AND value=$2 AND (active_until IS NULL OR active_until>$3))
	 OR EXISTS(SELECT 1 FROM named_entitlement_items WHERE account_id=$1 AND entitlement_type='theme_pack' AND value=$2)`, sponsor, access.EntitlementKey, valueTime(at)).Scan(&owns)
	if err != nil {
		return err
	}
	if !owns {
		return ErrTextReleaseUnavailable
	}
	return nil
}
